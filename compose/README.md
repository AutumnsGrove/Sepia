# Host side

Sepia's container never touches Docker. Everything in this folder runs on the potato, as the operator, outside the
container.

| File | What it is |
|---|---|
| `watcher/update.sh` | The update watcher. Checks GHCR, fast-forwards the checkout, pins the digest in `.env`, recreates the container, health-checks it, and rolls back on failure |
| `systemd/sepia-update.path` | Fires the watcher when the app writes `update-signal/requested` |
| `systemd/sepia-update.service` | One run of the watcher, as the operator, bounded at 900 s |
| `systemd/sepia-update.timer` | Hourly backstop: retries a leftover request, otherwise refreshes `available.json` |
| `systemd/sepia-heartbeat.timer` / `.service` | Writes the heartbeat every minute, so the app can tell "not installed" from "gone quiet" |

## The handshake

All of it is files in `@INSTALL_DIR@/update-signal/`, which compose mounts at `/update-signal` in the container.
The app writes, the watcher reads.

| File | Written by | Content |
|---|---|---|
| `requested` | app | `action=check\|update\|restart` and, for an update, `target=ghcr.io/autumnsgrove/sepia@sha256:...` |
| `last-request` | app | The same plus `at=<RFC 3339>`; kept so a result can be compared with the request that asked for it |
| `available` | watcher | `{latest, latest_digest, checked_at, update_available}` or `{checked_at, update_available: false, error}` |
| `result` | watcher | `{status: ok\|failed\|skipped, detail, target, finished_at}` |
| `heartbeat` | watcher | One RFC 3339 instant, refreshed every minute and at the start of every run |

Every write on both sides is a temp file plus a rename, so neither side can read the other half way through.

The compose file also mounts `./.env` read-only at `/run/sepia/env`. Nothing loads it as configuration (the values
arrive through the environment); it exists so `sepia doctor` can check the file's mode and owner without the host
involved.

## Installing the units

`@INSTALL_DIR@` and `@USER@` are placeholders. `docs/INSTALL.md` has the substitutions and the enable commands;
briefly:

```bash
INSTALL_DIR="$HOME/Sepia"
USER_NAME="$(id -un)"
for f in compose/systemd/sepia-*.path compose/systemd/sepia-*.service compose/systemd/sepia-*.timer; do
  sed -e "s|@INSTALL_DIR@|$INSTALL_DIR|g" -e "s|@USER@|$USER_NAME|g" "$f" |
    sudo tee "/etc/systemd/system/$(basename "$f")" >/dev/null
done
sudo systemctl daemon-reload
sudo systemctl enable --now sepia-update.path sepia-update.timer sepia-heartbeat.timer
```

## Trying it without waiting for a release

```bash
# A check only: writes available.json and installation files, changes nothing
systemctl start sepia-update.service

# What the app would ask for, by hand
printf 'action=check\n' > ~/Sepia/update-signal/requested
systemctl start sepia-update.service
cat ~/Sepia/update-signal/available

# The journal, which is where a failed run explains itself
journalctl -u sepia-update.service -n 50 --no-pager
```

The script is tested against scripted fakes of Docker, git and the registry in `internal/archtest/watcher_test.go`,
so a failure in the field can be reproduced without a board.
