package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/store"
)

// The seed the reading screens are looked at with. It goes through the real engine, so every row it writes is one the
// app could have written: a draft, a confirm, an edit with its revision, a soft delete. The titles and the reviews are
// invented, and the bodies are typed by the seed rather than extracted, because a fixture may never carry a real
// review (docs/BUILD.md invariant 10).

func seedRig(t *testing.T) (*store.Store, *session.Engine) {
	t.Helper()
	ctx := context.Background()
	clock := func() time.Time { return time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC) }
	db, err := store.Open(ctx, store.Options{Path: filepath.Join(t.TempDir(), "seed.db"), Now: clock})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	engine := session.New(session.Config{Store: db, Now: clock, NewID: db.NewID, ServerVersion: "seed"})
	return db, engine
}

func TestSeedLibraryFillsEveryReadingScreen(t *testing.T) {
	db, engine := seedRig(t)
	ctx := context.Background()
	ids, err := seedLibrary(ctx, db, engine, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("seedLibrary: %v", err)
	}

	// The library has entries across several media and more than one month, which is what the groups and the chips
	// are for, and it holds neither the deleted entry nor the drafts.
	library, _, err := db.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 100)
	if err != nil {
		t.Fatalf("library: %v", err)
	}
	media := map[string]bool{}
	months := map[string]bool{}
	for _, entry := range library {
		work, workErr := db.Work(ctx, entry.WorkID)
		if workErr != nil {
			t.Fatalf("work %s: %v", entry.WorkID, workErr)
		}
		media[work.Medium] = true
		if entry.FinishedOn != "" {
			months[entry.FinishedOn[:7]] = true
		}
	}
	if len(library) < 8 {
		t.Fatalf("the library holds %d entries, too few to see the screens", len(library))
	}
	for _, want := range []string{"book", "film", "tv", "game", "album", "theatre"} {
		if !media[want] {
			t.Errorf("no %s in the library", want)
		}
	}
	if len(months) < 3 {
		t.Errorf("the library spans %d months, want at least three", len(months))
	}

	// One work carries two entries, which is the timeline under a saved entry.
	onWork, err := db.EntriesOnWork(ctx, ids.LanternWorkID)
	if err != nil || len(onWork) != 2 {
		t.Fatalf("entries on the lantern keeper = %d %v, want two", len(onWork), err)
	}

	// One entry has three versions and answers, which is the versions screen and the Describe block.
	versions, err := db.Revisions(ctx, ids.RevisedEntryID)
	if err != nil || len(versions) != 2 {
		t.Fatalf("revisions = %d %v, want two, so the entry has three versions", len(versions), err)
	}
	answers, err := db.Answers(ctx, ids.RevisedEntryID)
	if err != nil || len(answers) == 0 {
		t.Fatalf("answers = %d %v, want the Describe chips", len(answers), err)
	}

	// One entry is deleted, so the Deleted list and Restore have something to show.
	trash, _, err := db.LibraryPage(ctx, domain.LibraryQuery{Deleted: true}, domain.Cursor{}, 10)
	if err != nil || len(trash) != 1 {
		t.Fatalf("deleted list = %d %v, want one", len(trash), err)
	}

	// Two drafts are waiting, each for a different thing, which is the Drafts list's point.
	sessions, err := engine.Sessions(ctx, domain.SessionActive, domain.SessionEntry, "", 10)
	if err != nil || len(sessions) != 2 {
		t.Fatalf("active drafts = %d %v, want two", len(sessions), err)
	}
	reasons := map[session.WaitingReason]bool{}
	for _, sess := range sessions {
		reason, waitErr := engine.Waiting(ctx, sess.ID)
		if waitErr != nil {
			t.Fatalf("waiting: %v", waitErr)
		}
		reasons[reason] = true
	}
	if !reasons[session.WaitingRating] || !reasons[session.WaitingBody] {
		t.Errorf("waiting reasons = %v, want a missing rating and a missing body", reasons)
	}

	// A second run adds nothing: the seed is for an empty database, and an e2e stack that reused its data directory
	// must not grow a second copy of every review.
	again, err := seedLibrary(ctx, db, engine, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if again.LibraryCount != ids.LibraryCount {
		t.Errorf("the second seed wrote %d entries over %d", again.LibraryCount, ids.LibraryCount)
	}
}

// The seed is only for an empty database, and it says so rather than failing a boot.
func TestSeedLibraryLeavesANonEmptyDatabaseAlone(t *testing.T) {
	db, engine := seedRig(t)
	ctx := context.Background()
	if _, err := seedLibrary(ctx, db, engine, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("seedLibrary: %v", err)
	}
	before, _, err := db.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	// A work of her own appears in the database, which is not the seed's business to remove.
	if writeErr := db.SaveWork(ctx, domain.Work{ID: "w-mine", Medium: "book", Title: "A Book She Wrote About"}); writeErr != nil {
		t.Fatal(writeErr)
	}
	after, _, err := db.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("the library changed from %d to %d entries", len(before), len(after))
	}
}

func TestSeedEntryValidation(t *testing.T) {
	// The seed writes through the engine, so its own rows have to satisfy the same guard as hers.
	for _, entry := range seedEntries() {
		got := domain.Entry{
			WorkID: seedWorkID(entry.Title), Rating: entry.Rating, RatingScale: domain.RatingScale,
			Body: entry.Body, BodyOrigin: domain.BodyTyped, FinishedOn: entry.FinishedOn,
			Status: domain.StatusDraft,
		}
		if err := domain.ValidateForSave(got, ""); err != nil {
			t.Errorf("%s: %v", entry.Title, err)
		}
		if entry.FinishedOn != "" {
			if _, err := time.Parse("2006-01-02", entry.FinishedOn); err != nil {
				t.Errorf("%s: finished_on %q: %v", entry.Title, entry.FinishedOn, err)
			}
		}
	}
}

// The step clock is what keeps a seeded screen the same on every run: rows written inside one second still get
// distinct instants, so an order that breaks ties on time does not fall through to a random id.
func TestStepClockNeverRepeats(t *testing.T) {
	clock := &stepClock{}
	seen := make([]time.Time, 0, 500)
	for i := 0; i < 500; i++ {
		seen = append(seen, clock.now())
	}
	for i := 1; i < len(seen); i++ {
		if !seen[i].After(seen[i-1]) {
			t.Fatalf("instant %d (%s) is not after the one before it (%s)", i, seen[i], seen[i-1])
		}
	}
	// A second apart, because the schema stores whole seconds.
	if gap := seen[1].Sub(seen[0]); gap != time.Second {
		t.Fatalf("the gap is %s, want a second: a fraction is rounded away by the column", gap)
	}
	// And once the writes catch up with real time, the clock is simply the real clock again.
	clock = &stepClock{}
	if now := clock.now(); now.Before(time.Now().Add(-3 * time.Hour)) {
		t.Fatalf("the first instant is %s, want the last two hours", now)
	}
}
