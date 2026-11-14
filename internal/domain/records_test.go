package domain

import "testing"

// ParseLibrarySort reads a query value. An empty value is the default; the two known orders are accepted; anything
// else is refused rather than quietly defaulted, because a list answered in an order the caller did not ask for is a
// page from a different query than its cursor names.
func TestParseLibrarySort(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want LibrarySort
		ok   bool
	}{
		{"empty is the default", "", SortNewest, true},
		{"newest", "newest", SortNewest, true},
		{"rating", "rating", SortRating, true},
		{"case matters", "RATING", "", false},
		{"no free-form sql", "'; drop table entries; --", "", false},
		{"no whitespace padding", " rating ", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseLibrarySort(tc.raw)
			if ok != tc.ok {
				t.Fatalf("ParseLibrarySort(%q) ok = %v, want %v", tc.raw, ok, tc.ok)
			}
			if got != tc.want {
				t.Errorf("ParseLibrarySort(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// A cursor with no id is the first page, and nothing else is.
func TestCursorEmptyIsOnlyTheFirstPage(t *testing.T) {
	if !(Cursor{}).Empty() {
		t.Error("an empty cursor is not reported as the first page")
	}
	if (Cursor{ID: "abc"}).Empty() {
		t.Error("a cursor with an id is reported as the first page")
	}
}

// An interaction is open until a sequence number resolves it, and closed once one does, including sequence zero.
func TestInteractionIsOpenUntilResolved(t *testing.T) {
	if !(Interaction{}).Open() {
		t.Error("an interaction with no resolved sequence is not reported as open")
	}
	zero := 0
	if (Interaction{ResolvedSeq: &zero}).Open() {
		t.Error("an interaction resolved at sequence zero is still reported as open")
	}
	seven := 7
	if (Interaction{ResolvedSeq: &seven}).Open() {
		t.Error("a resolved interaction is still reported as open")
	}
}
