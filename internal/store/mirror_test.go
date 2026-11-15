package store

import (
	"context"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The mirrors table is the backup receipts the host script writes and the app imports. Two things matter: an
// import that repeats itself must not double a run, and the list must be the newest first, because that is what the
// screen shows and what the Services row reads.

func mirror(id, target string, status domain.MirrorStatus, at time.Time, bytes int64) domain.Mirror {
	return domain.Mirror{ID: id, RanAt: at, Target: target, Status: status, Bytes: bytes}
}

func TestMirrorRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 15, 8, 30, 0, 0, time.UTC)

	if err := s.RecordMirror(ctx, mirror("run-1:db", "r2sepia:sepia-backups/db", domain.MirrorOK, at, 4_200_000)); err != nil {
		t.Fatalf("RecordMirror: %v", err)
	}
	if err := s.RecordMirror(ctx, domain.Mirror{
		ID: "run-1:covers", RanAt: at, Target: "r2sepia:sepia-backups/covers",
		Status: domain.MirrorFailed, Detail: "AccessDenied",
	}); err != nil {
		t.Fatalf("RecordMirror: %v", err)
	}

	got, err := s.Mirrors(ctx, 10)
	if err != nil {
		t.Fatalf("Mirrors: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("mirrors = %d, want 2: %+v", len(got), got)
	}
	byID := map[string]domain.Mirror{}
	for _, m := range got {
		byID[m.ID] = m
	}
	db := byID["run-1:db"]
	if !db.RanAt.Equal(at) {
		t.Errorf("ran_at = %v, want %v", db.RanAt, at)
	}
	if db.Status != domain.MirrorOK || db.Bytes != 4_200_000 || db.Target != "r2sepia:sepia-backups/db" {
		t.Errorf("db receipt = %+v", db)
	}
	covers := byID["run-1:covers"]
	if covers.Status != domain.MirrorFailed || covers.Detail != "AccessDenied" {
		t.Errorf("covers receipt = %+v", covers)
	}
}

// Bad: the same run imported twice is one row, which is what makes an import after a crash safe.
func TestMirrorImportIsIdempotent(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 15, 8, 30, 0, 0, time.UTC)
	receipt := mirror("run-1:db", "r2sepia:sepia-backups/db", domain.MirrorOK, at, 100)

	for i := 0; i < 3; i++ {
		if err := s.RecordMirror(ctx, receipt); err != nil {
			t.Fatalf("RecordMirror %d: %v", i, err)
		}
	}
	got, err := s.Mirrors(ctx, 10)
	if err != nil {
		t.Fatalf("Mirrors: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("mirrors = %d, want 1 after three imports", len(got))
	}
	if got[0].Bytes != 100 {
		t.Errorf("bytes = %d, want the first import kept", got[0].Bytes)
	}
}

// Bad: a receipt the schema would refuse is refused by the repository first, with a sentence rather than a CHECK
// constraint error.
func TestMirrorRefusesAnInvalidReceipt(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	if err := s.RecordMirror(ctx, domain.Mirror{Target: "r2sepia:sepia-backups/db", Status: domain.MirrorOK}); err == nil {
		t.Error("a receipt with no id was accepted")
	}
	if err := s.RecordMirror(ctx, domain.Mirror{ID: "run-1:db", Target: "t", Status: domain.MirrorStatus("exploded")}); err == nil {
		t.Error("a receipt with an unknown status was accepted")
	}
	got, err := s.Mirrors(ctx, 10)
	if err != nil {
		t.Fatalf("Mirrors: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("mirrors = %+v, want none", got)
	}
}

// The list is newest first, which is the order the screen draws and the Services row depends on.
func TestMirrorsAreNewestFirstAndBounded(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	for i, at := range []time.Time{
		time.Date(2026, 11, 13, 8, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 15, 8, 30, 0, 0, time.UTC),
		time.Date(2026, 11, 14, 8, 30, 0, 0, time.UTC),
	} {
		id := "run-" + string(rune('a'+i)) + ":db"
		if err := s.RecordMirror(ctx, mirror(id, "r2sepia:sepia-backups/db", domain.MirrorOK, at, 1)); err != nil {
			t.Fatalf("RecordMirror: %v", err)
		}
	}

	got, err := s.Mirrors(ctx, 10)
	if err != nil {
		t.Fatalf("Mirrors: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("mirrors = %d, want 3", len(got))
	}
	if !got[0].RanAt.Equal(time.Date(2026, 11, 15, 8, 30, 0, 0, time.UTC)) {
		t.Errorf("first receipt = %v, want the newest", got[0].RanAt)
	}
	if !got[2].RanAt.Equal(time.Date(2026, 11, 13, 8, 30, 0, 0, time.UTC)) {
		t.Errorf("last receipt = %v, want the oldest", got[2].RanAt)
	}

	one, err := s.Mirrors(ctx, 1)
	if err != nil {
		t.Fatalf("Mirrors: %v", err)
	}
	if len(one) != 1 {
		t.Errorf("limit 1 returned %d", len(one))
	}
	// A nonsense limit is the full bound rather than an error or an empty page: this is a screen, not a query language.
	none, err := s.Mirrors(ctx, 0)
	if err != nil {
		t.Fatalf("Mirrors with limit 0: %v", err)
	}
	if len(none) != 3 {
		t.Errorf("limit 0 returned %d, want every receipt", len(none))
	}
}

// A receipt with no timestamp is stamped by the store's own clock, so a caller that only knows what happened does not
// have to know when.
func TestMirrorStampsAnEmptyTimestamp(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.RecordMirror(ctx, domain.Mirror{ID: "run-1:db", Target: "t", Status: domain.MirrorOK}); err != nil {
		t.Fatalf("RecordMirror: %v", err)
	}
	got, err := s.Mirrors(ctx, 1)
	if err != nil {
		t.Fatalf("Mirrors: %v", err)
	}
	if len(got) != 1 || !got[0].RanAt.Equal(fixedNow()) {
		t.Fatalf("mirrors = %+v, want the store's clock", got)
	}
}

// Sad: a closed database is an error the screen can report, not a panic or an empty list that reads as "never ran".
func TestMirrorsOnAClosedStore(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := s.Mirrors(ctx, 10); err == nil {
		t.Fatal("Mirrors on a closed store returned no error")
	}
	if err := s.RecordMirror(ctx, mirror("run-1:db", "t", domain.MirrorOK, fixedNow(), 1)); err == nil {
		t.Fatal("RecordMirror on a closed store returned no error")
	}
}
