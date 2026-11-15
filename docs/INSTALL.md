# Installing Sepia on the potato

Sepia runs as one container on the Le Potato, listens on `127.0.0.1:8893` only, and updates itself through a host
watcher (docs/UPDATE.md). This is the whole install. The operator's live checks come before the first production
deploy (gate G5, docs/BUILD.md 6).

Anything that fails here has a line in `/var/log` or the journal, and `sepia doctor` below is the first thing to run.

## 0. What this expects

- The potato: Armbian trixie, Docker 29 with Compose v2, systemd, git, curl, and autumn in the `docker` group.
- `tailscale` logged in, with the tailnet name `lepotato.taild95077.ts.net` (the Host check allows it).
- Port 8893 free. 558 MiB of swap and 1.9 GiB of RAM are the constraint: **the board never builds the image.** CI
  publishes it and the board pulls it.

## 1. The checkout

```bash
git clone https://github.com/AutumnsGrove/sepia "$HOME/Sepia"
cd "$HOME/Sepia"
mkdir -p data update-signal
chmod 700 data
chmod 750 update-signal
```

`data/` holds SQLite, covers and exports; it is hers and nobody else's. `update-signal/` is the handshake with the
host watcher. Both are in `.gitignore` and must never be committed.

The checkout must stay on `main` and be cloneable over HTTPS: the watcher fast-forwards it on every update.

## 2. The environment file

```bash
cd "$HOME/Sepia"
cp compose/sepia.env.example .env
$EDITOR .env            # paste SEPIA_IMAGE, SEPIA_UID, SEPIA_GID and the two keys
chmod 600 .env
```

The image line comes from the `docker-publish` run's summary, which prints the pinned reference. `SEPIA_UID` and
`SEPIA_GID` are `id -u` and `id -g` on the potato (1000 and 1000 for autumn).

`sepia doctor` reports each line only as present, empty or missing. No agent reads the values, and nothing in this
repository ever contains one.

## 3. The first start

```bash
cd "$HOME/Sepia"
docker compose pull
docker compose up -d
curl -s http://localhost:8893/healthz
```

The answer is exactly `{"status":"ok","version":"r...."}`. If it is a refusal, the container's log names the config
key that is wrong: `docker compose logs --tail=50 sepia`.

`tailscale serve` is the only way in:

```bash
sudo tailscale serve --bg 8893
tailscale serve status | grep 8893
```

The address is `https://lepotato.taild95077.ts.net:8893`.

## 4. The update watcher

Five unit files, two placeholders. `@INSTALL_DIR@` is `$HOME/Sepia`; `@USER@` is `autumn`.

```bash
cd "$HOME/Sepia"
INSTALL_DIR="$HOME/Sepia"
USER_NAME="$(id -un)"
for f in compose/systemd/sepia-*.path compose/systemd/sepia-*.service compose/systemd/sepia-*.timer; do
  sed -e "s|@INSTALL_DIR@|$INSTALL_DIR|g" -e "s|@USER@|$USER_NAME|g" "$f" |
    sudo tee "/etc/systemd/system/$(basename "$f")" >/dev/null
done
sudo systemctl daemon-reload
sudo systemctl enable --now sepia-update.path sepia-update.timer sepia-heartbeat.timer
```

Check it without waiting for a release:

```bash
systemctl start sepia-update.service        # a check; installs nothing
cat "$HOME/Sepia/update-signal/available"  # what it found
cat "$HOME/Sepia/update-signal/heartbeat"  # must be under three minutes old
journalctl -u sepia-update.service -n 30 --no-pager
```

Settings → Updates should now say the helper is running, with the version it found.

## 5. The doctor

```bash
cd "$HOME/Sepia"
docker compose exec sepia /sepia doctor
```

It checks the bound address is loopback, the data directory is writable, `.env` is mode 0600 and owned by the
container's uid, the keys are present (never their values), the database's protections are present **and** refuse a
forbidden write, and whether the helper is beating. Exit 1 means something is broken; a warning means a feature is
off, which is normal for a missing key.

## 6. Meridian and the health check

Both live in other repositories, and changing them is the operator's. The lines are below.

`~/Documents/Projects/homelab/maintenance-setup/service-healthcheck.sh`, beside the Warbler block:

```bash
# Sepia: loopback listener behind tailscale serve, and /healthz carries only status + version. Host networking,
# so localhost is the box; the Host check allows localhost:8893.
check_docker_health "Sepia" "sepia" "sepia"
check_http "Sepia" "http://localhost:8893/healthz"
```

Meridian's tile (`examples/homelab.yaml` here, the live config on the board):

```yaml
  - id: sepia
    name: Sepia
    section: everyday
    icon: book-open
    description: private review journal
    url: https://lepotato.taild95077.ts.net:8893
    probes:
      health:
        kind: http_json
        url: http://localhost:8893/healthz
        path: status
        equals: ok
      vitals:
        - {kind: json_field, label: build, url: "http://localhost:8893/healthz", path: version}
    control:
      target: sepia
      actions: [restart, update]
```

Meridian's `update` action writes `action=update` into `~/Sepia/update-signal/requested`, which the watcher already
watches. A "reviews this month" vital is **not** available: `/healthz` is two fields by design and the entries list
has no month filter, so a vital would need a new endpoint (see the slice's handoff).

## 7. The gate: a failed update on a scratch copy

The first production deploy waits for G5: an update and a deliberately failed one with rollback, on a scratch
container. The scratch install is its own checkout, its own port, its own signal directory and its own units; the
production install is never touched.

```bash
git clone https://github.com/AutumnsGrove/sepia "$HOME/Sepia-scratch"
cd "$HOME/Sepia-scratch"
mkdir -p data update-signal
cp "$HOME/Sepia/.env" .env
sed -i 's/^SEPIA_IMAGE=.*/SEPIA_IMAGE=<the same pinned reference>/' .env
cat > docker-compose.override.yml <<'YAML'
# The scratch copy listens on 8897 and keeps its own signal files. Nothing here is for the production install.
services:
  sepia:
    environment:
      SEPIA_ADDR: 127.0.0.1:8897
      SEPIA_ALLOWED_HOSTS: localhost:8897,127.0.0.1:8897
      SEPIA_UPDATE_DIR: /update-signal
YAML
docker compose up -d
curl -s http://localhost:8897/healthz
```

Units for the scratch copy: the same five files with `sepia-` renamed to `sepia-scratch-`, the scratch install
directory, and `SEPIA_HEALTH_URL=http://127.0.0.1:8897/healthz` added under `[Service]` in the update service.

```bash
INSTALL_DIR="$HOME/Sepia-scratch"
USER_NAME="$(id -un)"
for f in compose/systemd/*; do
  sed -e "s|@INSTALL_DIR@|$INSTALL_DIR|g" -e "s|@USER@|$USER_NAME|g" \
      -e 's|sepia-|sepia-scratch-|g' -e 's|8893|8897|g' "$f" |
    sudo tee "/etc/systemd/system/$(basename "$f" | sed 's/^sepia-/sepia-scratch-/')" >/dev/null
done
sudo systemctl daemon-reload
sudo systemctl enable --now sepia-scratch-update.path sepia-scratch-update.timer sepia-scratch-heartbeat.timer
```

### A. An update that works

Press Update in the scratch Settings, or write the request by hand:

```bash
printf 'action=update\n' > "$HOME/Sepia-scratch/update-signal/requested"
systemctl start sepia-scratch-update.service
cat "$HOME/Sepia-scratch/update-signal/result"   # {"status":"ok",...}
```

### B. An update that fails its health check and rolls back

Make the **next** container fail to serve, without touching the running one:

```bash
cd "$HOME/Sepia-scratch"
cat >> docker-compose.override.yml <<'YAML'
    entrypoint: ["/bin/false"]
YAML
docker compose config -q                     # the override is valid
printf 'action=update\n' > update-signal/requested
systemctl start sepia-scratch-update.service
```

The new container exits immediately, the health poll fails, and the watcher re-pins the previous digest and recreates
it. The old container also carries the override, so it also fails: that is the **rollback-also-failed** result, and
Settings → Updates says "Sepia may not be running properly. Check the potato" with no retry button.

To watch the **successful rollback** result instead, remove the `entrypoint` line while the watcher is polling (up to
two minutes), so the old image starts healthy when the rollback runs:

```bash
sed -i '/entrypoint:/d' docker-compose.override.yml
# wait for the result
cat update-signal/result
```

Either way, recover with `docker compose up -d` after removing the override.

The watcher's own routes are covered against scripted fakes in `internal/archtest/watcher_test.go`; this recipe is the
live proof on the board, and its outcome goes in the G5 handoff.

## 8. Rolling back by hand

```bash
cd "$HOME/Sepia"
grep '^SEPIA_IMAGE=' .env        # the pinned digest
docker image ls | grep sepia     # what is on the board
```

The watcher's rollback is the normal path. By hand: set `SEPIA_IMAGE` to the previous digest, `docker compose pull`,
`docker compose up -d --force-recreate --no-deps sepia`, then check `/healthz`. Nothing in `data/` is affected by
either path.
