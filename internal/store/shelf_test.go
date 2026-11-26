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
		ALTER TABLE entries_old RENAME TO entries;
		-- A database written before 0010 has no search index either, and its triggers would otherwise be left
		-- naming a virtual table that is about to be dropped.
		DROP TRIGGER IF EXISTS entries_fts_insert; DROP TRIGGER IF EXISTS entries_fts_update;
		DROP TRIGGER IF EXISTS entries_fts_answer; DROP TRIGGER IF EXISTS entries_fts_work_title;
		DROP TABLE entries_fts;`
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

// A page boundary that lands on an unrated row is the case the zero-is-a-real-key rule exists for. Before this, the
// rating key went through int64Null, which reads 0 as "no cursor": the next page's row-value comparison became NULL
// and the page came back empty, silently ending a list that had more in it.
func TestTheRatingCursorSurvivesAPageThatEndsOnAnUnratedRow(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	ctx := context.Background()

	// Three abandoned rows, all rated 0, with the seeded finished row (18) leading the order.
	rows := []struct{ id, finished string }{{"e-ab1", "2026-11-01"}, {"e-ab2", "2026-11-02"}, {"e-ab3", "2026-11-03"}}
	for _, row := range rows {
		if err := raw(t, s, `INSERT INTO entries
			(id, work_id, rating_q, rating_scale, body, body_origin, completion, progress, finished_on, status,
			 created_at, updated_at)
			VALUES (?, 'w1', 0, 'quarter_stars_20', 'some words', 'typed', 'abandoned', 'two episodes', ?, 'saved',
			        '2026-11-01T00:00:00Z', '2026-11-01T00:00:00Z')`, row.id, row.finished); err != nil {
			t.Fatalf("insert %s: %v", row.id, err)
		}
	}

	seen := make([]string, 0, 4)
	cursor := domain.Cursor{}
	for page := 0; page < 10; page++ {
		entries, next, err := s.LibraryPage(ctx, domain.LibraryQuery{Sort: domain.SortRating}, cursor, 2)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		for _, entry := range entries {
			seen = append(seen, entry.ID)
		}
		if next.Empty() {
			break
		}
		cursor = next
	}
	if len(seen) != 4 {
		t.Fatalf("paging the rating order saw %v, want four rows", seen)
	}
	// The rated row leads, the unrated ones follow, and no row appears twice.
	if seen[0] != "e1" {
		t.Errorf("first row = %q, want the rated one", seen[0])
	}
	unique := map[string]bool{}
	for _, id := range seen {
		if unique[id] {
			t.Fatalf("%s appeared twice in %v", id, seen)
		}
		unique[id] = true
	}
}

// S22.1: the three totals the tabs carry. One read, not three, and each shelf's number is the shelf's own: a wish is
// not a review, and a stopped work is not a finish. A draft is on no shelf, because it is not a row of the library at
// all, and a deleted row is on none either, because the library hides it and the Deleted list has its own count.
func TestShelfCounts(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	shelves(t, s)

	counts, err := s.ShelfCounts(context.Background())
	if err != nil {
		t.Fatalf("ShelfCounts: %v", err)
	}
	want := domain.ShelfCounts{Finished: 2, Abandoned: 1, WantTo: 1}
	if counts != want {
		t.Fatalf("counts = %+v, want %+v", counts, want)
	}

	// A draft and a deleted row are outside every shelf.
	if rowErr := raw(t, s, `INSERT INTO entries
		(id, work_id, rating_q, rating_scale, body, body_origin, completion, status, created_at, updated_at)
		VALUES ('e-draft', 'w1', 0, 'quarter_stars_20', 'nothing yet', 'typed', 'want_to', 'draft',
			'2026-11-21T00:00:00Z', '2026-11-21T00:00:00Z')`); rowErr != nil {
		t.Fatalf("insert draft: %v", rowErr)
	}
	if rowErr := raw(t, s, `UPDATE entries SET deleted_at = '2026-11-22T00:00:00Z' WHERE id = 'e-finished'`); rowErr != nil {
		t.Fatalf("soft-delete: %v", rowErr)
	}
	counts, err = s.ShelfCounts(context.Background())
	if err != nil {
		t.Fatalf("ShelfCounts after draft and delete: %v", err)
	}
	want = domain.ShelfCounts{Finished: 1, Abandoned: 1, WantTo: 1}
	if counts != want {
		t.Fatalf("counts = %+v, want %+v: a draft and a deleted row are on no shelf", counts, want)
	}
}

// S22.2: the wanted shelf is ordered by when it was added, not by a finish date it does not have. S21 left it ordered
// by id, which is a random string: the list looked arbitrary and a page could not be resumed from it in any order she
// would recognise. The cursor is (created_at, id), which is total.
func TestTheWantedShelfIsNewestAddedFirst(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	const at = "2026-11-30T00:00:00Z"
	wishes := []struct{ id, created string }{
		{"w-oldest", "2026-11-01T00:00:00Z"},
		{"w-middle", "2026-11-17T00:00:00Z"},
		{"w-newest", "2026-11-25T00:00:00Z"},
		// The tie breaks on id, descending, so the order stays total.
		{"w-tie-b", "2026-11-20T00:00:00Z"},
		{"w-tie-a", "2026-11-20T00:00:00Z"},
	}
	for _, wish := range wishes {
		if err := raw(t, s, `INSERT INTO entries
			(id, work_id, rating_q, rating_scale, body, body_origin, completion, status, created_at, updated_at)
			VALUES (?, 'w1', 0, 'quarter_stars_20', '', 'typed', 'want_to', 'saved', ?, ?)`,
			wish.id, wish.created, at); err != nil {
			t.Fatalf("insert %s: %v", wish.id, err)
		}
	}

	want := []string{"w-newest", "w-tie-b", "w-tie-a", "w-middle", "w-oldest"}
	got := listIDs(t, s, domain.LibraryQuery{Shelf: domain.CompletionWantTo})
	if len(got) != len(want) {
		t.Fatalf("wanted shelf = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("wanted shelf = %v, want %v", got, want)
		}
	}

	// Paged two at a time it is the same list, with no row seen twice and none skipped: a page boundary that fell on
	// the tie is the case the second key exists for.
	var seen []string
	var cursor domain.Cursor
	for page := 0; page < 5; page++ {
		entries, next, err := s.LibraryPage(context.Background(),
			domain.LibraryQuery{Shelf: domain.CompletionWantTo}, cursor, 2)
		if err != nil {
			t.Fatalf("LibraryPage: %v", err)
		}
		for _, entry := range entries {
			seen = append(seen, entry.ID)
		}
		if next.Empty() {
			break
		}
		cursor = next
	}
	if len(seen) != len(want) {
		t.Fatalf("paging the wanted shelf saw %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("paging the wanted shelf saw %v, want %v", seen, want)
		}
	}
}
