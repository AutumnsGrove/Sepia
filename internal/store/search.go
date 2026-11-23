package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// Search is the library asked in her own words: SQLite's FTS5 over the work's title, the review body and the answers
// she gave, ranked title first, then body, then answers (docs/slices/S16-search-and-chat.md, Decisions 1).
//
// The index is derived data and the triggers in 0010_search keep it in step, but a file restored from an older backup
// or edited by hand can still be out of step. Every search therefore compares the index's row count with the number
// of saved entries and rebuilds it when the two disagree, so the failure a reader would see is a slightly slower
// search rather than a missing review. `sepia reindex` does the same rebuild on purpose.
//
// SQL is hand-written rather than generated: sqlc models tables, and an FTS5 virtual table is not one it can see, so
// naming entries_fts in a query file fails generation. The schema is still a migration, and the queries below are
// the only hand-written SQL in the store.
func (s *Store) Search(ctx context.Context, query domain.SearchQuery) ([]domain.SearchHit, int, error) {
	terms, err := domain.ParseSearchText(query.Text)
	if err != nil {
		return nil, 0, err
	}
	limit := clampLimit(query.Limit)
	if driftErr := s.repairSearchIndex(ctx); driftErr != nil {
		return nil, 0, driftErr
	}
	where := searchWhere(query)

	args := append([]any{domain.SnippetOpen, domain.SnippetClose, domain.SnippetOpen, domain.SnippetClose},
		whereArgs(terms.Match, query)...)
	args = append(args, int64(limit))
	rows, err := s.read(ctx, searchHitsSQL+strings.Join(where, " AND ")+`
ORDER BY rank_score, e.id
LIMIT ?`, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("store: search: %w", err)
	}
	defer func() { _ = rows.Close() }()

	hits := make([]domain.SearchHit, 0, limit)
	for rows.Next() {
		var (
			entryID string
			body    string
			answers string
			rank    float64
		)
		if scanErr := rows.Scan(&entryID, &body, &answers, &rank); scanErr != nil {
			return nil, 0, fmt.Errorf("store: scan search hit: %w", scanErr)
		}
		hit, hitErr := s.searchHit(ctx, entryID, body, answers, rank)
		if errors.Is(hitErr, sql.ErrNoRows) {
			// The index named an entry the table does not have. The next search will notice the counts disagree
			// and rebuild; skipping the row is what keeps this one honest rather than papering over it.
			continue
		}
		if hitErr != nil {
			return nil, 0, hitErr
		}
		hits = append(hits, hit)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, 0, fmt.Errorf("store: search rows: %w", rowsErr)
	}

	var total int
	countSQL := `SELECT count(*) FROM entries_fts CROSS JOIN entries e ON e.id = entries_fts.entry_id
JOIN works w ON w.id = e.work_id
WHERE ` + strings.Join(where, " AND ")
	if err := s.readers.QueryRowContext(ctx, countSQL, whereArgs(terms.Match, query)...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("store: count search: %w", err)
	}
	return hits, total, nil
}

// searchHitsSQL is the select every search runs. The weights are per column in the table's own order, so the first
// one weighs the UNINDEXED entry_id, which is not in the index at all.
//
// The FTS table drives the join, and CROSS JOIN is what makes it: without it SQLite may start from entries_library
// (a medium or date filter makes that look cheaper) and then probe the virtual table once per row, which on a seeded
// library of five thousand entries took 729 ms against 19 ms the other way round.
const searchHitsSQL = `
SELECT entries_fts.entry_id,
       snippet(entries_fts, 2, ?, ?, '…', 12) AS body_snippet,
       snippet(entries_fts, 3, ?, ?, '…', 12) AS answer_snippet,
       bm25(entries_fts, 0.0, 3.0, 1.0, 0.5) AS rank_score
FROM entries_fts CROSS JOIN entries e ON e.id = entries_fts.entry_id
JOIN works w ON w.id = e.work_id
WHERE `

// searchWhere is the conditions the filters add, in the order whereArgs supplies their values. Every value is a bound
// parameter: the only text in the statement is what this file wrote.
func searchWhere(query domain.SearchQuery) []string {
	where := []string{"entries_fts MATCH ?", "e.status = 'saved'", "e.deleted_at IS NULL"}
	if query.Medium != "" {
		where = append(where, "w.medium = ?")
	}
	if query.Favourite != nil {
		where = append(where, "e.favourite = ?")
	}
	if query.RatingMin > 0 {
		where = append(where, "e.rating_q >= ?")
	}
	if query.RatingMax > 0 {
		where = append(where, "e.rating_q <= ?")
	}
	if query.From != "" {
		where = append(where, "e.finished_on >= ?")
	}
	if query.To != "" {
		where = append(where, "e.finished_on <= ?")
	}
	return where
}

// whereArgs is searchWhere's arguments, in the same order.
func whereArgs(match string, query domain.SearchQuery) []any {
	args := []any{match}
	if query.Medium != "" {
		args = append(args, query.Medium)
	}
	if query.Favourite != nil {
		args = append(args, boolToInt(*query.Favourite))
	}
	if query.RatingMin > 0 {
		args = append(args, int64(query.RatingMin))
	}
	if query.RatingMax > 0 {
		args = append(args, int64(query.RatingMax))
	}
	if query.From != "" {
		args = append(args, query.From)
	}
	if query.To != "" {
		args = append(args, query.To)
	}
	return args
}

// searchHit fills one row in. The title match is the one case with nothing to show: the snippet would repeat the
// title in the line above it, so it is left empty rather than printed twice.
func (s *Store) searchHit(ctx context.Context, entryID, body, answers string, rank float64) (domain.SearchHit, error) {
	entry, err := s.Entry(ctx, entryID)
	if err != nil {
		return domain.SearchHit{}, err
	}
	work, err := s.Work(ctx, entry.WorkID)
	if err != nil {
		return domain.SearchHit{}, err
	}
	snippet := ""
	switch {
	case strings.Contains(body, domain.SnippetOpen):
		snippet = body
	case strings.Contains(answers, domain.SnippetOpen):
		snippet = answers
	}
	return domain.SearchHit{
		Kind: "entry", ID: entry.ID, Title: work.Title, Snippet: snippet,
		// bm25 is negative and more negative is a better match; the API's score is the other way round, so a
		// bigger number is better for whoever reads it.
		Score: -rank, Entry: entry, Work: work,
	}, nil
}

// SearchIndexDrifted reports whether the index holds a different number of entries from the library. Two counts are
// cheap next to a scan of every row's text, and they catch what a rebuild can fix: a missing row, an extra one, or an
// index that was never built.
func (s *Store) SearchIndexDrifted(ctx context.Context) (bool, error) {
	var indexed, saved int
	err := s.readers.QueryRowContext(ctx, `
SELECT (SELECT count(*) FROM entries_fts),
       (SELECT count(*) FROM entries WHERE status = 'saved' AND deleted_at IS NULL)`).Scan(&indexed, &saved)
	if err != nil {
		return false, fmt.Errorf("store: read search index counts: %w", err)
	}
	return indexed != saved, nil
}

// repairSearchIndex rebuilds the index when it is out of step, so a reader never has to know.
func (s *Store) repairSearchIndex(ctx context.Context) error {
	drifted, err := s.SearchIndexDrifted(ctx)
	if err != nil {
		return err
	}
	if !drifted {
		return nil
	}
	if _, err := s.RebuildSearchIndex(ctx); err != nil {
		return err
	}
	return nil
}

// RebuildSearchIndex throws the index away and writes it again from the entries and works, in one transaction, so a
// search never runs against a half-built index. It returns how many entries it wrote. It is the `sepia reindex`
// command, and what a search calls when the counts disagree.
func (s *Store) RebuildSearchIndex(ctx context.Context) (int, error) {
	var written int
	err := s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM entries_fts`); err != nil {
			return fmt.Errorf("store: clear search index: %w", err)
		}
		result, err := tx.ExecContext(ctx, `
INSERT INTO entries_fts (entry_id, title, body, answers)
SELECT e.id, w.title, e.body,
       COALESCE((SELECT group_concat(j.value, ' ')
                 FROM answers a, json_each(a.answer_json) j
                 WHERE a.entry_id = e.id), '')
FROM entries e JOIN works w ON w.id = e.work_id
WHERE e.status = 'saved' AND e.deleted_at IS NULL`)
		if err != nil {
			return fmt.Errorf("store: rebuild search index: %w", err)
		}
		rows, _ := result.RowsAffected()
		written = int(rows)
		return nil
	})
	if err != nil {
		return 0, err
	}
	return written, nil
}
