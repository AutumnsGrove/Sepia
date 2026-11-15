#!/usr/bin/env bash
#
# Sepia's update watcher. It runs on the potato, never inside the container: the container has no Docker socket,
# it only writes a request into update-signal/ (docs/UPDATE.md). This script is the other side of that handshake.
#
# Triggered by sepia-update.path the moment update-signal/requested appears, and by sepia-update.timer hourly as a
# backstop: a leftover request is retried, and otherwise the timer refreshes available.json without installing
# anything. sepia-heartbeat.timer runs it with --heartbeat every minute so the app can tell "no updater installed"
# from "the updater has gone quiet".
#
# Everything here is bounded. A stalled git remote, registry or Docker daemon must end as a recorded failure, not
# as a lock held forever and a panel that says "Updating" for the rest of the week.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# The checkout is two levels above this script (compose/watcher/). SEPIA_INSTALL_DIR exists for the tests, which
# copy this script into a temporary tree rather than run it against the repository.
INSTALL_DIR="${SEPIA_INSTALL_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
ENV_FILE="$INSTALL_DIR/.env"
SIGNAL_DIR="${SEPIA_UPDATE_DIR:-$INSTALL_DIR/update-signal}"
REQUESTED_FILE="$SIGNAL_DIR/requested"
# The app writes last-request too; the watcher never removes it, so a result can be compared with the request that
# produced it even after requested is gone.
HEARTBEAT_FILE="$SIGNAL_DIR/heartbeat"
AVAILABLE_FILE="$SIGNAL_DIR/available"
RESULT_FILE="$SIGNAL_DIR/result"
LOCK_FILE="$INSTALL_DIR/.update-watcher.lock"
CMD_LOG="$SIGNAL_DIR/.last-command.log"
HEALTH_URL="${SEPIA_HEALTH_URL:-http://127.0.0.1:8893/healthz}"

# Contract bounds (api/openapi.yaml): detail is at most 1000 characters and target at most 200.
MAX_DETAIL=1000
MAX_TARGET=200

# The health poll is bounded and its pace is settable, because a test must not wait two minutes to see a failure.
HEALTH_TRIES="${SEPIA_HEALTH_TRIES:-40}"
HEALTH_DELAY="${SEPIA_HEALTH_DELAY:-3}"

log() { printf '%s update.sh: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

# json_escape escapes the two characters that break a JSON string and drops newlines and control characters.
# Every value written below is built here, from a status word, our own sentences, a digest reference or truncated
# command output, so this does not need the full JSON escape table.
json_escape() {
	printf '%s' "$1" | tr -d '\000-\010\013\014\016-\037\n\r' | sed 's/\\/\\\\/g; s/"/\\"/g'
}

# truncate_detail keeps the failure text readable and inside the contract's limit. Only the tail matters: the last
# lines of a docker failure are the error.
truncate_detail() {
	local text="$1" max="${2:-$MAX_DETAIL}"
	if [ "${#text}" -gt "$max" ]; then
		printf '...%s' "${text: -$max}"
	else
		printf '%s' "$text"
	fi
}

# write_atomically writes a file the app may read at any moment: a rename is the only way to publish content the
# reader cannot catch half-written.
write_atomically() {
	local target="$1" body="$2"
	local tmp="$target.tmp.$$"
	printf '%s\n' "$body" >"$tmp"
	mv -f "$tmp" "$target"
}

write_heartbeat() {
	write_atomically "$HEARTBEAT_FILE" "$(now)"
}

# write_result is the update's outcome. status is ok, failed or skipped; detail is plain text for the panel.
write_result() {
	local status="$1" detail="$2" target="${3:-}"
	detail="$(truncate_detail "$detail")"
	target="$(truncate_detail "$target" "$MAX_TARGET")"
	write_atomically "$RESULT_FILE" "$(printf '{"status":"%s","detail":"%s","target":"%s","finished_at":"%s"}' \
		"$(json_escape "$status")" "$(json_escape "$detail")" "$(json_escape "$target")" "$(now)")"
}

# write_available records what the last check found. current is the app's own version, which the app fills in when
# it serves the response; the watcher only knows what GHCR holds and what .env has pinned.
write_available() {
	local latest="$1" digest="$2" update="$3" error="${4:-}"
	if [ -n "$error" ]; then
		write_atomically "$AVAILABLE_FILE" "$(printf '{"checked_at":"%s","update_available":false,"error":"%s"}' \
			"$(now)" "$(json_escape "$(truncate_detail "$error" 300)")")"
		return
	fi
	write_atomically "$AVAILABLE_FILE" "$(printf '{"latest":"%s","latest_digest":"%s","checked_at":"%s","update_available":%s}' \
		"$(json_escape "$latest")" "$(json_escape "$digest")" "$(now)" "$update")"
}

current_pinned_image() {
	grep '^SEPIA_IMAGE=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2- || true
}

# set_pinned_image rewrites the one line the compose file interpolates, preserving the file's mode. A temp file and
# a rename mean a crash never leaves .env half written.
set_pinned_image() {
	local value="$1"
	local tmp
	tmp="$(mktemp "$ENV_FILE.XXXXXX")"
	chmod 600 "$tmp"
	grep -v '^SEPIA_IMAGE=' "$ENV_FILE" >"$tmp" || true
	printf 'SEPIA_IMAGE=%s\n' "$value" >>"$tmp"
	mv -f "$tmp" "$ENV_FILE"
}

# image_repo is the image without a tag or digest: ghcr.io/autumnsgrove/sepia.
image_repo() {
	local ref="$1"
	ref="${ref%@*}"
	ref="${ref%:*}"
	printf '%s' "$ref"
}

valid_ref() {
	local ref="$1" repo="$2"
	[ -n "$repo" ] && [[ "$ref" == "$repo@sha256:"* ]] && [[ "${ref##*@}" =~ ^sha256:[0-9a-f]{64}$ ]]
}

# resolve_latest asks GHCR for the digest behind :latest and the version annotation the publish workflow puts on
# the index. It sets RESOLVED_REF and RESOLVED_VERSION, or returns non-zero with RESOLVE_ERROR set.
resolve_latest() {
	RESOLVED_REF=""
	RESOLVED_VERSION=""
	RESOLVE_ERROR=""

	local image repo slug token digest body version
	image="$(current_pinned_image)"
	if [ -z "$image" ]; then
		RESOLVE_ERROR="SEPIA_IMAGE is not set in .env, so there is nothing to compare against."
		return 1
	fi
	repo="$(image_repo "$image")"
	if [[ "$repo" != ghcr.io/* ]]; then
		RESOLVE_ERROR="SEPIA_IMAGE does not name a GHCR image: $repo"
		return 1
	fi
	slug="${repo#ghcr.io/}"

	if ! token="$(timeout 20 curl -fsS --max-time 20 \
		"https://ghcr.io/token?service=ghcr.io&scope=repository:${slug}:pull" 2>"$CMD_LOG")"; then
		RESOLVE_ERROR="GHCR is unreachable: $(truncate_detail "$(cat "$CMD_LOG")" 300)"
		return 1
	fi
	token="$(printf '%s' "$token" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')"
	if [ -z "$token" ]; then
		RESOLVE_ERROR="GHCR returned no token; the image may not be public."
		return 1
	fi

	if ! digest="$(timeout 20 curl -fsSI --max-time 20 \
		-H "Authorization: Bearer $token" \
		-H 'Accept: application/vnd.oci.image.index.v1+json' \
		"https://ghcr.io/v2/${slug}/manifests/latest" 2>"$CMD_LOG" |
		tr -d '\r' | sed -n 's/^[Dd]ocker-[Cc]ontent-[Dd]igest: *//p' | head -1)"; then
		RESOLVE_ERROR="Could not read the latest digest: $(truncate_detail "$(cat "$CMD_LOG")" 300)"
		return 1
	fi
	if ! [[ "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
		RESOLVE_ERROR="GHCR answered without a usable digest."
		return 1
	fi

	# The annotation is a nicety: without it the panel names the build by its short digest instead of its version.
	body="$(timeout 20 curl -fsS --max-time 20 \
		-H "Authorization: Bearer $token" \
		-H 'Accept: application/vnd.oci.image.index.v1+json' \
		"https://ghcr.io/v2/${slug}/manifests/latest" 2>/dev/null || true)"
	version="$(printf '%s' "$body" | sed -n 's/.*"org\.opencontainers\.image\.version":"\([^"]*\)".*/\1/p' | head -1)"

	RESOLVED_REF="${repo}@${digest}"
	RESOLVED_VERSION="${version:-sha256:${digest#sha256:}}"
	RESOLVED_VERSION="${RESOLVED_VERSION:0:40}"
	return 0
}

do_check() {
	if ! resolve_latest; then
		log "check failed: $RESOLVE_ERROR"
		write_available "" "" false "$RESOLVE_ERROR"
		return 1
	fi
	local current update=false
	current="$(current_pinned_image)"
	if [ "$current" != "$RESOLVED_REF" ]; then
		update=true
	fi
	write_available "$RESOLVED_VERSION" "$RESOLVED_REF" "$update"
	log "check: $RESOLVED_VERSION at $RESOLVED_REF, update available: $update"
}

# wait_healthy polls the service's own health check. The app is up only when it answers by itself; a running
# container is not the same thing.
wait_healthy() {
	local tries="${1:-$HEALTH_TRIES}" delay="${2:-$HEALTH_DELAY}" i
	for ((i = 0; i < tries; i++)); do
		if timeout 6 curl -fsS --max-time 5 "$HEALTH_URL" 2>/dev/null | grep -q '"status":"ok"'; then
			return 0
		fi
		sleep "$delay"
	done
	return 1
}

# rollback re-pins the previous image and tries to bring it back. A false return means the board may be down, and
# the result says so in its own words rather than pretending the old version returned.
rollback() {
	local previous="$1"
	if [ -z "$previous" ]; then
		return 1
	fi
	set_pinned_image "$previous"
	if ! timeout 60 docker compose up -d --force-recreate --no-deps sepia 2>&1 | tee "$CMD_LOG"; then
		return 1
	fi
	wait_healthy
}

do_update() {
	local target="$1" previous details
	if ! valid_ref "$target" "$(image_repo "$(current_pinned_image)")"; then
		write_result failed "That is not a digest of this image, so it was refused." "$target"
		return 1
	fi
	previous="$(current_pinned_image)"
	if [ "$previous" = "$target" ]; then
		write_result skipped "That version is already installed." "$target"
		return 0
	fi

	# The image alone is not the deployment: the compose file, the config tree and this script all live in the
	# checkout, so a release has to land there too. --ff-only refuses a diverged checkout rather than inventing a
	# merge on the board.
	if ! timeout 60 git -C "$INSTALL_DIR" pull --ff-only origin main 2>&1 | tee "$CMD_LOG"; then
		write_result failed "Could not update the checkout: $(truncate_detail "$(cat "$CMD_LOG")")" "$target"
		return 1
	fi

	set_pinned_image "$target"
	if ! timeout 300 docker compose pull sepia 2>&1 | tee "$CMD_LOG"; then
		set_pinned_image "$previous"
		write_result failed "Could not pull the image: $(truncate_detail "$(cat "$CMD_LOG")")" "$target"
		return 1
	fi

	if ! timeout 60 docker compose up -d --force-recreate --no-deps sepia 2>&1 | tee "$CMD_LOG"; then
		details="Sepia did not start: $(truncate_detail "$(cat "$CMD_LOG")")"
		if rollback "$previous"; then
			write_result failed "$details The previous version is running again." "$target"
		else
			write_result failed "$details The previous version did not come back either; Sepia may be down." "$target"
		fi
		return 1
	fi

	if ! wait_healthy; then
		details="$(timeout 20 docker compose logs --no-color --tail=30 sepia 2>&1 || true)"
		details="The new version did not pass its health check: $(truncate_detail "$details" 500)"
		if rollback "$previous"; then
			write_result failed "$details The previous version is running again." "$target"
		else
			write_result failed "$details The previous version did not come back either; Sepia may be down." "$target"
		fi
		return 1
	fi

	write_result ok "Updated to $(basename "$target")." "$target"
	log "updated to $target"
}

do_restart() {
	if ! timeout 60 docker compose up -d --force-recreate --no-deps sepia 2>&1 | tee "$CMD_LOG"; then
		write_result failed "Sepia did not restart: $(truncate_detail "$(cat "$CMD_LOG")")" "$(current_pinned_image)"
		return 1
	fi
	if ! wait_healthy; then
		write_result failed "Sepia restarted but did not answer its health check." "$(current_pinned_image)"
		return 1
	fi
	write_result ok "Restarted." "$(current_pinned_image)"
}

# read_request fills REQUEST_ACTION and REQUEST_TARGET from the key=value file, ignoring anything else. A malformed
# file is a failed request, not a reason to guess.
read_request() {
	REQUEST_ACTION=""
	REQUEST_TARGET=""
	local key value
	while IFS='=' read -r key value || [ -n "$key" ]; do
		case "$key" in
		action) REQUEST_ACTION="$value" ;;
		target) REQUEST_TARGET="$value" ;;
		esac
	done <"$REQUESTED_FILE"
	[[ "$REQUEST_ACTION" =~ ^(check|update|restart)$ ]]
}

main() {
	case "${1:-}" in
	--heartbeat)
		mkdir -p "$SIGNAL_DIR"
		write_heartbeat
		return 0
		;;
	"" | --check) ;;
	*)
		log "unknown argument: $1"
		return 2
		;;
	esac

	mkdir -p "$SIGNAL_DIR"
	cd "$INSTALL_DIR"
	write_heartbeat

	# A second trigger while an update is in flight is a no-op, not a queued duplicate.
	exec 200>"$LOCK_FILE"
	if ! flock -n 200; then
		log "an update is already running; this run is skipped"
		return 0
	fi

	if [ ! -f "$REQUESTED_FILE" ]; then
		# The timer's backstop with nothing waiting: refresh what is available and stop. The path unit never
		# reaches here, because it fires only when the file exists.
		if ! do_check; then
			write_heartbeat
			return 1
		fi
		write_heartbeat
		return 0
	fi

	if ! read_request; then
		rm -f "$REQUESTED_FILE"
		write_result failed "The update request could not be read." ""
		write_heartbeat
		return 1
	fi

	log "request: $REQUEST_ACTION${REQUEST_TARGET:+ $REQUEST_TARGET}"
	# A failed run exits non-zero so systemd records it as failed and the journal has the reason; the request is
	# still cleared and the heartbeat still written, because leaving either behind is worse than the failure.
	local status=0
	case "$REQUEST_ACTION" in
	check) do_check || status=1 ;;
	update)
		if [ -z "$REQUEST_TARGET" ]; then
			if resolve_latest; then
				REQUEST_TARGET="$RESOLVED_REF"
			else
				write_result failed "$RESOLVE_ERROR" ""
				rm -f "$REQUESTED_FILE"
				write_heartbeat
				return 1
			fi
		fi
		do_update "$REQUEST_TARGET" || status=1
		;;
	restart) do_restart || status=1 ;;
	esac
	rm -f "$REQUESTED_FILE"
	write_heartbeat
	return "$status"
}

main "$@"
