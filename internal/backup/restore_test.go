package backup

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The restore is offline and must never publish a database that only looks like hers, so the tests are about refusal:
// a tampered snapshot, a truncated one, a target that already holds a database, and a copy that does not equal the
// library it came from.

// makeDB writes a real SQLite database with the four tables the counts read, so the integrity check is SQLite's own
// and not a fake's idea of one.
func makeDB(t *testing.T, path string, entries, sessions int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()
	statements := []string{
		"CREATE TABLE entries (id TEXT PRIMARY KEY, work_id TEXT NOT NULL)",
		"CREATE TABLE works (id TEXT PRIMARY KEY)",
		"CREATE TABLE entry_revisions (id TEXT PRIMARY KEY, entry_id TEXT NOT NULL REFERENCES entries (id))",
		"CREATE TABLE media (id TEXT PRIMARY KEY)",
		"CREATE TABLE sessions (id TEXT PRIMARY KEY)",
	}
	if entries > 0 {
		statements = append(statements, "INSERT INTO works (id) VALUES ('w1')")
	}
	for i := 0; i < entries; i++ {
		statements = append(statements,
			"INSERT INTO entries (id, work_id) VALUES ('e"+strconv.Itoa(i)+"', 'w1')",
			"INSERT INTO entry_revisions (id, entry_id) VALUES ('r"+strconv.Itoa(i)+"', 'e"+strconv.Itoa(i)+"')",
			"INSERT INTO media (id) VALUES ('m"+strconv.Itoa(i)+"')",
		)
	}
	for i := 0; i < sessions; i++ {
		statements = append(statements, "INSERT INTO sessions (id) VALUES ('s"+strconv.Itoa(i)+"')")
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
}

func copyForTest(t *testing.T, from, to string) {
	t.Helper()
	body, err := os.ReadFile(from)
	if err != nil {
		t.Fatalf("read %s: %v", from, err)
	}
	if err := os.WriteFile(to, body, 0o600); err != nil {
		t.Fatalf("write %s: %v", to, err)
	}
}

// TestRestoreWritesAVerifiedCopy is the happy route: prove the snapshot, copy it, prove the copy, report the rows.
func TestRestoreWritesAVerifiedCopy(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "sepia-2026-11-15.db")
	makeDB(t, snapshot, 3, 2)

	out := filepath.Join(dir, "scratch")
	result, err := Restore(context.Background(), RestoreRequest{Snapshot: snapshot, Into: out})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if result.Path != filepath.Join(out, "sepia.db") {
		t.Errorf("path = %q", result.Path)
	}
	if result.Counts != (Counts{Entries: 3, Revisions: 3, Media: 3, Sessions: 2}) {
		t.Errorf("counts = %+v", result.Counts)
	}
	info, err := os.Stat(result.Path)
	if err != nil {
		t.Fatalf("stat restored: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("restored mode = %o, want 600", got)
	}
	for _, name := range []string{"sepia.db.tmp"} {
		if _, err := os.Stat(filepath.Join(out, name)); err == nil {
			t.Errorf("%s was left behind", name)
		}
	}
}

// TestRestoreRefusesATamperedSnapshot: a file that is not a database never reaches the target directory.
func TestRestoreRefusesATamperedSnapshot(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(snapshot, []byte("this is not a database, it is a sentence"), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	out := filepath.Join(dir, "scratch")

	_, err := Restore(context.Background(), RestoreRequest{Snapshot: snapshot, Into: out})
	if !errors.Is(err, ErrSnapshotUnusable) {
		t.Fatalf("err = %v, want ErrSnapshotUnusable", err)
	}
	if _, err := os.Stat(filepath.Join(out, "sepia.db")); !os.IsNotExist(err) {
		t.Error("a tampered snapshot was published anyway")
	}
}

// TestRestoreRefusesATruncatedSnapshot: the corruption that a half-finished copy leaves is caught by SQLite itself.
func TestRestoreRefusesATruncatedSnapshot(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.db")
	makeDB(t, good, 4, 1)
	body, err := os.ReadFile(good)
	if err != nil {
		t.Fatalf("read good: %v", err)
	}
	cut := filepath.Join(dir, "cut.db")
	if err := os.WriteFile(cut, body[:len(body)/2], 0o600); err != nil {
		t.Fatalf("write cut: %v", err)
	}

	if _, err := Restore(context.Background(), RestoreRequest{Snapshot: cut, Into: filepath.Join(dir, "scratch")}); err == nil {
		t.Fatal("a truncated snapshot was restored")
	}
}

// TestRestoreRefusesAnOccupiedTarget: the one thing a restore must never do is overwrite the database that is there.
func TestRestoreRefusesAnOccupiedTarget(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "sepia.db.bak")
	makeDB(t, snapshot, 1, 1)
	out := filepath.Join(dir, "scratch")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	live := filepath.Join(out, "sepia.db")
	if err := os.WriteFile(live, []byte("the database in use"), 0o600); err != nil {
		t.Fatalf("write live: %v", err)
	}

	_, err := Restore(context.Background(), RestoreRequest{Snapshot: snapshot, Into: out})
	if !errors.Is(err, ErrTargetOccupied) {
		t.Fatalf("err = %v, want ErrTargetOccupied", err)
	}
	body, err := os.ReadFile(live)
	if err != nil || string(body) != "the database in use" {
		t.Errorf("the live database was touched: %q, %v", body, err)
	}
}

// TestRestoreIntoADirectoryWithOtherFiles is the real recovery: the target is the data directory the board already
// uses, which holds covers and the backups themselves. Only a database of its own blocks a restore.
func TestRestoreIntoADirectoryWithOtherFiles(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "sepia-2026-11-15.db")
	makeDB(t, snapshot, 2, 1)
	out := filepath.Join(dir, "data")
	for _, sub := range []string{"covers", "backups/db"} {
		if err := os.MkdirAll(filepath.Join(out, sub), 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "covers", "cover-1.jpg"), []byte("jpeg"), 0o600); err != nil {
		t.Fatalf("write cover: %v", err)
	}

	result, err := Restore(context.Background(), RestoreRequest{Snapshot: snapshot, Into: out})
	if err != nil {
		t.Fatalf("restore into a data directory: %v", err)
	}
	if result.Counts.Entries != 2 {
		t.Errorf("counts = %+v", result.Counts)
	}
	if _, err := os.Stat(filepath.Join(out, "covers", "cover-1.jpg")); err != nil {
		t.Errorf("the restore disturbed the covers: %v", err)
	}
}

// A half-written restore from a previous attempt counts as a database in the way: the caller has to look.
func TestRestoreRefusesALeftoverTempDatabase(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "sepia.db.bak")
	makeDB(t, snapshot, 1, 0)
	out := filepath.Join(dir, "data")
	if err := os.MkdirAll(out, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(out, "sepia.db.tmp"), []byte("half a database"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if _, err := Restore(context.Background(), RestoreRequest{Snapshot: snapshot, Into: out}); !errors.Is(err, ErrTargetOccupied) {
		t.Fatalf("err = %v, want ErrTargetOccupied", err)
	}
}

// TestRestoreComparesTheLibrary is the drill's assertion: restore then compare equals the source, and a difference is
// named rather than rounded away.
func TestRestoreComparesTheLibrary(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	makeDB(t, source, 2, 1)
	snapshot := filepath.Join(dir, "snapshot.db")
	copyForTest(t, source, snapshot)

	result, err := Restore(context.Background(), RestoreRequest{
		Snapshot: snapshot, Into: filepath.Join(dir, "scratch"), Compare: source,
	})
	if err != nil {
		t.Fatalf("restore with a matching source: %v", err)
	}
	if result.Counts.Entries != 2 {
		t.Errorf("counts = %+v", result.Counts)
	}

	// One more review saved after the snapshot: the restored copy is behind, and that is a refusal, not a warning.
	other := filepath.Join(dir, "later.db")
	makeDB(t, other, 3, 1)
	out := filepath.Join(dir, "scratch2")
	_, err = Restore(context.Background(), RestoreRequest{Snapshot: snapshot, Into: out, Compare: other})
	if err == nil {
		t.Fatal("a restore that does not match its source was accepted")
	}
	if !strings.Contains(err.Error(), "entries=2") || !strings.Contains(err.Error(), "entries=3") {
		t.Errorf("err = %v, want it to name both counts", err)
	}
	if _, err := os.Stat(filepath.Join(out, "sepia.db")); !os.IsNotExist(err) {
		t.Error("a mismatched restore was left on disk")
	}
}

// TestRestoreNeedsASnapshot: an empty request is a usage error, not a panic.
func TestRestoreNeedsASnapshot(t *testing.T) {
	if _, err := Restore(context.Background(), RestoreRequest{}); err == nil {
		t.Fatal("an empty request was accepted")
	}
	if _, err := Restore(context.Background(), RestoreRequest{Snapshot: "/nonexistent.db", Into: t.TempDir()}); !errors.Is(err, ErrSnapshotUnusable) {
		t.Fatalf("err = %v, want ErrSnapshotUnusable", err)
	}
}
