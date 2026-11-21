package domain

import (
	"errors"
	"strings"
	"testing"
)

// The completion state is the second dimension on an entry: what she has to say about the work, kept apart from the
// lifecycle of the card. These tests are the ones that say a dropped book can be saved without a rating and a book
// she means to read can be saved without anything but the work.

// abandonedEntry is a dropped book: her words, no rating, and how far she got.
func abandonedEntry(t *testing.T) Entry {
	t.Helper()
	source := "started the lantern keeper but honestly i couldn't get into it, about a hundred pages in"
	got := Locate(source, []string{"i couldn't get into it"})
	if len(got.Spans) != 1 {
		t.Fatalf("fixture spans = %+v", got.Spans)
	}
	return Entry{
		Completion:  CompletionAbandoned,
		Body:        Compose(source, got.Spans),
		BodyOrigin:  BodyExtracted,
		BodySpans:   got.Spans,
		Progress:    "about a hundred pages in",
		FinishedOn:  "2026-11-19",
		Status:      StatusDraft,
		RatingScale: RatingScale,
	}
}

func TestCompletionStateReadsEmptyAsFinished(t *testing.T) {
	cases := []struct {
		name string
		in   Completion
		want Completion
	}{
		{"empty is finished", "", CompletionFinished},
		{"finished", CompletionFinished, CompletionFinished},
		{"abandoned", CompletionAbandoned, CompletionAbandoned},
		{"want to", CompletionWantTo, CompletionWantTo},
		{"unknown reads as finished", Completion("nonsense"), CompletionFinished},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := (Entry{Completion: c.in}).CompletionState(); got != c.want {
				t.Fatalf("CompletionState() = %q, want %q", got, c.want)
			}
		})
	}
}

// --- the guard ---------------------------------------------------------------------------------------------

func TestValidateForSaveAbandonedNeedsNoRating(t *testing.T) {
	e := abandonedEntry(t)
	if e.Rating != 0 {
		t.Fatalf("fixture is rated: %d", e.Rating)
	}
	if err := ValidateForSave(e, abandonSource); err != nil {
		t.Fatalf("an abandoned entry was refused: %v", err)
	}
	ok, missing := CanConfirm(e, abandonSource)
	if !ok || len(missing) != 0 {
		t.Fatalf("CanConfirm = %v %v, want true", ok, missing)
	}
}

func TestValidateForSaveAbandonedStillNeedsABody(t *testing.T) {
	e := abandonedEntry(t)
	e.Body = "   "
	e.BodySpans = nil
	err := ValidateForSave(e, abandonSource)
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeBodyRequired {
		t.Fatalf("err = %v, want %s", err, CodeBodyRequired)
	}
}

func TestValidateForSaveWantedNeedsNothing(t *testing.T) {
	e := Entry{Completion: CompletionWantTo, Status: StatusDraft, RatingScale: RatingScale}
	if err := ValidateForSave(e, ""); err != nil {
		t.Fatalf("a wanted work was refused: %v", err)
	}
	if ok, missing := CanConfirm(e, ""); !ok || len(missing) != 0 {
		t.Fatalf("CanConfirm = %v %v, want true", ok, missing)
	}
}

func TestValidateForSaveFinishedStillNeedsARating(t *testing.T) {
	e := abandonedEntry(t)
	e.Completion = CompletionFinished
	err := ValidateForSave(e, abandonSource)
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeRatingRequired {
		t.Fatalf("err = %v, want %s", err, CodeRatingRequired)
	}
}

func TestValidateForSaveRefusesAnUnknownCompletion(t *testing.T) {
	e := extractedEntry(t)
	e.Completion = Completion("halfway")
	err := ValidateForSave(e, session)
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeCompletionInvalid {
		t.Fatalf("err = %v, want %s", err, CodeCompletionInvalid)
	}
}

// --- how far I got -----------------------------------------------------------------------------------------

func TestValidateForSaveRefusesAProgressThatIsNotHerWords(t *testing.T) {
	e := abandonedEntry(t)
	e.Progress = "I gave up around page 250"
	err := ValidateForSave(e, abandonSource)
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeProgressNotYourWords {
		t.Fatalf("err = %v, want %s", err, CodeProgressNotYourWords)
	}
}

func TestValidateForSaveBoundsTheProgress(t *testing.T) {
	e := abandonedEntry(t)
	e.BodyOrigin = BodyTyped
	e.Progress = strings.Repeat("a", MaxProgressLength+1)
	err := ValidateForSave(e, "")
	var guard *GuardError
	if !errors.As(err, &guard) || guard.Code != CodeProgressTooLong {
		t.Fatalf("err = %v, want %s", err, CodeProgressTooLong)
	}
}

func TestValidateForSaveIgnoresProgressOnAFinishedEntry(t *testing.T) {
	// A finished entry does not show a how-far line, so a stale one is not her opinion about anything. It must not
	// block the save either, or a card she finished would be frozen by a field she cannot see.
	e := extractedEntry(t)
	e.Progress = "I wrote this on the train"
	if err := ValidateForSave(e, session); err != nil {
		t.Fatalf("a stale progress blocked a finished entry: %v", err)
	}
}

func TestNormalizeClearsProgressUnlessAbandoned(t *testing.T) {
	finished := extractedEntry(t)
	finished.Progress = "halfway"
	if got := finished.Normalized(); got.Progress != "" {
		t.Fatalf("progress on a finished entry = %q, want empty", got.Progress)
	}
	wanted := Entry{Completion: CompletionWantTo, Progress: "halfway"}
	if got := wanted.Normalized(); got.Progress != "" {
		t.Fatalf("progress on a wanted entry = %q, want empty", got.Progress)
	}
	abandoned := abandonedEntry(t)
	if got := abandoned.Normalized(); got.Progress != abandoned.Progress {
		t.Fatalf("progress on an abandoned entry = %q, want %q", got.Progress, abandoned.Progress)
	}
}

// --- the diff ----------------------------------------------------------------------------------------------

func TestDiffNamesTheCompletionAndTheProgress(t *testing.T) {
	before := extractedEntry(t)
	after := abandonedEntry(t)
	changes := Diff(before, after)
	fields := make([]string, 0, len(changes))
	for _, c := range changes {
		fields = append(fields, c.Field)
	}
	joined := strings.Join(fields, ",")
	if !strings.Contains(joined, "completion") {
		t.Errorf("diff fields = %s, want a completion change", joined)
	}
	if !strings.Contains(joined, "progress") {
		t.Errorf("diff fields = %s, want a progress change", joined)
	}
}

// --- the cues ----------------------------------------------------------------------------------------------

const abandonSource = "started the lantern keeper but honestly i couldn't get into it, about a hundred pages in"

func TestCompletionCues(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Completion
		ok   bool
	}{
		{"no cue", "the middle dragged a bit but the end stayed with me", "", false},
		{"finished out loud", "finished the lantern keeper last night, 4.5", CompletionFinished, true},
		{"read it", "i read it in two nights and loved it", CompletionFinished, true},
		{"couldn't get into it", "i couldn't get into the lantern keeper", CompletionAbandoned, true},
		{"gave up", "i gave up on the salt road about halfway", CompletionAbandoned, true},
		{"didn't finish", "didn't finish the bear season three", CompletionAbandoned, true},
		{"dnf", "dnf on piranesi, not for me", CompletionAbandoned, true},
		{"put it down", "i put it down after two chapters", CompletionAbandoned, true},
		{"stopped watching", "stopped watching after two episodes", CompletionAbandoned, true},
		{"want to read", "want to read piranesi next", CompletionWantTo, true},
		{"on my list", "the salt road is on my list", CompletionWantTo, true},
		{"tbr", "adding this to the tbr", CompletionWantTo, true},
		{"want to watch", "i want to watch dune part two", CompletionWantTo, true},

		// A message that says both names the stronger claim about the thing (decision 7).
		{"both, abandoned wins", "i want to read it but i couldn't get into it", CompletionAbandoned, true},

		// Negation and cross-talk must not flip a card.
		{"negated want", "i don't want to read that", "", false},
		{"negated finish", "i haven't finished it yet", "", false},
		// One sentence doing two jobs is read as the work she is logging, which is the one she finished.
		{"finished then want the sequel", "finished dune and now i want to read messiah", CompletionFinished, true},
		{"want to read it is not a past tense", "i want to read it", CompletionWantTo, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := CompletionCues(c.text)
			if ok != c.ok {
				t.Fatalf("CompletionCues(%q) ok = %v, want %v (value %q)", c.text, ok, c.ok, got)
			}
			if ok && got != c.want {
				t.Fatalf("CompletionCues(%q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

func TestCompletionCuesIsCaseAndPunctuationTolerant(t *testing.T) {
	if got, ok := CompletionCues("Couldn't get into it."); !ok || got != CompletionAbandoned {
		t.Fatalf("got %q %v", got, ok)
	}
	if got, ok := CompletionCues("DNF."); !ok || got != CompletionAbandoned {
		t.Fatalf("got %q %v", got, ok)
	}
}
