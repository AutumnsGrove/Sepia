package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The restore command is the offline half of S14. It is thin on purpose: the refusal logic is in internal/backup and
// tested there against real databases, so this file tests the command's own seams, the flags and the exit codes.

func TestRestoreArgs(t *testing.T) {
	good := []string{"--snapshot", "/s/sepia.db", "--into", "/scratch"}
	req, err := restoreArgs(good)
	if err != nil {
		t.Fatalf("restoreArgs: %v", err)
	}
	if req.Snapshot != "/s/sepia.db" || req.Into != "/scratch" || req.Compare != "" {
		t.Errorf("request = %+v", req)
	}

	req, err = restoreArgs([]string{"--snapshot=/s.db", "--into=/out", "--compare=/live.db"})
	if err != nil {
		t.Fatalf("restoreArgs with equals: %v", err)
	}
	if req.Snapshot != "/s.db" || req.Into != "/out" || req.Compare != "/live.db" {
		t.Errorf("request = %+v", req)
	}

	bad := [][]string{
		{},
		{"--snapshot", "/s.db"},
		{"--into", "/out"},
		{"--snapshot"},
		{"--snapshot", "/s.db", "--into", "/out", "--sometimes", "1"},
		{"--snapshot", " ", "--into", "/out"},
	}
	for _, args := range bad {
		if _, err := restoreArgs(args); err == nil {
			t.Errorf("restoreArgs(%v) accepted a bad command", args)
		}
	}
}

// Bad: a usage error exits 2 and prints the usage, which is what distinguishes it from a restore that refused.
func TestRestoreUsageError(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"restore", "--snapshot", "/s.db"}, nil, &out, &errOut)
	if code != 2 {
		t.Fatalf("exit = %d, want 2; stderr %s", code, errOut.String())
	}
	if !strings.Contains(errOut.String(), "sepia restore") {
		t.Errorf("stderr does not name the command:\n%s", errOut.String())
	}
}

// Sad: a snapshot that is not there exits 1 with a plain sentence and writes nothing.
func TestRestoreRefusesAMissingSnapshot(t *testing.T) {
	dir := t.TempDir()
	into := filepath.Join(dir, "scratch")
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"restore", "--snapshot", filepath.Join(dir, "nope.db"), "--into", into}, nil, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1; stderr %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(into, "sepia.db")); !os.IsNotExist(err) {
		t.Error("a refused restore wrote a database")
	}
}

// Happy: the command restores a real database and prints the row counts the operator compares against the board.
func TestRestoreCommand(t *testing.T) {
	dir := t.TempDir()
	snapshot := filepath.Join(dir, "sepia.db")
	db, err := sql.Open("sqlite", snapshot)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE entries (id TEXT PRIMARY KEY)",
		"CREATE TABLE entry_revisions (id TEXT PRIMARY KEY)",
		"CREATE TABLE media (id TEXT PRIMARY KEY)",
		"CREATE TABLE sessions (id TEXT PRIMARY KEY)",
		"INSERT INTO entries (id) VALUES ('e1')",
		"INSERT INTO sessions (id) VALUES ('s1')",
	} {
		if _, err := db.ExecContext(context.Background(), statement); err != nil {
			t.Fatalf("exec %q: %v", statement, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close snapshot: %v", err)
	}

	into := filepath.Join(dir, "scratch")
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"restore", "--snapshot", snapshot, "--into", into}, nil, &out, &errOut); code != 0 {
		t.Fatalf("exit = %d, want 0: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "entries=1 revisions=0 media=0 sessions=1") {
		t.Errorf("output does not name the counts:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(into, "sepia.db")); err != nil {
		t.Errorf("no restored database: %v", err)
	}
}
