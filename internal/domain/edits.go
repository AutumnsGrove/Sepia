package domain

import "time"

// Edits is what the library chat may change about a saved entry. A nil field is not part of the edit, so "leave the
// rating alone" and "set the rating to nothing" stay different questions, and the one the chat never asks is the
// second: dropping a rating is a thing she does on the card, where she can see what she is losing.
//
// The body, the verdict and their spans are deliberately absent. The chat can read her words and can point at an
// entry; it cannot write them (docs/BUILD.md invariant 1), and an edit that cannot name a field cannot change it.
type Edits struct {
	Rating     *int
	Completion *Completion
	Favourite  *bool
	Format     *string
	FinishedOn *string
}

// Empty reports whether the edit asks for nothing, which is how the chat knows it has not understood a request.
func (e Edits) Empty() bool {
	return e.Rating == nil && e.Completion == nil && e.Favourite == nil && e.Format == nil && e.FinishedOn == nil
}

// ApplyEdits returns the entry with the edit applied, or a guard error naming what is wrong.
//
// It checks the fields the edit names rather than re-running ValidateForSave: a saved entry's body was proven when it
// was saved, and there is no source text left on the server to prove it against again. What it does keep is every rule
// that still applies after an edit, so an edit can never leave an entry that could not have been saved: a finished
// entry keeps a rating, and an abandoned one keeps the words it needs.
func ApplyEdits(e Entry, edits Edits) (Entry, error) {
	if edits.Empty() {
		return Entry{}, &GuardError{Code: CodeNoChange, Message: "There is nothing in that change."}
	}
	out := e
	if edits.Rating != nil {
		if !ValidRating(*edits.Rating) {
			return Entry{}, &GuardError{Code: CodeRatingInvalid, Message: "That is not a rating Sepia can store."}
		}
		out.Rating = *edits.Rating
	}
	if edits.Completion != nil {
		if !validCompletion(*edits.Completion) {
			return Entry{}, &GuardError{Code: CodeCompletionInvalid, Message: "That is not how far you got."}
		}
		out.Completion = *edits.Completion
	}
	if edits.Favourite != nil {
		out.Favourite = *edits.Favourite
	}
	if edits.Format != nil {
		if !KnownFormat(*edits.Format) {
			return Entry{}, &GuardError{Code: CodeFormatInvalid, Message: "That is not a format Sepia knows."}
		}
		out.Format = *edits.Format
	}
	if edits.FinishedOn != nil {
		if *edits.FinishedOn != "" {
			if _, err := time.Parse(dateLayout, *edits.FinishedOn); err != nil {
				return Entry{}, &GuardError{Code: CodeDateInvalid, Message: "That is not a date."}
			}
		}
		out.FinishedOn = *edits.FinishedOn
	}

	// The rules that survive an edit, in the same words the card uses.
	switch out.CompletionState() {
	case CompletionFinished:
		if !ValidRating(out.Rating) {
			return Entry{}, &GuardError{Code: CodeRatingRequired, Message: "Add a rating before saving."}
		}
	case CompletionAbandoned:
		if out.Body == "" {
			return Entry{}, &GuardError{Code: CodeBodyRequired, Message: "Add a few words of your own before saving."}
		}
	case CompletionWantTo:
		// Handled below with the rest of the wish rules, so the switch stays exhaustive.
	}
	// A wish keeps no opinion of its own: a rating or a finish date on a work she has not read would be a score it
	// never had, and the card and the row would both draw it.
	if out.CompletionState() == CompletionWantTo {
		out.Rating = 0
		out.FinishedOn = ""
		out.Progress = ""
		out.Favourite = false
	}
	return out, nil
}

// KnownFormat reports whether a format is one the app knows how to read. Empty is known: a card may have none.
func KnownFormat(format string) bool {
	if format == "" {
		return true
	}
	for _, cue := range formatCues {
		if cue.format == format {
			return true
		}
	}
	return false
}

// EditChange is one field an edit would change, as the confirm card shows it: the label she reads, and the two values.
// It is the same shape Diff uses for a revision, so an edit read in the chat and the version list describe the same
// change the same way.
type EditChange struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// ChangesOf lists what ApplyEdits changed, in the order the card shows its facts. The before and after values are the
// text she sees on the card, so the confirm cannot disagree with the entry it is about.
func ChangesOf(before, after Entry) []EditChange {
	out := make([]EditChange, 0, 5)
	add := func(field, label, b, a string) {
		if b != a {
			out = append(out, EditChange{Field: field, Label: label, Before: b, After: a})
		}
	}
	add("rating", "Rating", FormatRating(before.Rating), FormatRating(after.Rating))
	add("completion", "Shelf", string(before.CompletionState()), string(after.CompletionState()))
	add("favourite", "Favourite", boolString(before.Favourite), boolString(after.Favourite))
	add("format", "Format", before.Format, after.Format)
	add("finished_on", "Date", before.FinishedOn, after.FinishedOn)
	return out
}
