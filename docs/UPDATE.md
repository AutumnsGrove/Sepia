# Updating Sepia

Sepia updates itself the way Polaris, Ivy and Warbler do. The page asks, a host-side watcher does the
work, and a failed update rolls back on its own. The state machine below is modelled on Warbler's
`update/view.ts`, which has the most careful version of it. The screens are in
[`mockups/05-settings-and-updates.html`](../mockups/05-settings-and-updates.html).

## How it works

1. **The container never controls Docker.** There is no socket mount. The panel calls Sepia's own REST API,
   and Sepia writes a signal file that a **host-side watcher** (a systemd path unit, `sepia-update.path`)
   picks up.
2. **The watcher updates.** It resolves the latest published image digest from GHCR, fast-forwards the host
   checkout so `docker-compose.yml` and the bind-mounted config land with the image, pins the new digest in
   `.env`, recreates the container, and checks `GET http://localhost:8893/healthz` (port 8893, loopback only —
   see `PLAN.md` section 8). Every external step is bounded, and systemd's start timeout is above their sum, so
   the script always gets to write its result.
3. **A failed health check rolls back** to the previous digest and records a result of `failed`, in one of two
   sentences: the previous version is running again, or it did not come back either and the board may be down.
4. **The watcher keeps a heartbeat**, so the app can tell "no updater installed" from "the updater has gone
   quiet". Three missed minutes is quiet; a heartbeat from more than a minute in the future is treated the same
   way, because two clocks that disagree that much are not both telling the truth.

## The handshake, exactly

Everything is a file in the signal directory (`/update-signal` in the container, `./update-signal` on the host,
mounted from the same place). Both sides write through a temp file and a rename, so neither can read the other
half way through.

| File | Written by | Content |
|---|---|---|
| `requested` | app | `action=check`, `action=update` with `target=<digest reference>`, or `action=restart` |
| `last-request` | app | The same lines plus `at=<RFC 3339>`; kept after the request is cleared so a result can be judged against it |
| `available` | watcher | `{latest, latest_digest, checked_at, update_available}`, or `{checked_at, update_available: false, error}` when the check could not finish |
| `result` | watcher | `{status: ok\|failed\|skipped, detail, target, finished_at}` |
| `heartbeat` | watcher | One RFC 3339 instant, every minute and at the start and end of every run |
| `backup-requested` | app | `action=backup`; the host's `sepia-backup.path` fires the backup script, which removes the file when the run is done (S14) |

Three rules follow from that, and each is a test in `internal/update`:

- **A result belongs to a request.** It is shown only when the helper is alive, a request is on record, nothing
  new is pending, and `finished_at` is not more than a minute older than the request's `at`. That allowance is
  the phone's clock and the potato's; anything further apart is a leftover file, not this update's answer.
- **A result without a heartbeat is not trusted.** A write in the directory is not proof that a watcher ran.
- **A failed check is not "up to date".** It reports an error, and the panel says it could not look.

## REST

| Endpoint | Purpose |
|---|---|
| `GET /api/version` | `{version, deployment: docker\|bare, updater: {state: ready, not_installed, stale; heartbeat_at}}` |
| `POST /api/update/check` | Ask the watcher to look for a newer image. Answers `202 {pending: true}` |
| `GET /api/update/available` | `{available: {current, latest, latest_digest, checked_at, update_available, error?}, check_pending}` |
| `POST /api/update/start` | Install the digest the page was shown. Answers `202 {pending: true}`. Refused with `reply_in_progress` while a reply is being written |
| `GET /api/update/status` | `{pending, result: {status: ok, failed, skipped; detail; target; finished_at}}` |
| `POST /api/restart` | Restart only: no pull, no rebuild. Refused while a reply is being written |
| `GET /api/services` | Reachability of each dependency, for the Services screen |

All state-changing requests go through the same Origin check as everything else
([`PROTOCOL.md`](PROTOCOL.md) section 1).

## The panel's states

A pure function from what the page knows to one of these views, so every state is testable without a
browser:

| View | Shows |
|---|---|
| loading | Nothing yet |
| no updater | "Updates are not working on this server", the helper's state (not installed, or gone quiet), and the command to run on the potato |
| checking | A check was asked for and the watcher has not answered |
| up to date | The version, "Last checked ...", and Check for updates |
| available | The new version and Update now |
| updating | An indeterminate bar, "This takes a minute. Sepia will reload when it is done.", and a longer-than-expected line after 90 s |
| done | "Updated to ...", then the page reloads |
| failed | The reason in plain text. If the rollback worked: "the previous version is running again" and Try again. If the rollback also failed: "Sepia may not be running properly. Check the potato." with no button |

## Rules that are specific to Sepia

- **Update is refused while a reply is being written,** and the panel says "Finish the reply first". A turn in
  flight would be killed by the restart. Drafts, sessions and entries are in SQLite and are untouched.
- **Open sessions reconnect on their own.** The WebSocket drops, the client reconnects with its last
  sequence number ([`PROTOCOL.md`](PROTOCOL.md) section 6), and the update never loses a draft.
- **The new server may carry a new embedded frontend.** `hello` includes the server's version, and a client
  that sees a different one than it was loaded with reloads itself, so an old UI never talks to a new server.
- **"Rollback also failed" cannot always be shown,** because if no version is running there is no app to
  draw the message. The panel covers the case where the app is up and the watcher says so; otherwise the
  fix is on the potato (`journalctl -u sepia-update.service`).
- **Restart is separate from Update.** It does no pull and no rebuild, and is for when something is simply
  stuck.

## Installing and checking it

[`INSTALL.md`](INSTALL.md) has the install: the checkout, `.env`, the five systemd units, `tailscale serve`, the
Meridian tile and the health-check line, and the scratch-copy recipe for the G5 demonstration.

`sepia doctor`, run as `docker compose exec sepia /sepia doctor`, checks the install rather than the app: the bound
address is loopback, the data directory is writable, `.env` is mode 0600 and owned by the container's uid, every key
line is present (never its value), the database's protections are present and refuse a forbidden write, and whether
the helper's heartbeat is fresh.

## Fixture cases to write

- the page derives each view above from synthetic inputs, including a result older than the update that was
  just started (the phone's clock and the potato's differ by up to a minute);
- an update requested while a reply is streaming is refused;
- an update during which the socket drops and resumes loses no draft text;
- a client loaded with an older frontend reloads after `hello` reports a newer server version.
