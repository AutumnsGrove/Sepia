package archtest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The backup script is shell, but it is the piece that decides whether her words survive a dead board, so it is
// tested like code: a temporary install tree, a scripted fake rclone and sqlite3, and the routes the slice names.
// The fakes are files on PATH, the same way the update watcher's are, so a failure in the field can be reproduced
// by hand. One test deliberately uses the real sqlite3 with the real migrations, because a fake cannot prove that
// `.backup` and `PRAGMA integrity_check` agree with the script's idea of them.

type backupTree struct {
	root   string
	data   string
	signal string
	bin    string
	log    string
}

// newBackupTree builds a tree shaped like the potato's: the script under compose/backup, data/ holding the database
// and covers, a signal directory, and a bin/ of fakes first on PATH.
func newBackupTree(t *testing.T, fakes bool) *backupTree {
	t.Helper()
	root := t.TempDir()
	b := &backupTree{
		root:   root,
		data:   filepath.Join(root, "data"),
		signal: filepath.Join(root, "update-signal"),
		bin:    filepath.Join(root, "bin"),
		log:    filepath.Join(root, "fake.log"),
	}
	for _, dir := range []string{
		filepath.Join(root, "compose", "backup"), b.data, filepath.Join(b.data, "covers"),
		filepath.Join(b.data, "backups", "db"), b.signal, b.bin,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "compose", "backup", "backup.sh"))
	if err != nil {
		t.Fatalf("read backup.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose", "backup", "backup.sh"), script, 0o755); err != nil {
		t.Fatalf("copy backup.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(b.data, "sepia.db"), []byte("not really a database"), 0o600); err != nil {
		t.Fatalf("write database placeholder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(b.data, "covers", "cover-1.jpg"), []byte("jpeg bytes"), 0o600); err != nil {
		t.Fatalf("write cover: %v", err)
	}
	if fakes {
		for name, body := range backupFakes() {
			if err := os.WriteFile(filepath.Join(b.bin, name), []byte(body), 0o755); err != nil {
				t.Fatalf("write fake %s: %v", name, err)
			}
		}
	}
	return b
}

// backupFakes are the two commands the script shells out to. Both log every invocation, and both can be made to
// fail in the way the real one fails: sqlite3 by refusing the backup or reporting a corrupt snapshot, rclone by
// denying access or dropping a file between the upload and the read-back.
func backupFakes() map[string]string {
	return map[string]string{
		"sqlite3": `#!/usr/bin/env bash
printf 'sqlite3 %s\n' "$*" >>"$FAKE_LOG"
last="${!#}"
case "$last" in
	.backup*)
		if [ "${FAKE_SQLITE_BACKUP_FAIL:-0}" = "1" ]; then
			printf 'Error: disk I/O error\n' >&2
			exit 1
		fi
		target="${last#.backup }"
		target="${target%\'}"
		target="${target#\'}"
		printf 'snapshot bytes' >"$target"
		;;
	"PRAGMA integrity_check;")
		printf '%s\n' "${FAKE_SQLITE_INTEGRITY:-ok}"
		;;
	"PRAGMA journal_mode=DELETE;")
		printf '%s\n' "${FAKE_SQLITE_JOURNAL_MODE:-delete}"
		;;
	*)
		if [ "${FAKE_SQLITE_COUNTS_NO_SESSIONS:-0}" = "1" ]; then
			printf '0 0 0 0\n'
		elif [ "${FAKE_SQLITE_SNAPSHOT_MORE_ROWS:-0}" = "1" ] && [ "${last%% *}" = "SELECT" ]; then
			case "$*" in
				*.tmp.*) printf '9 9 9 9\n' ;;
				*) printf '1 1 0 1\n' ;;
			esac
		else
			printf '1 1 0 1\n'
		fi
		;;
esac
exit 0
`,
		"rclone": `#!/usr/bin/env bash
printf 'rclone %s\n' "$*" >>"$FAKE_LOG"
case "$1" in
	sync)
		if [ "${FAKE_RCLONE_SYNC_FAIL:-0}" = "1" ]; then
			printf 'ERROR : AccessDenied: Access Denied\n' >&2
			exit 3
		fi
		;;
	check)
		if [ "${FAKE_RCLONE_CHECK_FAIL:-0}" = "1" ]; then
			printf 'ERROR : 1 file missing from remote\n' >&2
			exit 1
		fi
		;;
esac
exit 0
`,
	}
}

// run executes the backup script with the fakes first on PATH and the given environment overrides.
func (b *backupTree) run(t *testing.T, extra map[string]string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "bash",
		filepath.Join(b.root, "compose", "backup", "backup.sh"))
	env := []string{
		"PATH=" + b.bin + ":" + os.Getenv("PATH"),
		"HOME=" + b.root,
		"SEPIA_INSTALL_DIR=" + b.root,
		"SEPIA_DATA_DIR=" + b.data,
		"SEPIA_SIGNAL_DIR=" + b.signal,
		"FAKE_LOG=" + b.log,
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// receipt is one JSON line from data/backups/receipts.jsonl, the file the app imports into mirrors.
type receipt struct {
	Run    string `json:"run"`
	RanAt  string `json:"ran_at"`
	Target string `json:"target"`
	Status string `json:"status"`
	Bytes  int64  `json:"bytes"`
	Detail string `json:"detail"`
}

func (b *backupTree) receipts(t *testing.T) []receipt {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(b.data, "backups", "receipts.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read receipts: %v", err)
	}
	var got []receipt
	for _, line := range strings.Split(strings.TrimSpace(string(body)), "\n") {
		if line == "" {
			continue
		}
		var r receipt
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("receipt is not JSON (%q): %v", line, err)
		}
		got = append(got, r)
	}
	return got
}

func (b *backupTree) names(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read %s: %v", dir, err)
	}
	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	sort.Strings(got)
	return got
}

func (b *backupTree) calls(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(b.log)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read fake log: %v", err)
	}
	return string(body)
}

// todaySnapshot and thisWeek are the two names the script derives from the clock, and the test derives them the
// same way: both are UTC, so the board's timezone cannot rename the file under the test.
func todaySnapshot() string {
	return "sepia-" + time.Now().UTC().Format("2006-01-02") + ".db"
}

func thisWeekSnapshot() string {
	year, week := time.Now().UTC().ISOWeek()
	return fmt.Sprintf("sepia-%d-W%02d.db", year, week)
}

func findReceipt(t *testing.T, got []receipt, target string) receipt {
	t.Helper()
	var found *receipt
	for i := range got {
		if got[i].Target == target {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("no receipt for %s in %+v", target, got)
	}
	return *found
}

// TestBackupSnapshotAndMirror is the happy route: a verified snapshot, a weekly copy, rotation, both mirrors
// recorded, the heartbeat written, and the on-demand request cleared.
func TestBackupSnapshotAndMirror(t *testing.T) {
	b := newBackupTree(t, true)
	if err := os.WriteFile(filepath.Join(b.signal, "backup-requested"), []byte("action=backup\n"), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	if out, err := b.run(t, nil); err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	info, err := os.Stat(filepath.Join(b.data, "backups", "db", todaySnapshot()))
	if err != nil {
		t.Fatalf("today's snapshot is missing: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("snapshot mode = %o, want 600", got)
	}
	if !contains(b.names(t, filepath.Join(b.data, "backups", "db")), thisWeekSnapshot()) {
		t.Errorf("weekly copy %s is missing from %v", thisWeekSnapshot(), b.names(t, filepath.Join(b.data, "backups", "db")))
	}

	got := b.receipts(t)
	if len(got) != 2 {
		t.Fatalf("receipts = %d, want 2: %+v", len(got), got)
	}
	db := findReceipt(t, got, "r2sepia:sepia-backups/db")
	if db.Status != "ok" {
		t.Errorf("db receipt = %q (%s), want ok", db.Status, db.Detail)
	}
	if db.Bytes <= 0 {
		t.Errorf("db receipt bytes = %d, want the size of what was mirrored", db.Bytes)
	}
	covers := findReceipt(t, got, "r2sepia:sepia-backups/covers")
	if covers.Status != "ok" {
		t.Errorf("covers receipt = %q (%s), want ok", covers.Status, covers.Detail)
	}
	if !strings.HasPrefix(db.Run, time.Now().UTC().Format("20060102")) {
		t.Errorf("db receipt run id = %q, want today's date", db.Run)
	}

	if _, err := os.Stat(filepath.Join(b.data, "backups", "heartbeat")); err != nil {
		t.Errorf("heartbeat is missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b.signal, "backup-requested")); !os.IsNotExist(err) {
		t.Errorf("the request file was not cleared: %v", err)
	}

	calls := b.calls(t)
	if !strings.Contains(calls, "rclone sync "+filepath.Join(b.data, "backups", "db")) {
		t.Errorf("rclone never synced the snapshots:\n%s", calls)
	}
	if !strings.Contains(calls, "--max-delete 20") {
		t.Errorf("rclone ran without the deletion cap:\n%s", calls)
	}
	if !strings.Contains(calls, "--one-way") {
		t.Errorf("rclone never read the mirror back:\n%s", calls)
	}
}

// TestBackupCorruptSnapshotIsThrownAway is the bad route that matters most: a snapshot that fails its integrity
// check must never be mirrored and must never replace the good one from yesterday.
func TestBackupCorruptSnapshotIsThrownAway(t *testing.T) {
	b := newBackupTree(t, true)
	yesterday := "sepia-2000-01-01.db"
	path := filepath.Join(b.data, "backups", "db", yesterday)
	if err := os.WriteFile(path, []byte("the good one"), 0o600); err != nil {
		t.Fatalf("seed yesterday's snapshot: %v", err)
	}

	out, err := b.run(t, map[string]string{"FAKE_SQLITE_INTEGRITY": "database disk image is malformed"})
	if err == nil {
		t.Fatalf("a corrupt snapshot did not fail the run:\n%s", out)
	}

	if _, statErr := os.Stat(filepath.Join(b.data, "backups", "db", todaySnapshot())); !os.IsNotExist(statErr) {
		t.Errorf("a corrupt snapshot was published: %v", statErr)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "the good one" {
		t.Errorf("yesterday's good snapshot was disturbed: %q, %v", body, err)
	}
	snap := findReceipt(t, b.receipts(t), "local:snapshot")
	if snap.Status != "failed" {
		t.Errorf("snapshot receipt = %q, want failed", snap.Status)
	}
	if !strings.Contains(snap.Detail, "integrity check") {
		t.Errorf("detail = %q, want it to name the integrity check", snap.Detail)
	}
	for _, name := range b.names(t, filepath.Join(b.data, "backups", "db")) {
		if strings.Contains(name, ".tmp.") {
			t.Errorf("a failed snapshot left its temp file behind: %s", name)
		}
	}
}

// TestBackupRefusesASnapshotWithTooManyRows covers the second verification: integrity alone does not prove the
// snapshot is the database, so a snapshot claiming more rows than the source is thrown away.
func TestBackupRefusesASnapshotWithTooManyRows(t *testing.T) {
	b := newBackupTree(t, true)

	out, err := b.run(t, map[string]string{"FAKE_SQLITE_SNAPSHOT_MORE_ROWS": "1"})
	if err == nil {
		t.Fatalf("a snapshot with more rows than the database did not fail the run:\n%s", out)
	}
	snap := findReceipt(t, b.receipts(t), "local:snapshot")
	if snap.Status != "failed" || !strings.Contains(snap.Detail, "more rows") {
		t.Errorf("snapshot receipt = %q (%s), want failed and naming the row count", snap.Status, snap.Detail)
	}
	if _, err := os.Stat(filepath.Join(b.data, "backups", "db", todaySnapshot())); !os.IsNotExist(err) {
		t.Errorf("a bad snapshot was published: %v", err)
	}
}

// TestBackupRefusesASnapshotThatStaysInWAL: a snapshot whose journal will not collapse would leave -wal and -shm
// beside it on the next open, and rclone would mirror those as if they were backups. It is thrown away instead.
func TestBackupRefusesASnapshotThatStaysInWAL(t *testing.T) {
	b := newBackupTree(t, true)

	out, err := b.run(t, map[string]string{"FAKE_SQLITE_JOURNAL_MODE": "wal"})
	if err == nil {
		t.Fatalf("a snapshot stuck in WAL mode did not fail the run:\n%s", out)
	}
	snap := findReceipt(t, b.receipts(t), "local:snapshot")
	if snap.Status != "failed" || !strings.Contains(snap.Detail, "collapse") {
		t.Errorf("snapshot receipt = %q (%s), want failed and naming the collapse", snap.Status, snap.Detail)
	}
	if _, statErr := os.Stat(filepath.Join(b.data, "backups", "db", todaySnapshot())); !os.IsNotExist(statErr) {
		t.Errorf("a WAL-mode snapshot was published: %v", statErr)
	}
	for _, name := range b.names(t, filepath.Join(b.data, "backups", "db")) {
		if strings.Contains(name, ".tmp.") {
			t.Errorf("a refused snapshot left %s behind", name)
		}
	}
}

// TestBackupSnapshotFailureStillMirrors is the local-first rule: when the snapshot cannot be taken, the verified
// snapshots already on disk are still copied, and the failure is recorded rather than thrown away.
func TestBackupSnapshotFailureStillMirrors(t *testing.T) {
	b := newBackupTree(t, true)
	if err := os.WriteFile(filepath.Join(b.data, "backups", "db", "sepia-2000-01-01.db"), []byte("old"), 0o600); err != nil {
		t.Fatalf("seed snapshot: %v", err)
	}

	out, err := b.run(t, map[string]string{"FAKE_SQLITE_BACKUP_FAIL": "1"})
	if err == nil {
		t.Fatalf("a failed snapshot did not fail the run:\n%s", out)
	}
	if snap := findReceipt(t, b.receipts(t), "local:snapshot"); snap.Status != "failed" {
		t.Errorf("snapshot receipt = %q, want failed", snap.Status)
	}
	if !strings.Contains(b.calls(t), "rclone sync") {
		t.Errorf("the existing snapshots were not mirrored after a failed snapshot:\n%s", b.calls(t))
	}
}

// TestBackupMirrorFailuresAreRecorded covers R2 unreachable, an auth failure and a partial upload. Each is a
// warning with a receipt, never a lost review, and the other target still runs.
func TestBackupMirrorFailuresAreRecorded(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		contains string
	}{
		{"auth failure", map[string]string{"FAKE_RCLONE_SYNC_FAIL": "1"}, "AccessDenied"},
		{"partial upload", map[string]string{"FAKE_RCLONE_CHECK_FAIL": "1"}, "read back clean"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBackupTree(t, true)
			out, err := b.run(t, tc.env)
			if err == nil {
				t.Fatalf("a failed mirror did not fail the run:\n%s", out)
			}
			got := b.receipts(t)
			db := findReceipt(t, got, "r2sepia:sepia-backups/db")
			if db.Status != "failed" {
				t.Errorf("db receipt = %q, want failed", db.Status)
			}
			if !strings.Contains(db.Detail, tc.contains) {
				t.Errorf("db detail = %q, want it to contain %q", db.Detail, tc.contains)
			}
			if covers := findReceipt(t, got, "r2sepia:sepia-backups/covers"); covers.Status != "failed" {
				t.Errorf("covers receipt = %q, want the second target attempted too", covers.Status)
			}
			if len(db.Detail) > 1000 {
				t.Errorf("detail = %d characters, want the contract's 1000", len(db.Detail))
			}
			if strings.ContainsAny(db.Detail, "\n\r") {
				t.Errorf("detail carries a newline into the receipt: %q", db.Detail)
			}
		})
	}
}

// TestBackupRotationKeepsSevenAndFour is the retention decision: a week of dailies, four weeklies, and the
// snapshot just taken is never the one pruned.
func TestBackupRotationKeepsSevenAndFour(t *testing.T) {
	b := newBackupTree(t, true)
	dir := filepath.Join(b.data, "backups", "db")
	for day := 1; day <= 9; day++ {
		name := fmt.Sprintf("sepia-2000-01-%02d.db", day)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}
	for week := 1; week <= 5; week++ {
		name := fmt.Sprintf("sepia-2000-W%02d.db", week)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("old"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	if out, err := b.run(t, nil); err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	var dailies, weeklies int
	for _, name := range b.names(t, dir) {
		if strings.Contains(name, "-W") {
			weeklies++
		} else {
			dailies++
		}
	}
	if dailies != 7 {
		t.Errorf("dailies = %d, want 7 (%v)", dailies, b.names(t, dir))
	}
	if weeklies != 4 {
		t.Errorf("weeklies = %d, want 4 (%v)", weeklies, b.names(t, dir))
	}
	if _, err := os.Stat(filepath.Join(dir, todaySnapshot())); err != nil {
		t.Errorf("rotation removed the snapshot it had just taken: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sepia-2000-01-01.db")); !os.IsNotExist(err) {
		t.Errorf("the oldest daily survived rotation: %v", err)
	}
}

// TestBackupSkipsWhileOneIsRunning proves a slow run cannot stack: the lock is taken, and the second trigger
// records nothing at all rather than half a backup.
func TestBackupSkipsWhileOneIsRunning(t *testing.T) {
	b := newBackupTree(t, true)
	holder := exec.CommandContext(context.Background(), "bash", "-c",
		`exec 200>"$1"; flock -n 200; sleep 30`, "lock-holder", filepath.Join(b.root, ".backup-watcher.lock"))
	if err := holder.Start(); err != nil {
		t.Fatalf("start lock holder: %v", err)
	}
	defer func() { _ = holder.Process.Kill() }()
	// Give flock a moment to take the lock: the short read is the point, not a race the test should gamble on.
	time.Sleep(300 * time.Millisecond)

	out, err := b.run(t, nil)
	if err != nil {
		t.Fatalf("a skipped run should exit zero: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already running") {
		t.Errorf("output does not say it skipped:\n%s", out)
	}
	if got := b.receipts(t); len(got) != 0 {
		t.Errorf("a skipped run recorded receipts: %+v", got)
	}
}

// TestBackupWithNoDatabase records a named failure instead of an empty run.
func TestBackupWithNoDatabase(t *testing.T) {
	b := newBackupTree(t, true)
	if err := os.Remove(filepath.Join(b.data, "sepia.db")); err != nil {
		t.Fatalf("remove database: %v", err)
	}

	out, err := b.run(t, nil)
	if err == nil {
		t.Fatalf("a missing database did not fail the run:\n%s", out)
	}
	snap := findReceipt(t, b.receipts(t), "local:snapshot")
	if snap.Status != "failed" || !strings.Contains(snap.Detail, "no database") {
		t.Errorf("snapshot receipt = %q (%s), want failed naming the missing database", snap.Status, snap.Detail)
	}
}

// TestBackupSnapshotWithRealSQLite runs the script against the real sqlite3 binary and the real migrations, with
// only rclone faked. It is the test that would catch a wrong PRAGMA, a dot-command that changed, or a row-count
// query that stopped matching the schema. It skips where sqlite3 is not installed, because the script's real
// target is the potato.
func TestBackupSnapshotWithRealSQLite(t *testing.T) {
	sqlite, err := exec.LookPath("sqlite3")
	if err != nil {
		t.Skip("sqlite3 is not installed here")
	}
	migrations, err := filepath.Glob(filepath.Join("..", "store", "migrations", "*.sql"))
	if err != nil || len(migrations) == 0 {
		t.Fatalf("read migrations: %v, %d files", err, len(migrations))
	}
	b := newBackupTree(t, true)
	// The fakes are sqlite3 and rclone; keep only rclone so the real binary is the one on PATH.
	if err := os.Remove(filepath.Join(b.bin, "sqlite3")); err != nil {
		t.Fatalf("remove fake sqlite3: %v", err)
	}

	dbPath := filepath.Join(b.data, "sepia.db")
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("remove placeholder database: %v", err)
	}
	var schema strings.Builder
	for _, path := range migrations {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		schema.Write(body)
		schema.WriteString("\n")
	}
	seed := schema.String() + `
PRAGMA journal_mode = WAL;
INSERT INTO sessions (id, kind, created_at, updated_at)
VALUES ('s1', 'general', '2026-11-15T00:00:00Z', '2026-11-15T00:00:00Z');
`
	cmd := exec.CommandContext(context.Background(), sqlite, dbPath)
	cmd.Stdin = strings.NewReader(seed)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("seed database: %v\n%s", err, out)
	}

	if out, err := b.run(t, nil); err != nil {
		t.Fatalf("backup failed: %v\n%s", err, out)
	}

	snapshot := filepath.Join(b.data, "backups", "db", todaySnapshot())
	if body, err := exec.CommandContext(context.Background(), sqlite, snapshot, "PRAGMA integrity_check;").CombinedOutput(); err != nil || strings.TrimSpace(string(body)) != "ok" {
		t.Fatalf("the snapshot does not pass sqlite3's own check: %v %q", err, body)
	}
	if body, err := exec.CommandContext(context.Background(), sqlite, snapshot, "SELECT count(*) FROM sessions;").CombinedOutput(); err != nil || strings.TrimSpace(string(body)) != "1" {
		t.Fatalf("the snapshot lost the row: %v %q", err, body)
	}
	if db := findReceipt(t, b.receipts(t), "r2sepia:sepia-backups/db"); db.Status != "ok" {
		t.Errorf("db receipt = %q (%s), want ok", db.Status, db.Detail)
	}
	if body, err := exec.CommandContext(context.Background(), sqlite, snapshot, "PRAGMA journal_mode;").CombinedOutput(); err != nil || strings.TrimSpace(string(body)) != "delete" {
		t.Errorf("the published snapshot is not a single file: %v %q", err, body)
	}
	// A snapshot is one file. The backup inherits the source's WAL mode, so an open of it creates -wal and -shm
	// beside it, and rclone would copy those as if they were backups. Nothing but the snapshots belongs in the dir.
	for _, name := range b.names(t, filepath.Join(b.data, "backups", "db")) {
		if !strings.HasSuffix(name, ".db") {
			t.Errorf("%s is not a snapshot and must not be in the backup directory", name)
		}
	}
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
