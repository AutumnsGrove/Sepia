package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// movingTest opens a store whose clock a test can move, which is what a version needs to change: openTest's clock is
// fixed so that a timestamp is never the reason a test differs from the last run.
func movingTest(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	now := fixedNow()
	var next int
	s, err := Open(context.Background(), Options{
		Path:  filepath.Join(t.TempDir(), "sepia.db"),
		Now:   func() time.Time { return now },
		NewID: func() string { next++; return fmt.Sprintf("test-id-%d", next) },
	})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.VerifyTriggers(context.Background()); err != nil {
		t.Fatalf("VerifyTriggers: %v", err)
	}
	return s, &now
}

// S16.4: a change proposed from a snapshot she has seen is written only if the row is still at that version. The
// comparison is inside the write's transaction, so a save from another screen cannot slip between them.

// savedVersion is the entry at e1, which seed() writes.
func savedVersion(t *testing.T, s *Store) domain.Entry {
	t.Helper()
	entry, err := s.Entry(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	return entry
}

func TestSaveEntryAtWritesTheVersionSheSaw(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	ctx := context.Background()

	shown := savedVersion(t, s)
	shown.Rating = 20
	revision, err := s.SaveEntryAt(ctx, shown, shown.UpdatedAt)
	if err != nil {
		t.Fatalf("SaveEntryAt: %v", err)
	}
	if revision == 0 {
		t.Fatal("no revision was kept for the change")
	}
	if after := savedVersion(t, s); after.Rating != 20 {
		t.Fatalf("rating = %d, want 20", after.Rating)
	}
}

func TestSaveEntryAtRefusesAVersionThatMoved(t *testing.T) {
	s, now := movingTest(t)
	seed(t, s)
	ctx := context.Background()

	// She is shown 4.25, and the change is proposed from that.
	shown := savedVersion(t, s)
	proposed := shown
	proposed.Rating = 20

	// Another screen saves first, so the row is no longer the one the proposal was drawn from.
	other := shown
	other.Rating = 16
	*now = now.Add(time.Minute)
	if _, err := s.SaveEntry(ctx, other); err != nil {
		t.Fatalf("the other screen's save: %v", err)
	}

	if _, err := s.SaveEntryAt(ctx, proposed, shown.UpdatedAt); !errors.Is(err, ErrStaleVersion) {
		t.Fatalf("err = %v, want ErrStaleVersion", err)
	}
	if after := savedVersion(t, s); after.Rating != 16 {
		t.Fatalf("rating = %d, want 16: the change she was shown was overwritten", after.Rating)
	}
}

func TestSaveEntryAtRefusesAnEntryThatIsGone(t *testing.T) {
	s := openTest(t)
	seed(t, s)
	ctx := context.Background()

	_, err := s.SaveEntryAt(ctx, domain.Entry{
		ID: "e-missing", WorkID: "w1", Rating: 18, RatingScale: domain.RatingScale,
	}, time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
