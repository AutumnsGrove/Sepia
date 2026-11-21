package domain

import "strings"

// Completion is how far she got with a work. It is the second dimension on an entry: Status is the lifecycle of the
// card (draft, then saved), while Completion is what she has to say about the thing itself (a book she means to
// read, one she finished, one she put down). Keeping them apart is the point — a saved entry can be any of the
// three, and a draft is a draft until she confirms it (docs/slices/S21-wishlist-and-unfinished.md).
type Completion string

const (
	// CompletionWantTo is a work on her list that she has not started. It is not a review: there is nothing to rate
	// and nothing she has read yet, so only the work is required.
	CompletionWantTo Completion = "want_to"
	// CompletionFinished is a work she finished, and the ordinary case: it needs a rating and a body.
	CompletionFinished Completion = "finished"
	// CompletionAbandoned is a work she started and stopped. It needs a body and no rating, and may carry one line
	// of how far she got.
	CompletionAbandoned Completion = "abandoned"
)

// MaxProgressLength bounds "how far I got", which is one short line about a book, not a second review.
const MaxProgressLength = 200

// CompletionState reads the completion to act on. An empty value is finished, because every entry that existed
// before the shelves did was one, and because a card JSON that omits the field must not become a wish. An unknown
// value also reads as finished here; ValidateForSave is where a value that is not one of the three is refused, so
// behaviour never guesses and the bad route still has a stable code.
func (e Entry) CompletionState() Completion {
	switch e.Completion {
	case CompletionWantTo, CompletionAbandoned:
		return e.Completion
	default:
		return CompletionFinished
	}
}

// Normalized returns the entry with the fields the completion makes meaningless cleared. "How far I got" is only
// shown for a work she stopped, so a stale line on a finished or wanted entry is not her opinion about anything and
// travels no further. It is a pure function so the caller decides when to apply it.
func (e Entry) Normalized() Entry {
	if e.CompletionState() != CompletionAbandoned {
		e.Progress = ""
	}
	return e
}

// validCompletion refuses a value that is not one of the three, while keeping the empty value valid: an entry
// literal written before the shelves existed, and a card JSON that omits the field, both mean finished.
func validCompletion(c Completion) bool {
	switch c {
	case "", CompletionWantTo, CompletionFinished, CompletionAbandoned:
		return true
	default:
		return false
	}
}

// ParseCompletion reads a completion a client, a tool or the agent sent. The empty value is finished, the same as
// everywhere else, so a caller that omits the field gets the ordinary case rather than a refusal. ok is false for a
// value that is not one of the three.
func ParseCompletion(raw string) (Completion, bool) {
	if !validCompletion(Completion(raw)) {
		return CompletionFinished, false
	}
	return Entry{Completion: Completion(raw)}.CompletionState(), true
}

// --- the cues --------------------------------------------------------------------------------------------------
//
// These are plain rules, not a model (PLAN.md 5 step 3). The same words always give the same answer, the card is
// hers to correct, and the read runs whether or not the decisions backend is configured.

// abandonPhrases are the ways a message says she stopped. Each is a whole phrase on purpose: "halfway" alone is not
// on the list, because "halfway through and loving it" is a finished review with a slow start.
var abandonPhrases = []string{
	"didn't finish",
	"did not finish",
	"didn't manage to finish",
	"didn't get through",
	"couldn't get through",
	"could not get through",
	"couldn't finish",
	"could not finish",
	"couldn't get into",
	"could not get into",
	"didn't get into",
	"can't get into",
	"gave up on",
	"gave up after",
	"gave up halfway",
	"i gave up",
	"dnf",
	"dropped it",
	"dropped this",
	"abandoned it",
	"abandoned this",
	"stopped reading",
	"stopped watching",
	"stopped playing",
	"stopped listening",
	"stopped at",
	"stopped partway",
	"stopped halfway",
	"put it down",
	"put it aside",
	"not for me",
}

// wantPhrases are the ways a message says she means to start something.
var wantPhrases = []string{
	"want to read",
	"want to watch",
	"want to play",
	"want to listen",
	"want to get to",
	"planning to read",
	"planning to watch",
	"planning to play",
	"thinking about reading",
	"thinking about watching",
	"on my list",
	"on the list",
	"reading list",
	"watch list",
	"watchlist",
	"to be read",
	"tbr",
	"next up",
	"can't wait to read",
	"can't wait to watch",
	"looking forward to reading",
	"looking forward to watching",
	"should read",
	"should watch",
}

// saidFinishedPhrases are her saying she finished something, out loud. They are the strong form: they beat a want
// phrase in the same sentence, because "finished dune and now i want to read messiah" is a finished review that
// mentions a wish. They are also what lets a card that started as a wish become a review, since without them only a
// tap on the shelf could move it and she would have said "finished it" for nothing.
var saidFinishedPhrases = []string{
	"finished",
	"done with",
}

// pastPhrases are the weaker ways a sentence can be about something she has already been through. They are not
// enough on their own to beat a want phrase, because the same words turn up in sentences about wanting: "i want to
// read it because i watched the film" is a wish, not a finish. The want phrase's own words are masked out before
// these are checked, because "want to read it" contains "read it".
var pastPhrases = []string{
	"read it",
	"read this",
	"read that",
	"watched it",
	"watched the",
	"saw it",
	"played it",
	"played through",
	"listened to it",
}

// CompletionCues reads how far she got from the words of one message. ok is false when the message named none, so a
// card is never changed by a message that did not mention its shelf.
func CompletionCues(text string) (Completion, bool) {
	lower := strings.ToLower(text)

	// A stopped work is the stronger claim about the thing, so it wins when one message carries both
	// ("i want to read it but i couldn't get into it").
	if anyPhrase(lower, abandonPhrases) {
		return CompletionAbandoned, true
	}

	wantSpans := phraseSpans(lower, wantPhrases)
	masked := lower
	if len(wantSpans) > 0 {
		masked = blank(lower, wantSpans)
	}
	// A wish wins unless she said, out loud, that she finished something. An incidental "read it" is not enough:
	// those words appear in sentences about wanting to read.
	if len(wantSpans) > 0 && !anyPhrase(masked, saidFinishedPhrases) {
		return CompletionWantTo, true
	}
	// She finished it. On a fresh card this is the default anyway; on a card that started as a wish it is the only
	// thing that moves it to a review without her having to find the shelf control.
	if anyPhrase(masked, saidFinishedPhrases) || anyPhrase(masked, pastPhrases) {
		return CompletionFinished, true
	}
	return CompletionFinished, false
}

// span is where a phrase was found, so one phrase's words can be hidden before another pass.
type span struct{ start, end int }

// phraseSpans finds every phrase occurrence that is not negated. A "don't want to read" is not a wish, and a
// substring test alone would say it was.
func phraseSpans(lower string, phrases []string) []span {
	var out []span
	for _, phrase := range phrases {
		from := 0
		for {
			index := strings.Index(lower[from:], phrase)
			if index < 0 {
				break
			}
			start := from + index
			end := start + len(phrase)
			// A one-word phrase has to be a whole word, or "finished" matches inside "unfinished". A phrase of several
			// words is specific enough on its own.
			whole := strings.ContainsRune(phrase, ' ') || wholeWordAt(lower, start, end)
			if whole && !negated(lower, start) {
				out = append(out, span{start: start, end: end})
			}
			from = end
		}
	}
	return out
}

func anyPhrase(lower string, phrases []string) bool {
	return len(phraseSpans(lower, phrases)) > 0
}

// wholeWordAt reports whether the bytes either side of a match are word boundaries, so a one-word cue cannot match a
// fragment of a longer word.
func wholeWordAt(text string, start, end int) bool {
	if start > 0 && isWordByte(text[start-1]) {
		return false
	}
	if end < len(text) && isWordByte(text[end]) {
		return false
	}
	return true
}

func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '\''
}

// negated reports whether a negator sits immediately before the phrase. The window is short on purpose: "finished
// dune. i don't want to read messiah" must not read as a negation of "finished".
func negated(lower string, start int) bool {
	const window = 12
	from := start - window
	if from < 0 {
		from = 0
	}
	before := lower[from:start]
	for _, negator := range []string{"n't ", "not ", "no ", "never ", "don't", "do not", "didn't"} {
		if strings.Contains(before, negator) {
			return true
		}
	}
	return false
}

// blank replaces the matched ranges with spaces and leaves every other byte where it was. Masking rather than
// deleting is deliberate: deleting "want to read" from "want to read it" would leave " it", and deleting could
// join two neighbours into a phrase that was never there.
func blank(text string, spans []span) string {
	out := []byte(text)
	for _, s := range spans {
		if s.start < 0 || s.end > len(out) || s.start >= s.end {
			continue
		}
		for i := s.start; i < s.end; i++ {
			out[i] = ' '
		}
	}
	return string(out)
}
