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
