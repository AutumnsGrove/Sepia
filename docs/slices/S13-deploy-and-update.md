# S13. Deploy and update

## Purpose

Put Sepia on the potato the way every other app of yours lives there, and make it update itself. The image is
published to GHCR by digest, a host-side watcher pulls and recreates it, a failed health check rolls back, and
Settings shows every state of that flow. After this slice, every later change reaches the potato by pressing a
button.

## Depends on

S09 (a working service) and S12 (the Settings shell). S00 supplies the CI that builds the image.
**S14 depends on it.**

## Where we start

[`../UPDATE.md`](../UPDATE.md) specifies the flow, modelled on Warbler, Polaris and Ivy. Polaris's
`compose/watcher/update.sh`, Warbler's `update/view.ts` and Ivy's `updateFlow.ts` are the references. The
mockups are page 05.

**The board, checked 2026-11-14** (`ssh potato-remote`): Armbian 26.2.1 trixie, aarch64, Docker 29.6.1,
Compose v5.3.1, autumn in the `docker` group. 233 G of disk with 195 G free. **1.9 GiB of RAM with about
760 MiB available and 557 MiB of the 958 MiB swap already in use**, which is the tightest resource on it and
the reason the board never builds the image. Port 8893 is free (8891 Meridian, 8892 Warbler, 8895, 8896
Outpost, 8899 Polaris, 8890 on every interface; 8894, 8897 and 8898 also free). `tailscale serve` already
proxies 8891, 8892, 8895, 8896 and 8899 and has no line for 8893 yet. Four watchers are installed and
waiting: `warbler-update.path`, `ivy-update.path`, `polaris-update.path`, `pixelsort-update.path`. `~/Sepia`
did not exist; `~/Sepia/.env` was created for the keys (see Decisions).

**The house pattern, as it really is** (`docker ps` on 2026-11-14). Every app there is a container, and each
compose file sits at its repository root, so `docker compose` in the project folder is the whole invocation:
**Polaris** `127.0.0.1:8899->8899` plus a `host.docker.internal:host-gateway` entry so it can reach its searxng,
**Warbler** `127.0.0.1:8892->8080` with `read_only`, `cap_drop: [ALL]`, `no-new-privileges` and `tmpfs: /tmp`,
**Pixelsort** `127.0.0.1:8895->8895`, **Ivy** `0.0.0.0:8418` where the host-name check is the guard, and
**searxng** and **vaultwarden** on every interface. **None of them uses host networking.** A `polaris.service`
unit exists but is `inactive` and `disabled`: Polaris is a container now and the unit is a leftover. What that
implies for this slice is in Decisions.

## Decisions (answered 2026-11-14, before starting)

- **Host networking.** `network_mode: host`, not a published loopback port. The container's `127.0.0.1` is the
  potato's, so the loopback-only rule in `config.Load` stays literally true with no exception in the code, the
  host's `tailscale serve` and the watcher's health check reach it the same way as every other app, and Sepia
  can reach Polaris on `127.0.0.1:8899`, which closes `PLAN.md` open question 3 without a second decision. Cost
  accepted: no port mapping and no per-container health isolation, it only behaves on Linux, and `sepia-dev`
  stays the macOS story.
  - **This is a deliberate departure from every sibling, and both alternatives were worse.** Three of them
    publish on loopback, which needs the process to bind `0.0.0.0` *inside* its own namespace, so `config.Load`
    would need an exception to the rule in `STANDARDS.md` 6 that it enforces today, and reaching Polaris would
    need the `host-gateway` line Polaris itself carries. Ivy publishes on every interface and leans on the
    host-name check, which is the guard the loopback rule exists so as not to need. Host networking keeps the
    rule true in the code rather than in a YAML file, and is the only shape that makes the Polaris call work
    with no alias. The cost: Meridian and the health-check script see no published ports, so both reach Sepia
    with a plain `curl http://localhost:8893/healthz`, as they already do for every other app.
- **The compose file sits at the repository root** (`docker-compose.yml`), not in `compose/`, because every
  sibling does it that way and because `docker compose` resolves the project directory from the file's folder:
  anywhere else turns a mount of `./data` into `./compose/data`, and Meridian's compose targets run
  `docker compose` in the project folder. `compose/` keeps the watcher and the systemd units, which is what
  `BUILD.md` 4 means by "docker-compose.yml, the host watcher, systemd units"; that layout line is amended in
  this commit.
- **The watcher fast-forwards the checkout** in the same run as the image swap, as Polaris does, so `compose/`
  and any config the new image needs land with it rather than a release later.
- **The ruleset stays on direct pushes.** The PR-required phase S00 planned for this slice is **declined**:
  Autumn pushes to `main` occasionally and mostly works through pull requests by preference, so forcing a
  review on a single-operator repository buys ceremony and no reviewer. The guards that matter (no force-push,
  no deletion, every required check on every push) are already in place. `BUILD.md` 5, `CI.md` 4.6 and
  `REPO-SETTINGS.md` are amended in this commit rather than left describing a phase that will not happen.
- **Secrets.** The G4 temporary keys were never rotated and have now expired. `~/Sepia/.env` is staged on the
  potato with mode 0600 and owner `autumn:autumn`, holding `SEPIA_IMAGE`, `SEPIA_UID`, `SEPIA_GID` and two
  empty key lines. **Autumn pastes the values; no agent reads them.** The doctor command in S13.6 checks only
  presence, mode and owner, and never echoes a value.
- **G5 is demonstrated on the potato, not locally.** A second compose project, `~/Sepia-scratch`, on port
  8897 (verified free) with its own `sepia-scratch-update.path` and a deliberately broken image. A local Docker
  run would prove the script's logic; the scratch project proves the units, the Docker on that board and the
  rollback under the real systemd path unit.

## Settled

- The container never touches Docker; the watcher is a host systemd path unit (`sepia-update.path`) reacting to a
  signal file, with a heartbeat. It resolves the latest digest, fast-forwards the host checkout, pins the digest
  in `.env`, recreates the container, health-checks `/healthz`, and rolls back on failure.
- REST: `GET /api/version`, `POST /api/update/check`, `GET /api/update/available`, `POST /api/update/start`,
  `GET /api/update/status`, `POST /api/restart`, `GET /api/services`.
- The panel's eight states derive from a pure function; Update is refused while a reply is being written; the
  new frontend is picked up by the `hello` version check.
- Compose project at `~/Sepia` with `network_mode: host` (see Decisions: this supersedes the published-port
  wording in earlier drafts), non-root, read-only root filesystem, port 8893 (verified free), `tailscale serve`,
  `/healthz` with two fields, a Meridian tile, and one line in `service-healthcheck.sh`.

## Scope

1. **S13.1 Image and compose.** The multi-stage `Dockerfile` (the frontend and Go stages pinned to
   `$BUILDPLATFORM`), the hardened compose file, `.env` handling and the env passthrough list.
2. **S13.2 Publish.** `docker-publish.yml` from [`../CI.md`](../CI.md) 2: multi-arch, by digest, SBOM, Trivy.
3. **S13.3 The watcher.** The systemd units and update script, the signal files, the heartbeat, the rollback, and
   the "rollback also failed" result.
4. **S13.4 The update API.** The endpoints above, the in-flight refusal, and the restart.
5. **S13.5 Screens.** Settings, Updates (all states) and Services (reachability and what each outage costs), from
   page 05, driven by a pure view function with a fixture per state.
6. **S13.6 Operations.** The Meridian tile and vital ("reviews this month"), the health-check line, `tailscale
   serve`, the doctor command (`.env` mode 0600, listener not public, triggers present), and `docs/INSTALL.md`.

## Tests and exit

| Route | Cases |
|---|---|
| Happy | A newer digest is detected, installed and reported as done; the page reloads onto the new version |
| Bad | An update requested while a reply streams is refused with a reason; a request with a hostile `Origin` is refused; a watcher result older than the request (clock skew up to a minute) is ignored; a fabricated result file is not trusted without the heartbeat |
| Sad | The health check fails and the old version returns (the **demonstrated, deliberately failed update**); the rollback also fails and the panel says so honestly; the helper is not installed or has gone quiet; the potato reboots mid-update; GHCR is unreachable |

**View function:** a fixture per state. **Watcher:** script tests against a scripted fake Docker in CI. **Live:**
the real flow on the potato is the operator's check.

## Gates

**G5** before the first production deploy: an update and a deliberately failed update with rollback shown on a
scratch container, and the secrets plan.

## Open questions

None. The two that were open (host networking or a published port, and whether the watcher fast-forwards the
checkout) were answered on 2026-11-14 and are recorded under Decisions above.

## Done when

The image deploys to the potato, an update from Settings succeeds, a deliberately failed one rolls back, the
Meridian tile shows, and the operator has run the live check.
