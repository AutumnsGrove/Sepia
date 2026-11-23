package store

import (
	"context"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The marker is read from the first line only, and an unknown one is a load error rather than a comment: a
// migration that silently did not get the mode it asked for is worse than one that refuses to run.
func TestParsePragma(t *testing.T) {
	off, body, err := parsePragma("-- pragma: foreign_keys=off\nSELECT 1;")
	if err != nil || !off || body != "SELECT 1;" {
		t.Fatalf("parse = %v, %q, %v", off, body, err)
	}
	off, body, err = parsePragma("SELECT 1;")
	if err != nil || off || body != "SELECT 1;" {
		t.Fatalf("no marker = %v, %q, %v", off, body, err)
	}
	if _, _, err := parsePragma("-- pragma: make_it_fast\nSELECT 1;"); err == nil {
		t.Fatal("an unknown pragma was accepted")
	}
	// A marker on any line but the first is a comment, not a directive.
	if off, _, err := parsePragma("-- a note\n-- pragma: foreign_keys=off\nSELECT 1;"); err != nil || off {
		t.Fatalf("a later marker was honoured: off=%v err=%v", off, err)
	}
}

// Enforcement must be back on when the rebuild finishes. Left off, the next writer could bypass every trigger that
// protects the review, which is worse than the migration failing.
func TestRebuildRestoresForeignKeys(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	var enabled int
	if err := s.writer.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatalf("read pragma: %v", err)
	}
	if enabled != 1 {
		t.Errorf("foreign_keys = %d after migrating, want 1", enabled)
	}
}

// The rebuild keeps every row, and keeps every rule: an unrated draft is storable, a rating of 21 is not, and the
// triggers that stop a delete or a backward status change are still there afterwards.
func TestTheRebuildKeepsRowsAndRules(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seed(t, s)

	// Rebuild again over live data, the way 0004 did, and check the copy. 0004 rebuilt the table as it was then, so
	// everything added to entries since has to follow it the same way the real migrations do. Otherwise this test
	// would assert a schema the app no longer has, which is the exact failure the rebuild rule exists to catch.
	body, err := migrationsFS.ReadFile("migrations/0004_allow_unrated_drafts.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	off, rebuildSQL, err := parsePragma(string(body))
	if err != nil || !off {
		t.Fatalf("parse 0004 = %v, %v", off, err)
	}
	later, err := migrationsFS.ReadFile("migrations/0009_completion.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	rebuildSQL += "\n" + string(later)
	version, err := s.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if applyErr := s.apply(ctx, migration{version: version + 1, name: "rebuild_again", sql: rebuildSQL, foreignKeysOff: true}); applyErr != nil {
		t.Fatalf("apply rebuild: %v", applyErr)
	}

	entry, err := s.Entry(ctx, "e1")
	if err != nil {
		t.Fatalf("entry after the rebuild: %v", err)
	}
	if entry.Rating != 18 || entry.Body != "loved it" {
		t.Errorf("entry = %+v, want the rows copied", entry)
	}
	// A row that existed before the shelves did is a finished review, not a wish and not a stopped one.
	if entry.Completion != domain.CompletionFinished {
		t.Errorf("completion after the rebuild = %q, want %q", entry.Completion, domain.CompletionFinished)
	}

	// 0 is a draft with no rating yet.
	if err := raw(t, s, `UPDATE entries SET rating_q = 0 WHERE id = 'e1'`); err != nil {
		t.Errorf("an unrated draft was refused: %v", err)
	}
	if err := raw(t, s, `UPDATE entries SET rating_q = 21 WHERE id = 'e1'`); err == nil {
		t.Error("a rating of 21 was stored")
	}
	if err := raw(t, s, `DELETE FROM entries WHERE id = 'e1'`); err == nil {
		t.Error("the delete trigger did not survive the rebuild")
	}
	if err := raw(t, s, `UPDATE entries SET status = 'draft' WHERE id = 'e1'`); err == nil {
		t.Error("the status trigger did not survive the rebuild")
	}

	// The search triggers came back with the table, and they still do their job: the row that existed before the
	// rebuild is indexed, and a write after it is indexed too. Without this, a restore loop that silently skipped
	// would leave every test above green and search quietly stale.
	ctxSearch := context.Background()
	if hits, total, searchErr := s.Search(ctxSearch, domain.SearchQuery{Text: "loved", Limit: 10}); searchErr != nil {
		t.Fatalf("search after the rebuild: %v", searchErr)
	} else if len(hits) != 1 || total != 1 {
		t.Fatalf("search after the rebuild = %+v, total %d, want the seeded entry", hits, total)
	}
	if err := raw(t, s, `UPDATE entries SET body = 'loved it so much i read it twice' WHERE id = 'e1'`); err != nil {
		t.Fatalf("rewrite the body: %v", err)
	}
	if hits, _, err := s.Search(ctxSearch, domain.SearchQuery{Text: "twice", Limit: 10}); err != nil {
		t.Fatalf("search after a write: %v", err)
	} else if len(hits) != 1 {
		t.Fatalf("a write after the rebuild is not indexed: %+v", hits)
	}

	// And the indexes the lists depend on are back.
	for _, name := range []string{"entries_library", "entries_work", "entries_drafts"} {
		var found string
		if err := s.writer.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&found); err != nil {
			t.Errorf("index %s is missing after the rebuild: %v", name, err)
		}
	}
}

// A rebuild that leaves a dangling reference is rolled back whole, and the version does not move.
func TestABadRebuildIsRefused(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seed(t, s)
	before, err := s.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}

	// With enforcement off this statement succeeds, and leaves e1 pointing at a work that is gone.
	bad := "-- pragma: foreign_keys=off\nDELETE FROM works WHERE id = 'w1';"
	off, sql, err := parsePragma(bad)
	if err != nil || !off {
		t.Fatalf("parse = %v, %v", off, err)
	}
	if applyErr := s.apply(ctx, migration{version: before + 1, name: "bad_rebuild", sql: sql, foreignKeysOff: true}); applyErr == nil {
		t.Fatal("a rebuild with a dangling reference was committed")
	}

	after, err := s.Version(ctx)
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if after != before {
		t.Errorf("version moved to %d after a refused rebuild", after)
	}
	if _, err := s.Work(ctx, "w1"); err != nil {
		t.Errorf("the refused rebuild still removed the work: %v", err)
	}
	var enabled int
	if err := s.writer.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
		t.Errorf("foreign_keys = %d after a refused rebuild, want 1", enabled)
	}
}

// A draft's rating can be absent, and the domain guard still refuses to confirm it: the schema now permits the
// state, and the rule that matters lives where it always did.
func TestAnUnratedDraftCannotBeConfirmed(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	seed(t, s)
	if err := s.CreateDraft(ctx, domain.Entry{
		ID: "e-unrated", WorkID: "w1", Status: domain.StatusDraft,
		RatingScale: "quarter_stars_20", Body: "", BodyOrigin: domain.BodyTyped,
	}); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	drafts, err := s.Drafts(ctx, 10)
	if err != nil {
		t.Fatalf("Drafts: %v", err)
	}
	found := false
	for _, draft := range drafts {
		if draft.ID == "e-unrated" {
			found = true
			if draft.Rating != 0 {
				t.Errorf("draft rating = %d", draft.Rating)
			}
		}
	}
	if !found {
		t.Fatal("the unrated draft is not in the Drafts list")
	}
	if err := domain.ValidateForSave(domain.Entry{WorkID: "w1", Rating: 0, Body: "words", BodyOrigin: domain.BodyTyped}, "words"); err == nil {
		t.Error("the save guard accepted a draft with no rating")
	}
}
