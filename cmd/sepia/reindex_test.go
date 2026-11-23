package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/store"
)

// reindex is the command the search index's own healing cannot cover: a file whose counts agree and whose text does
// not. These tests drive it through the real store, because the command's job is exactly "open the database the
// service opens and rewrite the index".

func TestReindexRebuildsTheIndex(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, store.Options{Path: filepath.Join(dir, "sepia.db")})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if migrateErr := db.Migrate(ctx); migrateErr != nil {
		t.Fatalf("migrate: %v", migrateErr)
	}
	if workErr := db.SaveWork(ctx, domain.Work{ID: "w1", Medium: "book", Title: "Piranesi", Verified: true}); workErr != nil {
		t.Fatalf("work: %v", workErr)
	}
	if draftErr := db.CreateDraft(ctx, domain.Entry{
		ID: "e1", WorkID: "w1", Rating: 19, RatingScale: domain.RatingScale, Body: "started slow, then it grabbed me",
		BodyOrigin: domain.BodyTyped, Status: domain.StatusSaved, Completion: domain.CompletionFinished,
	}); draftErr != nil {
		t.Fatalf("entry: %v", draftErr)
	}
	if closeErr := db.Close(); closeErr != nil {
		t.Fatalf("close: %v", closeErr)
	}

	var out, errOut bytes.Buffer
	code := run(ctx, []string{"reindex"}, []string{
		"SEPIA_DATA_DIR=" + dir, "SEPIA_ADDR=127.0.0.1:8893", "SEPIA_ALLOWED_HOSTS=localhost:8893,127.0.0.1:8893",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, want 0: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "rebuilt: 1 entry") {
		t.Errorf("output does not name the count:\n%s", out.String())
	}

	// The index is readable again, from a fresh handle, which is what a service starting after the command sees.
	reopened, err := store.Open(ctx, store.Options{Path: filepath.Join(dir, "sepia.db")})
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	hits, total, err := reopened.Search(ctx, domain.SearchQuery{Text: "grabbed", Limit: 10})
	if err != nil || total != 1 || len(hits) != 1 {
		t.Fatalf("search after reindex = %+v, %d, %v", hits, total, err)
	}
}

// Sad: an empty data directory is an empty library, not an error. The store creates the file it is pointed at, which
// is what every other command does too, and the command says "0 entries" rather than pretending it rebuilt something.
func TestReindexOnAnEmptyDataDirectory(t *testing.T) {
	dir := t.TempDir()
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"reindex"}, []string{
		"SEPIA_DATA_DIR=" + dir, "SEPIA_ADDR=127.0.0.1:8893", "SEPIA_ALLOWED_HOSTS=localhost:8893,127.0.0.1:8893",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit = %d, want 0: %s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "rebuilt: 0 entries") {
		t.Errorf("output = %q", out.String())
	}
	cfg, err := config.Load([]string{"SEPIA_DATA_DIR=" + dir, "SEPIA_ALLOWED_HOSTS=localhost:8893,127.0.0.1:8893"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != dir {
		t.Fatalf("data dir = %q", cfg.DataDir)
	}
}

// Bad: a broken environment is refused before the database is touched.
func TestReindexRefusesBadConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"reindex"}, []string{"SEPIA_ALLOWED_HOSTS=*"}, &out, &errOut)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	if out.Len() != 0 {
		t.Errorf("it wrote to stdout: %q", out.String())
	}
}
