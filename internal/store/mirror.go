package store

import (
	"context"
	"fmt"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/store/db"
)

// The backup receipts. The host script appends them to a file and the app imports them here, so the same run can be
// imported twice without doubling: the import id is the run plus the target, and the insert ignores a duplicate id.
// Nothing here is ever updated or deleted (docs/BUILD.md invariant 4).

// RecordMirror writes one receipt. A repeat of an id already present is a no-op rather than an error, which is what
// makes the import idempotent after a crash between the file and the database.
func (s *Store) RecordMirror(ctx context.Context, m domain.Mirror) error {
	if m.ID == "" {
		return fmt.Errorf("store: a mirror receipt needs an id")
	}
	if !m.Status.Valid() {
		return fmt.Errorf("store: %q is not a mirror status", m.Status)
	}
	if m.RanAt.IsZero() {
		m.RanAt = s.Now()
	}
	return s.InTx(ctx, func(q *db.Queries) error {
		return q.RecordMirror(ctx, db.RecordMirrorParams{
			ID: m.ID, RanAt: m.RanAt.UTC().Format(timeLayout), Target: m.Target,
			Status: string(m.Status), Bytes: m.Bytes, Detail: m.Detail,
		})
	})
}

// Mirrors returns the newest receipts, most recent first.
func (s *Store) Mirrors(ctx context.Context, limit int) ([]domain.Mirror, error) {
	if limit <= 0 || limit > maxMirrors {
		limit = maxMirrors
	}
	rows, err := s.ReadQueries().ListMirrors(ctx, int64(limit))
	if err != nil {
		return nil, fmt.Errorf("store: list mirrors: %w", err)
	}
	out := make([]domain.Mirror, 0, len(rows))
	for _, row := range rows {
		out = append(out, domain.Mirror{
			ID: row.ID, RanAt: parseTime(row.RanAt), Target: row.Target,
			Status: domain.MirrorStatus(row.Status), Bytes: row.Bytes, Detail: row.Detail,
		})
	}
	return out, nil
}

// maxMirrors bounds the receipt read: two targets a night is a year of history in under a thousand rows, and the
// screen shows the newest handful.
const maxMirrors = 500
