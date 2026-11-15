#!/usr/bin/env bash
#
# Sepia's nightly backup. It runs on the potato, never inside the container: the container is distroless, it has no
# rclone and no sqlite3, and the R2 token must not enter it (docs/slices/S14-backups.md, Decisions).
#
# Triggered by sepia-backup.timer at 03:30 and by sepia-backup.path when the app writes update-signal/backup-requested.
# The whole sequence is: an online SQLite snapshot into data/backups/db/, an integrity check on that snapshot, a
# weekly copy kept on Mondays, rotation, then `rclone sync` of the snapshots and the covers to R2. One JSON receipt
# per mirror is appended to data/backups/receipts.jsonl, which the app imports into the mirrors table and shows on
# the Backups screen.
#
# Everything is bounded: sqlite3 and rclone each run under `timeout`, rclone's deletion is capped, receipt text is
# truncated, and a lock keeps a slow run from stacking. A failed mirror is a warning, never a lost review: the
# snapshot is local before anything leaves the board (invariant 3).
set -euo pipefail

umask 077

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# The checkout is two levels above this script (compose/backup/). SEPIA_INSTALL_DIR exists for the tests, which copy
# this script into a temporary tree rather than run it against the repository.
INSTALL_DIR="${SEPIA_INSTALL_DIR:-$(cd "$SCRIPT_DIR/../.." && pwd)}"
DATA_DIR="${SEPIA_DATA_DIR:-$INSTALL_DIR/data}"
SIGNAL_DIR="${SEPIA_SIGNAL_DIR:-$INSTALL_DIR/update-signal}"
DB_FILE="$DATA_DIR/sepia.db"
COVERS_DIR="$DATA_DIR/covers"
BACKUP_DIR="$DATA_DIR/backups"
DAILY_DIR="$BACKUP_DIR/db"
RECEIPTS_FILE="$BACKUP_DIR/receipts.jsonl"
HEARTBEAT_FILE="$BACKUP_DIR/heartbeat"
REQUEST_FILE="$SIGNAL_DIR/backup-requested"
LOCK_FILE="$INSTALL_DIR/.backup-watcher.lock"
# The remote is a bucket-scoped token in its own rclone remote: r2: is Vaultwarden-only and must not be touched.
REMOTE="${SEPIA_BACKUP_REMOTE:-r2sepia:sepia-backups}"

# Retention (decisions, 2026-11-15): a week of dailies, four weeklies. A week of dailies alone loses a mistake found
# on the eighth day, which is what the weeklies are for.
KEEP_DAILY="${SEPIA_BACKUP_KEEP_DAILY:-7}"
KEEP_WEEKLY="${SEPIA_BACKUP_KEEP_WEEKLY:-4}"

# Time bounds. rclone's is the transfer; sqlite3's is the online backup of a database that is being written to.
RCLONE_TIMEOUT="${SEPIA_BACKUP_RCLONE_TIMEOUT:-600}"
SQLITE_TIMEOUT="${SEPIA_BACKUP_SQLITE_TIMEOUT:-120}"
# A sync that would delete more than this many remote objects is refused rather than obeyed: rotation deletes one
# or two a night, so a bigger number means the local directory was lost, not that the policy changed.
MAX_DELETE="${SEPIA_BACKUP_MAX_DELETE:-20}"

# Contract bounds (api/openapi.yaml): detail is at most 1000 characters and target at most 200.
MAX_DETAIL=1000
MAX_TARGET=200

log() { printf '%s backup.sh: %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*" >&2; }

now() { date -u +%Y-%m-%dT%H:%M:%SZ; }

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)-$$"

# json_escape escapes the two characters that break a JSON string and drops newlines and control characters. Every
# value written below is built here, from a status word, our own sentences, a path or truncated command output, so
# this does not need the full JSON escape table.
json_escape() {
	printf '%s' "$1" | tr -d '\000-\010\013\014\016-\037\n\r' | sed 's/\\/\\\\/g; s/"/\\"/g'
}

# truncate_detail keeps failure text readable and inside the contract's limit. Only the tail matters: the last lines
# of an rclone or sqlite3 failure are the error.
truncate_detail() {
	local text="$1" max="${2:-$MAX_DETAIL}"
	if [ "${#text}" -gt "$max" ]; then
		printf '...%s' "${text: -$max}"
	else
		printf '%s' "$text"
	fi
}

# dir_bytes is the size the screen shows: block usage in kilobytes, rounded up by the filesystem, which is honest
# enough for "4.2 MB" and identical on the board and in a test. A missing directory is zero rather than an error.
dir_bytes() {
	local dir="$1"
	if [ ! -d "$dir" ]; then
		printf '0'
		return 0
	fi
	du -sk "$dir" 2>/dev/null | awk '{ printf "%d", $1 * 1024 }'
}

# record writes one receipt line. Appends are the only writes to this file, so a crash costs at most the line that
# was in flight, and the app imports the whole file idempotently.
record() {
	local target="$1" status="$2" bytes="$3" detail="$4"
	target="$(truncate_detail "$target" "$MAX_TARGET")"
	detail="$(truncate_detail "$detail" "$MAX_DETAIL")"
	printf '{"run":"%s","ran_at":"%s","target":"%s","status":"%s","bytes":%s,"detail":"%s"}\n' \
		"$(json_escape "$RUN_ID")" "$(now)" "$(json_escape "$target")" "$status" "$bytes" \
		"$(json_escape "$detail")" >>"$RECEIPTS_FILE"
	log "receipt $status target=$target bytes=$bytes"
}

# row_counts is the fingerprint a reader can compare across a snapshot and the source database: entries, revisions,
# media and sessions, in that order. Four counts rather than a hash, because it survives a VACUUM and names what
# differs.
row_counts() {
	local db="$1"
	sqlite3 -readonly "$db" \
		"SELECT (SELECT count(*) FROM entries) || ' ' || (SELECT count(*) FROM entry_revisions) ||
		        ' ' || (SELECT count(*) FROM media) || ' ' || (SELECT count(*) FROM sessions);" 2>/dev/null
}

# take_snapshot writes today's snapshot through a temp file, proves it, and only then renames it into place. A failed
# check leaves yesterday's snapshot untouched: a corrupt snapshot must never replace a good one.
take_snapshot() {
	local today tmp check
	today="$DAILY_DIR/sepia-$(date -u +%Y-%m-%d).db"
	tmp="$today.tmp.$$"

	if [ ! -f "$DB_FILE" ]; then
		record "local:snapshot" failed 0 "no database at $DB_FILE"
		return 1
	fi

	# A killed run leaves its temp snapshot and sidecars behind; they are ours and nothing else names them.
	find "$DAILY_DIR" -maxdepth 1 -name '*.tmp.*' -mmin +60 -delete 2>/dev/null || true
	rm -f "$DAILY_DIR"/*.tmp.*-wal "$DAILY_DIR"/*.tmp.*-shm 2>/dev/null || true

	# .backup is the online backup API: one consistent read transaction against a database another process is
	# writing to, which a plain file copy cannot promise under WAL.
	if ! timeout "$SQLITE_TIMEOUT" sqlite3 "$DB_FILE" ".backup '$tmp'" 2>/dev/null; then
		rm -f "$tmp" "$tmp-wal" "$tmp-shm"
		record "local:snapshot" failed 0 "the online snapshot did not finish inside ${SQLITE_TIMEOUT}s"
		return 1
	fi

	# The backup inherits the source's WAL journal mode, and an open of a WAL database creates -wal and -shm beside it.
	# A snapshot must be one file, or rclone mirrors the sidecars as if they were backups (found on the potato,
	# 2026-11-15), so collapse the journal into the file before anything checks or copies it.
	timeout "$SQLITE_TIMEOUT" sqlite3 "$tmp" 'PRAGMA journal_mode=DELETE;' >/dev/null 2>&1 || true
	rm -f "$tmp-wal" "$tmp-shm"

	# Read-only, so the check cannot create the sidecars it just got rid of.
	check="$(timeout "$SQLITE_TIMEOUT" sqlite3 -readonly "$tmp" 'PRAGMA integrity_check;' 2>/dev/null || true)"
	if [ "$check" != "ok" ]; then
		rm -f "$tmp" "$tmp-wal" "$tmp-shm"
		record "local:snapshot" failed 0 "a snapshot failed its integrity check and was thrown away"
		return 1
	fi

	local source_counts snapshot_counts
	source_counts="$(row_counts "$DB_FILE" || true)"
	snapshot_counts="$(row_counts "$tmp" || true)"
	# A review saved between the snapshot and this read explains fewer rows in the snapshot, and that is fine. More
	# rows than the source cannot happen, so it is treated as a bad snapshot.
	if [ -n "$source_counts" ] && [ -n "$snapshot_counts" ] &&
		! awk -v source="$source_counts" -v snapshot="$snapshot_counts" 'BEGIN {
			n = split(snapshot, s, " ")
			m = split(source, l, " ")
			if (m < n) { exit 1 }
			for (i = 1; i <= n; i++) { if (s[i] + 0 > l[i] + 0) { exit 1 } }
			exit 0
		}'; then
		rm -f "$tmp" "$tmp-wal" "$tmp-shm"
		record "local:snapshot" failed 0 "a snapshot held more rows than the database and was thrown away"
		return 1
	fi

	mv -f "$tmp" "$today"
	rm -f "$today-wal" "$today-shm"
	chmod 600 "$today"
	log "snapshot $today ($(wc -c <"$today") bytes, $snapshot_counts)"
	return 0
}

# keep_weekly copies today's snapshot once per ISO week, named by week rather than by day, so the four weeklies are
# four different weeks even if the board is off for a night.
keep_weekly() {
	local today week target
	today="$DAILY_DIR/sepia-$(date -u +%Y-%m-%d).db"
	week="$(date -u +%G-W%V)"
	target="$DAILY_DIR/sepia-$week.db"
	[ -f "$today" ] || return 0
	[ -f "$target" ] || cp -p "$today" "$target"
}

# prune keeps the newest N of a name pattern on disk. The pattern is ours, so a sort by name is a sort by date, and
# the file that was just written is always in the newest N by construction.
prune() {
	local pattern="$1" keep="$2"
	local files
	mapfile -t files < <(find "$DAILY_DIR" -maxdepth 1 -type f -name "$pattern" | sort)
	local count="${#files[@]}"
	if [ "$count" -le "$keep" ]; then
		return 0
	fi
	local i=0
	while [ "$i" -lt $((count - keep)) ]; do
		rm -f "${files[$i]}"
		i=$((i + 1))
	done
	log "pruned $((count - keep)) old $pattern file(s), keeping $keep"
}

# mirror syncs one directory to the remote and records the outcome. It returns a failure so the run is marked failed,
# but the caller keeps going: one unreachable target must not cost the receipt of the other.
mirror() {
	local source="$1" target="$2" label="$3"
	if [ ! -d "$source" ]; then
		record "$target" skipped 0 "nothing to mirror: $label has no directory yet"
		return 0
	fi
	local bytes out rc=0
	bytes="$(dir_bytes "$source")"
	out="$(mktemp)"
	timeout "$RCLONE_TIMEOUT" rclone sync "$source" "$target" \
		--checksum --stats 0 --log-level ERROR --max-delete "$MAX_DELETE" >"$out" 2>&1 || rc=$?
	if [ "$rc" -ne 0 ]; then
		record "$target" failed "$bytes" "rclone exited $rc: $(tail -c 2000 "$out")"
		rm -f "$out"
		return 1
	fi
	# A sync that exited zero is not yet a mirror: the read-back is what a partial upload or a truncated object looks
	# like from here, and only a clean read-back earns the ok.
	rc=0
	timeout "$RCLONE_TIMEOUT" rclone check "$source" "$target" --one-way --log-level ERROR >"$out" 2>&1 || rc=$?
	if [ "$rc" -ne 0 ]; then
		record "$target" failed "$bytes" "the mirror did not read back clean: $(tail -c 2000 "$out")"
	else
		record "$target" ok "$bytes" ""
	fi
	rm -f "$out"
	[ "$rc" -eq 0 ] || return 1
	return 0
}

main() {
	mkdir -p "$DAILY_DIR" "$(dirname "$RECEIPTS_FILE")"

	# The heartbeat is the run itself: the app treats a beat older than a day as "the timer has stopped", which is a
	# different sentence from "never installed, no beat at all".
	date -u +%Y-%m-%dT%H:%M:%SZ >"$HEARTBEAT_FILE.tmp.$$"
	mv -f "$HEARTBEAT_FILE.tmp.$$" "$HEARTBEAT_FILE"

	# A second trigger while a run is in flight is a no-op, not a queued duplicate.
	exec 200>"$LOCK_FILE"
	if ! flock -n 200; then
		log "a backup is already running; this run is skipped"
		return 0
	fi

	local failed=0
	if take_snapshot; then
		keep_weekly
		prune 'sepia-????-??-??.db' "$KEEP_DAILY"
		prune 'sepia-????-W??.db' "$KEEP_WEEKLY"
	else
		failed=1
	fi

	# The mirror runs even when the snapshot failed: last night's verified snapshot is still worth copying, and the
	# failure is already recorded against the target.
	mirror "$DAILY_DIR" "$REMOTE/db" "the snapshots" || failed=1
	mirror "$COVERS_DIR" "$REMOTE/covers" "the covers" || failed=1

	# The request is cleared whatever the outcome, so the app stops saying "pending" and reads the receipts instead.
	rm -f "$REQUEST_FILE"
	return "$failed"
}

main "$@"
