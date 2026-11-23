package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// Search is the one read whose cost grows with the library, so it gets a budget rather than only a correctness test.
// The library here is 5,000 entries: ten times the largest personal journal this app has seen, and enough that a query
// that scanned every body instead of using the index would be obvious.
//
// The budget is deliberately loose. It runs on the board's ARM chip through Docker with a cold page cache, which is
// several times slower than the machine that wrote it, and a test that fails on a busy laptop teaches nothing.
//
// Under the race detector there is no absolute budget: the runtime costs a factor of a hundred on this read, so a wall
// clock there measures the detector rather than the query. What is asserted on both builds is the *shape* of the cost,
// which is what a bad plan changes: a filtered search may not cost several times what an unfiltered one does.

const (
	// searchBudget is the longest one search may take on that library, when it is measured without the race runtime.
	searchBudget = 250 * time.Millisecond
	// searchFilterRatio is how much more a filtered search may cost than an unfiltered one. The plan that made the
	// filtered read 40 times slower passed every correctness test in this package.
	searchFilterRatio = 3.0
)

// seedLargeLibrary writes the invented library straight into the tables in three statements, because five thousand
// confirmed cards through the engine would make the test about the engine rather than about search.
func seedLargeLibrary(t *testing.T, s *Store) {
	t.Helper()
	ctx := context.Background()
	const at = "2026-10-31T12:00:00Z"
	statements := []statement{
		{
			`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < ? - 1)
		  INSERT INTO works (id, medium, title, year, created_at, updated_at)
		  SELECT 'w-' || i,
		         CASE i % 8 WHEN 0 THEN 'book' WHEN 1 THEN 'film' WHEN 2 THEN 'tv' WHEN 3 THEN 'game'
		                    WHEN 4 THEN 'album' WHEN 5 THEN 'podcast' WHEN 6 THEN 'theatre' ELSE 'other' END,
		         'Invented Work ' || i, 1900 + i % 120, ?, ? FROM n`,
			[]any{searchBenchEntries, at, at},
		},
		{
			`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < ? - 1)
		  INSERT INTO entries (id, work_id, rating_q, rating_scale, body, body_origin, status, completion,
		                       finished_on, created_at, updated_at)
		  SELECT 'e-' || i, 'w-' || i, 1 + i % 20, 'quarter_stars_20',
		         'entry ' || i || ': a slow burn about lighthouses, memory and the weather at the harbour',
		         'typed', 'saved', 'finished',
		         printf('2026-%02d-%02d', 1 + i % 12, 1 + i % 28), ?, ? FROM n`,
			[]any{searchBenchEntries, at, at},
		},
		{
			`WITH RECURSIVE n(i) AS (SELECT 0 UNION ALL SELECT i + 1 FROM n WHERE i < ? - 1)
		  INSERT INTO answers (id, entry_id, question_id, category, prompt, answer_type, answer_json, created_at)
		  SELECT 'a-' || i, 'e-' || i, 'pace', 'mood', 'How did it move?', 'tags', '["lighthouse"]', ? FROM n`,
			[]any{searchBenchEntries, at},
		},
	}
	err := s.write(ctx, func(tx *sql.Tx) error {
		for _, stmt := range statements {
			if _, err := tx.ExecContext(ctx, stmt.sql, stmt.args...); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed %d entries: %v", searchBenchEntries, err)
	}
}

// statement is one seed statement and the values it binds.
type statement struct {
	sql  string
	args []any
}

// timedSearch runs one search and returns how long it took, with its hits and its count.
func timedSearch(t *testing.T, s *Store, query domain.SearchQuery) (time.Duration, []domain.SearchHit, int) {
	t.Helper()
	start := time.Now()
	hits, total, err := s.Search(context.Background(), query)
	took := time.Since(start)
	if err != nil {
		t.Fatalf("search %q: %v", query.Text, err)
	}
	return took, hits, total
}

// Happy: on a library of five thousand entries, a search for a word most of them hold answers a page and its count
// within the budget, with the snippet and the entry it names.
func TestSearchBudgetOnALargeLibrary(t *testing.T) {
	if testing.Short() {
		t.Skip("the large library is not built in a short run")
	}
	s := openTest(t)
	seedLargeLibrary(t, s)

	// One search first: the first read pays for the plan cache.
	_, _, _ = timedSearch(t, s, domain.SearchQuery{Text: "harbour", Limit: 50})

	var slowest time.Duration
	for run := 0; run < 3; run++ {
		took, hits, total := timedSearch(t, s, domain.SearchQuery{Text: "harbour", Limit: 50})
		if len(hits) != 50 || total != searchBenchEntries {
			t.Fatalf("hits = %d, total = %d", len(hits), total)
		}
		// The row a result needs comes with the hit: the entry it names and the sentence it was found in.
		if hits[0].Entry.ID == "" || hits[0].Work.Title == "" || hits[0].Snippet == "" {
			t.Fatalf("hit is missing its row: %+v", hits[0])
		}
		if took > slowest {
			slowest = took
		}
	}
	t.Logf("unfiltered search over %d entries: slowest of three = %v", searchBenchEntries, slowest)
	if !raceDetector && slowest > searchBudget {
		t.Fatalf("a search took %v, over the %v budget", slowest, searchBudget)
	}

	// A filtered search is the same order of cost: the filters are part of the query, not applied afterwards.
	filtered, hits, total := timedSearch(t, s, domain.SearchQuery{
		Text: "harbour", Medium: "book", From: "2026-06-01", To: "2026-12-31", Limit: 50,
	})
	t.Logf("filtered search: %v for %d of %d", filtered, len(hits), total)
	if limit := time.Duration(float64(slowest)*searchFilterRatio) + 50*time.Millisecond; filtered > limit {
		t.Fatalf("a filtered search took %v, over %v: the query plan is driving the wrong way round", filtered, limit)
	}
	for _, hit := range hits {
		if hit.Work.Medium != "book" || hit.Entry.FinishedOn < "2026-06-01" || hit.Entry.FinishedOn > "2026-12-31" {
			t.Fatalf("filtered hit escapes the filter: %+v", hit.Entry)
		}
	}
	if total == 0 || total == searchBenchEntries {
		t.Fatalf("filtered total = %d, want a subset of the library", total)
	}
	if len(hits) == 0 || len(hits) > 50 {
		t.Fatalf("%d filtered hits", len(hits))
	}
}
