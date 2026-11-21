package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The three shelves, in the store. These tests are about what the list queries return, not about what the domain
// guard allows: a wish may sit in the same table as a review, but it must not be in the same list unless she asked
// for it.

// shelves inserts one entry per shelf on the same work, with distinct finish dates so the newest order is known.
func shelves(t *testing.T, s *Store) {
	t.Helper()
	const at = "2026-11-20T00:00:00Z"
	rows := []struct {
		id         string
		completion string
		rating     int
		progress   string
		finishedOn any
		created    string
	}{
		{"e-finished", "finished", 18, "", "2026-11-01", "2026-11-01T00:00:00Z"},
		{"e-abandoned", "abandoned", 0, "about a hundred pages in", "2026-11-10", "2026-11-10T00:00:00Z"},
		{"e-wanted", "want_to", 0, "", nil, "2026-11-20T00:00:00Z"},
	}
	for _, row := range rows {
		if err := raw(t, s, `INSERT INTO entries
			(id, work_id, rating_q, rating_scale, body, body_origin, completion, progress, finished_on, status,
			 created_at, updated_at)
			VALUES (?, 'w1', ?, 'quarter_stars_20', 'some words', 'typed', ?, ?, ?, 'saved', ?, ?)`,
			row.id, row.rating, row.completion, row.progress, row.finishedOn, row.created, at); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}
}

func listIDs(t *testing.T, s *Store, query domain.LibraryQuery) []string {
	t.Helper()
	entries, _, err := s.LibraryPage(context.Background(), query, domain.Cursor{}, 50)
	if err != nil {
		t.Fatalf("LibraryPage %+v: %v", query, err)
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func TestTheDefaultLibraryShelfIsEverythingReviewed(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	shelves(t, s)

	// e1 comes from seed and is a finished review.
	got := listIDs(t, s, domain.LibraryQuery{})
	want := []string{"e-abandoned", "e-finished", "e1"}
	if len(got) != len(want) {
		t.Fatalf("default shelf = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("default shelf = %v, want %v", got, want)
		}
	}
	// A wish is not a review and has no finish date, so it is not in the list she scans.
	for _, id := range got {
		if id == "e-wanted" {
			t.Fatal("a wanted row was in the default library")
		}
	}
}

func TestEachShelfShowsOnlyItsOwnRows(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	shelves(t, s)

	cases := []struct {
		shelf domain.Completion
		want  []string
	}{
		{domain.CompletionWantTo, []string{"e-wanted"}},
		{domain.CompletionAbandoned, []string{"e-abandoned"}},
		{domain.CompletionFinished, []string{"e-finished", "e1"}},
	}
	for _, c := range cases {
		got := listIDs(t, s, domain.LibraryQuery{Shelf: c.shelf})
		if len(got) != len(c.want) {
			t.Fatalf("shelf %q = %v, want %v", c.shelf, got, c.want)
		}
		for i := range c.want {
			if got[i] != c.want[i] {
				t.Fatalf("shelf %q = %v, want %v", c.shelf, got, c.want)
			}
		}
	}
}

func TestTheRatingOrderPutsUnfinishedRowsAfterTheRatedOnes(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	shelves(t, s)

	got := listIDs(t, s, domain.LibraryQuery{Sort: domain.SortRating})
	// e1 and e-finished are rated, so they lead. The dropped book has no rating to rank by, so it follows rather
	// than sitting among the rated ones at zero. The wish is not on this shelf at all.
	if len(got) != 3 {
		t.Fatalf("rating order = %v, want three reviewed rows", got)
	}
	if got[0] != "e-finished" && got[0] != "e1" {
		t.Errorf("rating order starts with %q, want a finished row", got[0])
	}
	if got[1] != "e-finished" && got[1] != "e1" {
		t.Errorf("rating order second is %q, want the other finished row", got[1])
	}
	if got[2] != "e-abandoned" {
		t.Errorf("rating order tail = %q, want the unrated row last", got[2])
	}

	// The wish sorts into the same tail when its own shelf is asked for, ahead of the finished rows.
	wanted := listIDs(t, s, domain.LibraryQuery{Sort: domain.SortRating, Shelf: domain.CompletionWantTo})
	if len(wanted) != 1 || wanted[0] != "e-wanted" {
		t.Errorf("wanted shelf by rating = %v", wanted)
	}

	// The cursor carries the rank, so the page after the boundary neither repeats nor skips a row.
	first, next, err := s.LibraryPage(context.Background(), domain.LibraryQuery{Sort: domain.SortRating},
		domain.Cursor{}, 2)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if next.Empty() || next.CompletionRank != 1 {
		t.Fatalf("first page cursor = %+v, want a ranked key", next)
	}
	second, _, err := s.LibraryPage(context.Background(), domain.LibraryQuery{Sort: domain.SortRating}, next, 2)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	seen := map[string]bool{}
	for _, entry := range append(first, second...) {
		if seen[entry.ID] {
			t.Fatalf("%s appeared on both pages", entry.ID)
		}
		seen[entry.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("two pages covered %d rows, want 3", len(seen))
	}
}

func TestProgressIsStoredOnlyForAnAbandonedEntry(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	ctx := context.Background()

	if err := s.CreateDraft(ctx, domain.Entry{
		ID: "e-dropped", WorkID: "w1", RatingScale: domain.RatingScale, Status: domain.StatusDraft,
		Completion: domain.CompletionAbandoned, Progress: "two episodes in",
		Body: "not for me", BodyOrigin: domain.BodyTyped,
	}); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	stored, err := s.Entry(ctx, "e-dropped")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if stored.Completion != domain.CompletionAbandoned || stored.Progress != "two episodes in" {
		t.Fatalf("stored = %+v", stored)
	}

	// A stale progress line on a finished entry is cleared on the way in, because the card never shows it and it
	// would otherwise travel into the export as an opinion about nothing.
	if createErr := s.CreateDraft(ctx, domain.Entry{
		ID: "e-stale", WorkID: "w1", Rating: 16, RatingScale: domain.RatingScale, Status: domain.StatusDraft,
		Completion: domain.CompletionFinished, Progress: "two episodes in",
		Body: "good", BodyOrigin: domain.BodyTyped,
	}); createErr != nil {
		t.Fatalf("CreateDraft: %v", createErr)
	}
	stale, err := s.Entry(ctx, "e-stale")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if stale.Progress != "" {
		t.Fatalf("progress on a finished entry = %q, want empty", stale.Progress)
	}
}

func TestTheCompletionCheckIsInTheDatabase(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	if err := raw(t, s, `UPDATE entries SET completion = 'halfway' WHERE id = 'e1'`); err == nil {
		t.Fatal("an unknown completion was stored")
	}
}

// A database written before the shelves existed is upgraded by the default, and every row it held is a finished
// review. This is the migration path a running deployment takes, so it is tested from a real 0008 database rather
// than from a fresh one.
func TestUpgradingFromBeforeTheShelvesKeepsEveryRowFinished(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "sepia.db")
	s, err := Open(ctx, Options{Path: path, Now: fixedNow, NewID: func() string { return "test-id" }})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if migrateErr := s.Migrate(ctx); migrateErr != nil {
		t.Fatalf("Migrate: %v", migrateErr)
	}
	seed(t, s)

	// Step back to the schema just before 0009 by removing its two columns the way SQLite documents it, which is
	// what a database written by the previous release actually looks like. It runs through the rebuild path, because
	// dropping a table that other tables reference needs enforcement off.
	stepBack := `CREATE TABLE entries_old (
			id TEXT PRIMARY KEY, work_id TEXT NOT NULL REFERENCES works (id) ON DELETE RESTRICT,
			rating_q INTEGER NOT NULL DEFAULT 0 CHECK (rating_q BETWEEN 0 AND 20),
			rating_scale TEXT NOT NULL DEFAULT 'quarter_stars_20',
			favourite INTEGER NOT NULL DEFAULT 0 CHECK (favourite IN (0, 1)),
			season INTEGER CHECK (season IS NULL OR season >= 1),
			verdict TEXT NOT NULL DEFAULT '', body TEXT NOT NULL,
			body_origin TEXT NOT NULL CHECK (body_origin IN ('extracted','typed','mixed','imported')),
			imported_from TEXT NOT NULL DEFAULT '', body_spans_json TEXT NOT NULL DEFAULT '[]',
			format TEXT NOT NULL DEFAULT '', finished_on TEXT,
			status TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','saved','published')),
			deleted_at TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
		INSERT INTO entries_old SELECT id, work_id, rating_q, rating_scale, favourite, season, verdict, body,
			body_origin, imported_from, body_spans_json, format, finished_on, status, deleted_at, created_at,
			updated_at FROM entries;
		DROP TABLE entries;
		ALTER TABLE entries_old RENAME TO entries;`
	if applyErr := s.apply(ctx, migration{version: 8, name: "step_back", sql: stepBack, foreignKeysOff: true}); applyErr != nil {
		t.Fatalf("step back: %v", applyErr)
	}

	if upgradeErr := s.Migrate(ctx); upgradeErr != nil {
		t.Fatalf("Migrate to 0009: %v", upgradeErr)
	}
	entry, err := s.Entry(ctx, "e1")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if entry.Completion != domain.CompletionFinished {
		t.Fatalf("completion = %q, want %q", entry.Completion, domain.CompletionFinished)
	}
	if entry.Progress != "" {
		t.Fatalf("progress = %q, want empty", entry.Progress)
	}
	if entry.Rating != 18 || entry.Body != "loved it" {
		t.Fatalf("entry = %+v, want the row unchanged", entry)
	}
}
