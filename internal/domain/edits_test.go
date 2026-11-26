package domain

import (
	"errors"
	"testing"
)

// S16.4: what the library chat may change about an entry. The chat is a second way in, not a second set of rules: an
// edit goes through the same guard as a typed one, and it can never write her words, because a body is hers.

func editable() Entry {
	return Entry{
		ID: "e1", WorkID: "w1", Status: StatusSaved, Completion: CompletionFinished,
		Rating: 17, RatingScale: RatingScale, Favourite: false, Format: FormatPaperback,
		FinishedOn: "2026-10-30", Body: "loved it, the middle dragged", BodyOrigin: BodyTyped,
	}
}

func TestApplyEditsHappy(t *testing.T) {
	rating := 18
	updated, err := ApplyEdits(editable(), Edits{Rating: &rating})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if updated.Rating != 18 || updated.Body != "loved it, the middle dragged" {
		t.Fatalf("entry = %+v, want the rating changed and the words untouched", updated)
	}
	if updated.Completion != CompletionFinished || updated.FinishedOn != "2026-10-30" {
		t.Fatalf("entry = %+v, want the fields the edit did not name left alone", updated)
	}

	// Every editable field at once, which is what a chat answer like "it was a hardback, and a favourite" reaches.
	completion := CompletionAbandoned
	favourite := true
	format := FormatHardback
	day := "2026-11-03"
	all, err := ApplyEdits(editable(), Edits{
		Rating: &rating, Completion: &completion, Favourite: &favourite, Format: &format, FinishedOn: &day,
	})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if all.Completion != CompletionAbandoned || !all.Favourite || all.Format != FormatHardback ||
		all.FinishedOn != "2026-11-03" || all.Rating != 18 {
		t.Fatalf("entry = %+v, want every named field applied", all)
	}
}

func TestApplyEditsBad(t *testing.T) {
	tooHigh := 21
	zero := 0
	unknownCompletion := Completion("halfway")
	unknownFormat := "hologram"
	notADay := "yesterday"

	cases := map[string]struct {
		edits Edits
		want  string
	}{
		"a rating above five":          {Edits{Rating: &tooHigh}, CodeRatingInvalid},
		"a rating of zero":             {Edits{Rating: &zero}, CodeRatingInvalid},
		"a shelf Sepia does not know":  {Edits{Completion: &unknownCompletion}, CodeCompletionInvalid},
		"a format Sepia does not know": {Edits{Format: &unknownFormat}, CodeFormatInvalid},
		"a date that is not a day":     {Edits{FinishedOn: &notADay}, CodeDateInvalid},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ApplyEdits(editable(), tc.edits)
			var guard *GuardError
			if !errors.As(err, &guard) || guard.Code != tc.want {
				t.Fatalf("err = %v, want %s", err, tc.want)
			}
		})
	}
}

// A finished entry needs a rating, and dropping a rating is not something a chat may do: the guard that holds for a
// typed edit holds here, so an edit that would leave the entry unsavable is refused before anything is written.
func TestApplyEditsSadAnEditThatWouldBreakTheEntry(t *testing.T) {
	if _, err := ApplyEdits(editable(), Edits{Rating: intp(0)}); err == nil {
		t.Fatal("a finished entry with no rating was accepted")
	}
	// A rated import has no words at all, so stopping partway through it would leave an abandoned entry with nothing
	// said about it. The card asks for a line first, and so does the chat.
	bare := editable()
	bare.Body = ""
	abandoned := CompletionAbandoned
	_, err := ApplyEdits(bare, Edits{Completion: &abandoned})
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeBodyRequired {
		t.Fatalf("err = %v, want %s", err, CodeBodyRequired)
	}
	// Moving it to the wanted shelf is allowed, and takes the score and the date with it: a wish has neither.
	wanted := CompletionWantTo
	wish, err := ApplyEdits(bare, Edits{Completion: &wanted})
	if err != nil {
		t.Fatalf("ApplyEdits to want_to: %v", err)
	}
	if wish.Rating != 0 || wish.FinishedOn != "" {
		t.Fatalf("wish = %+v, want no rating and no date", wish)
	}
}

func intp(v int) *int { return &v }

// The diff the confirm card draws. It names only what changed, in the order the card shows its facts, and it uses the
// words the card uses, so the chat cannot describe a change the entry page would describe differently.
func TestChangesOfNamesOnlyWhatChanged(t *testing.T) {
	before := editable()
	newRating := 18
	favourite := true
	format := FormatDigitalFormat()
	day := "2026-11-03"
	after, err := ApplyEdits(before, Edits{Rating: &newRating, Favourite: &favourite, Format: &format, FinishedOn: &day})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	changes := ChangesOf(before, after)
	fields := make([]string, 0, len(changes))
	for _, change := range changes {
		fields = append(fields, change.Field)
		if change.Label == "" || change.Before == change.After {
			t.Fatalf("change = %+v, want a label and two different values", change)
		}
	}
	want := []string{"rating", "favourite", "format", "finished_on"}
	if len(fields) != len(want) {
		t.Fatalf("fields = %v, want %v", fields, want)
	}
	for i := range want {
		if fields[i] != want[i] {
			t.Fatalf("fields = %v, want %v", fields, want)
		}
	}
	if changes[0].Before != "4.25" || changes[0].After != "4.5" {
		t.Fatalf("rating change = %+v, want the words the card shows", changes[0])
	}
	// Nothing changed is no diff at all, which is how the chat answers "it is already 4.5".
	if got := ChangesOf(before, before); len(got) != 0 {
		t.Fatalf("changes = %+v, want none", got)
	}
}

// A format is a closed set, and empty is one of the values: a card may have none.
func TestKnownFormat(t *testing.T) {
	if !KnownFormat("") || !KnownFormat(FormatPaperback) || !KnownFormat(FormatCinema) {
		t.Fatal("a known format was refused")
	}
	if KnownFormat("hologram") {
		t.Fatal("an unknown format was accepted")
	}
}

// FormatDigitalFormat is the format this medium uses in the fixture, kept as a helper so the test reads plainly.
func FormatDigitalFormat() string { return FormatEbook }

// The branches the three routes above do not reach: clearing a date, and the two ways a date or a format is refused
// after the field is set. They are small, but each is a rule the chat must not get wrong, so each is asked.
func TestApplyEditsEdges(t *testing.T) {
	// Clearing the date is a real edit: "I do not remember when" is a thing she can say.
	empty := ""
	cleared, err := ApplyEdits(editable(), Edits{FinishedOn: &empty})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	if cleared.FinishedOn != "" {
		t.Fatalf("finished_on = %q, want it cleared", cleared.FinishedOn)
	}
	// A format that is not one Sepia knows, named with a rating that is fine, so the refusal is the format's.
	bad := "hologram"
	rating := 18
	if _, err := ApplyEdits(editable(), Edits{Format: &bad, Rating: &rating}); err == nil {
		t.Fatal("an unknown format was accepted")
	}
	// A finished entry rated to nothing, and an abandoned one with nothing said: the two rules that survive an edit.
	abandoned := editable()
	abandoned.Completion = CompletionAbandoned
	abandoned.Body = ""
	if _, err := ApplyEdits(abandoned, Edits{Completion: &[]Completion{CompletionAbandoned}[0]}); err == nil {
		t.Fatal("an abandoned entry with no words was accepted")
	}
}

// An abandoned entry that is moved to the wanted shelf loses its how-far line, and the card has to say so: a confirm
// that hides half of what it does is not a confirm.
func TestChangesOfShowsTheHowFarLineAWishDrops(t *testing.T) {
	stopped := editable()
	stopped.Completion = CompletionAbandoned
	stopped.Rating = 0
	stopped.Progress = "about a hundred pages in"
	wanted := CompletionWantTo
	after, err := ApplyEdits(stopped, Edits{Completion: &wanted})
	if err != nil {
		t.Fatalf("ApplyEdits: %v", err)
	}
	labels := map[string]domainChange{}
	for _, change := range ChangesOf(stopped, after) {
		labels[change.Field] = domainChange{change.Before, change.After}
	}
	howFar, ok := labels["progress"]
	if !ok {
		t.Fatalf("changes = %+v, want the how-far line in it", ChangesOf(stopped, after))
	}
	if howFar.before != "about a hundred pages in" || howFar.after != "" {
		t.Fatalf("how far = %+v, want it shown as dropped", howFar)
	}
}

type domainChange struct{ before, after string }

// An empty edit is a refusal rather than a no-op, so a chat that understood nothing cannot write nothing successfully.
func TestApplyEditsRefusesAnEmptyEdit(t *testing.T) {
	_, err := ApplyEdits(editable(), Edits{})
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeNoChange {
		t.Fatalf("err = %v, want %s", err, CodeNoChange)
	}
}

// A finished shelf needs a rating, and an unrated entry moved onto it is refused: finishing something is a claim, and
// a claim with no score is not one the card would accept either.
func TestApplyEditsRefusesAFinishWithNoRating(t *testing.T) {
	unrated := editable()
	unrated.Rating = 0
	unrated.Completion = CompletionWantTo
	finished := CompletionFinished
	_, err := ApplyEdits(unrated, Edits{Completion: &finished})
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeRatingRequired {
		t.Fatalf("err = %v, want %s", err, CodeRatingRequired)
	}
}
