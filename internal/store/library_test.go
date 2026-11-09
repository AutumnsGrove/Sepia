package store

import (
	"context"
	"strings"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The reading screens' store work: the library's second order, the two filters they added, the Deleted list and its
// way back, and the settings row the pre-read switch lives in. Every list is walked a page at a time, because a
// cursor that repeats or skips a row is the one bug a screenshot can never show.

// savedEntry writes a saved entry with the finish date and rating the case needs. An empty finishedOn is a real
// case: a rating and a body are the only things a confirm requires (docs/ENTRY_CARD.md 2).
func savedEntry(t *testing.T, s *Store, id, workID, finishedOn string, rating int) {
	t.Helper()
	e := sampleEntry(id, workID, rating)
	e.Status = domain.StatusSaved
	e.FinishedOn = finishedOn
	if err := s.CreateDraft(context.Background(), e); err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
}

func ids(entries []domain.Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

// walk collects every row of a list by following the cursor, which is the only way a page test can prove the order is
// total rather than merely plausible on page one.
func walk(t *testing.T, s *Store, q domain.LibraryQuery, limit int) []string {
	t.Helper()
	ctx := context.Background()
	var all []string
	cursor := domain.Cursor{}
	for page := 0; page < 20; page++ {
		entries, next, err := s.LibraryPage(ctx, q, cursor, limit)
		if err != nil {
			t.Fatalf("page %d: %v", page, err)
		}
		all = append(all, ids(entries)...)
		if next.Empty() {
			return all
		}
		cursor = next
	}
	t.Fatal("the list never ended; a cursor is repeating")
	return nil
}

// Happy: both orders walk every row exactly once, one page at a time.
func TestLibraryPagesAreTotal(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.SaveWork(ctx, sampleWork("w1")); err != nil {
		t.Fatal(err)
	}
	savedEntry(t, s, "e1", "w1", "2026-10-30", 18)
	savedEntry(t, s, "e2", "w1", "2026-10-02", 16)
	savedEntry(t, s, "e3", "w1", "2026-09-14", 18)
	savedEntry(t, s, "e4", "w1", "2026-03-12", 15)

	cases := map[string]struct {
		sort domain.LibrarySort
		want []string
	}{
		"newest": {domain.SortNewest, []string{"e1", "e2", "e3", "e4"}},
		// e1 and e3 share a rating, and fixedNow gives them one created_at, so the id breaks the tie.
		"rating": {domain.SortRating, []string{"e3", "e1", "e2", "e4"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := walk(t, s, domain.LibraryQuery{Sort: c.sort}, 3)
			if strings.Join(got, ",") != strings.Join(c.want, ",") {
				t.Fatalf("order = %v, want %v", got, c.want)
			}
		})
	}
}

// Sad: a saved entry with no finish date is still in the library, last, and the page after it is reached rather than
// skipped. The old query compared the cursor against a null, so this row and everything after it disappeared.
func TestLibraryPagesAnUndatedEntry(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.SaveWork(ctx, sampleWork("w1")); err != nil {
		t.Fatal(err)
	}
	savedEntry(t, s, "e1", "w1", "2026-10-30", 18)
	savedEntry(t, s, "e2", "w1", "", 16)
	savedEntry(t, s, "e3", "w1", "", 14)

	got := walk(t, s, domain.LibraryQuery{}, 1)
	if strings.Join(got, ",") != "e1,e3,e2" {
		t.Fatalf("order = %v, want the dated entry first and the undated ones by id, descending", got)
	}
}

// Bad: the sort key is never null, so a page boundary inside one rating neither repeats nor skips a row even when the
// finish dates are missing or equal.
func TestRatingOrderBreaksTiesOnCreatedAt(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.SaveWork(ctx, sampleWork("w1")); err != nil {
		t.Fatal(err)
	}
	savedEntry(t, s, "e1", "w1", "2026-10-30", 18)
	savedEntry(t, s, "e2", "w1", "", 18)
	savedEntry(t, s, "e3", "w1", "2026-10-02", 18)
	// Same rating, so the order is when each was logged: e1 was logged last and comes first.
	for id, at := range map[string]string{"e1": "2026-10-31T09:00:00Z", "e2": "2026-10-31T10:00:00Z", "e3": "2026-10-31T11:00:00Z"} {
		if err := raw(t, s, `UPDATE entries SET created_at = ? WHERE id = ?`, at, id); err != nil {
			t.Fatalf("set created_at for %s: %v", id, err)
		}
	}
	got := walk(t, s, domain.LibraryQuery{Sort: domain.SortRating}, 1)
	if strings.Join(got, ",") != "e3,e2,e1" {
		t.Fatalf("order = %v, want the most recently logged first", got)
	}
}

// Happy: the two filters the entry page needed. Entries on one work, and a page that holds nothing else.
func TestLibraryFiltersByWorkAndMedium(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.SaveWork(ctx, sampleWork("w1")); err != nil {
		t.Fatal(err)
	}
	other := sampleWork("w2")
	other.Title = "Piranesi"
	if err := s.SaveWork(ctx, other); err != nil {
		t.Fatal(err)
	}
	savedEntry(t, s, "e1", "w1", "2026-10-30", 18)
	savedEntry(t, s, "e2", "w1", "2024-01-15", 16)
	savedEntry(t, s, "e3", "w2", "2026-09-28", 19)

	onWork, _, err := s.LibraryPage(ctx, domain.LibraryQuery{Work: "w1"}, domain.Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(ids(onWork), ","); got != "e1,e2" {
		t.Fatalf("entries on w1 = %s", got)
	}

	// Bad: a work filter and a medium filter that disagree return nothing rather than everything.
	none, _, err := s.LibraryPage(ctx, domain.LibraryQuery{Work: "w1", Medium: "film"}, domain.Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("a contradictory filter returned %v", ids(none))
	}
}

// Happy and sad: the Deleted list holds only deleted entries, newest deletion first, and restore is the way back.
func TestDeletedListAndRestore(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()
	if err := s.SaveWork(ctx, sampleWork("w1")); err != nil {
		t.Fatal(err)
	}
	savedEntry(t, s, "e1", "w1", "2026-10-30", 18)
	savedEntry(t, s, "e2", "w1", "2026-10-02", 16)
	savedEntry(t, s, "e3", "w1", "2026-09-14", 18)

	if err := s.SoftDeleteEntry(ctx, "e1"); err != nil {
		t.Fatalf("delete e1: %v", err)
	}
	// The second deletion happens a day later, so the Deleted list has a real order to prove.
	if err := raw(t, s, `UPDATE entries SET deleted_at = '2026-10-30T21:00:00Z' WHERE id = 'e1'`); err != nil {
		t.Fatal(err)
	}
	if err := s.SoftDeleteEntry(ctx, "e3"); err != nil {
		t.Fatalf("delete e3: %v", err)
	}
	if err := raw(t, s, `UPDATE entries SET deleted_at = '2026-11-01T08:00:00Z' WHERE id = 'e3'`); err != nil {
		t.Fatal(err)
	}

	deleted := walk(t, s, domain.LibraryQuery{Deleted: true}, 1)
	if strings.Join(deleted, ",") != "e3,e1" {
		t.Fatalf("deleted = %v, want the most recently deleted first", deleted)
	}
	library, _, err := s.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids(library), ",") != "e2" {
		t.Fatalf("library = %v, want only the entry that is still there", ids(library))
	}

	if err := s.RestoreEntry(ctx, "e1"); err != nil {
		t.Fatalf("restore e1: %v", err)
	}
	library, _, err = s.LibraryPage(ctx, domain.LibraryQuery{}, domain.Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids(library), ",") != "e1,e2" {
		t.Fatalf("library after restore = %v", ids(library))
	}
	deleted = walk(t, s, domain.LibraryQuery{Deleted: true}, 10)
	if strings.Join(deleted, ",") != "e3" {
		t.Fatalf("deleted after restore = %v", deleted)
	}
	// A restored entry is back in its old place, and its old finish date is what puts it there.
	if got, entryErr := s.Entry(ctx, "e1"); entryErr != nil || got.Deleted {
		t.Fatalf("e1 = %+v %v", got, entryErr)
	}
}

// Happy and bad: the settings row, its default, and a write that replaces rather than duplicating.
func TestSettingsRoundTrip(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	value, err := s.Setting(ctx, "preread", "on")
	if err != nil {
		t.Fatalf("Setting: %v", err)
	}
	if value != "on" {
		t.Fatalf("a setting that was never written = %q, want the default", value)
	}
	if err := s.SetSetting(ctx, "preread", "off"); err != nil {
		t.Fatalf("SetSetting: %v", err)
	}
	if value, err = s.Setting(ctx, "preread", "on"); err != nil || value != "off" {
		t.Fatalf("Setting = %q %v, want off", value, err)
	}
	// The second attempt updates the row: one key, one row, whatever the number of writes.
	if err := s.SetSetting(ctx, "preread", "on"); err != nil {
		t.Fatalf("SetSetting again: %v", err)
	}
	if value, err = s.Setting(ctx, "preread", "off"); err != nil || value != "on" {
		t.Fatalf("Setting = %q %v, want on", value, err)
	}
	var count int
	if err := s.readers.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("settings rows = %d, want 1", count)
	}
}
