package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AutumnsGrove/nova/tool"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// search_library is how the general chat answers "have I reviewed this already?" and "what did I watch in March?".
// It reads the same full-text index the search screen does, so the two can never disagree about what her words match.

func saveSearchable(t *testing.T, r *rig, id, title, medium, body string, rating int, finished string) {
	t.Helper()
	ctx := context.Background()
	workID := "w-" + id
	if err := r.store.SaveWork(ctx, domain.Work{ID: workID, Medium: medium, Title: title, Verified: true}); err != nil {
		t.Fatalf("work %s: %v", id, err)
	}
	if err := r.store.CreateDraft(ctx, domain.Entry{
		ID: id, WorkID: workID, Rating: rating, RatingScale: domain.RatingScale, Body: body,
		BodyOrigin: domain.BodyTyped, Status: domain.StatusSaved, Completion: domain.CompletionFinished,
		FinishedOn: finished,
	}); err != nil {
		t.Fatalf("entry %s: %v", id, err)
	}
}

func callSearchLibrary(t *testing.T, r *rig, args string) tool.Result {
	t.Helper()
	state := &RunState{Agent: r.agent, SessionID: "general-1"}
	result, err := searchLibrary(context.Background(), json.RawMessage(args), &tool.Context{Data: state})
	if err != nil {
		t.Fatalf("searchLibrary(%s): %v", args, err)
	}
	return result
}

// Happy: a word from her body finds the entry, and the sentence it was found in comes back as plain text rather than
// with the invisible markers the screen uses.
func TestSearchLibraryFindsHerWords(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow but then it grabbed me", 19, "2026-09-28")

	result := callSearchLibrary(t, r, `{"query":"slow"}`)
	if result.IsError {
		t.Fatalf("search failed: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Piranesi") || !strings.Contains(result.Content, "entry e1") {
		t.Fatalf("content = %q", result.Content)
	}
	if !strings.Contains(result.Content, "started slow but then it grabbed me") {
		t.Fatalf("the matched sentence is missing: %q", result.Content)
	}
	if strings.Contains(result.Content, domain.SnippetOpen) || strings.Contains(result.Content, domain.SnippetClose) {
		t.Fatalf("the match markers reached the model: %q", result.Content)
	}
	var details toolDetails
	if err := json.Unmarshal(result.Details, &details); err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Summary != "1 match" {
		t.Fatalf("summary = %q", details.Summary)
	}
}

// Happy: the filters narrow by medium and by day, which is what makes "what did I watch in March?" answerable.
func TestSearchLibraryFilters(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e-film", "Blade Runner 2049", "film", "slow and deliberate", 19, "2026-10-27")
	saveSearchable(t, r, "e-book", "Piranesi", "book", "slow to start", 17, "2026-09-28")

	cases := map[string]string{
		"a medium":     `{"query":"slow","medium":"film"}`,
		"from a day":   `{"query":"slow","from":"2026-10-01"}`,
		"to a day":     `{"query":"slow","to":"2026-09-30"}`,
		"a whole span": `{"query":"slow","medium":"book","from":"2026-09-01","to":"2026-09-30"}`,
	}
	want := map[string]string{"a medium": "e-film", "from a day": "e-film", "to a day": "e-book", "a whole span": "e-book"}
	for name, args := range cases {
		result := callSearchLibrary(t, r, args)
		if result.IsError {
			t.Fatalf("%s: %s", name, result.Content)
		}
		if !strings.Contains(result.Content, "entry "+want[name]) {
			t.Fatalf("%s = %q, want entry %s", name, result.Content, want[name])
		}
	}
}

// Happy: more matches than the model may see are counted rather than silently dropped, so it can say how many there
// were.
func TestSearchLibraryCountsPastTheCap(t *testing.T) {
	r := newRig(t, rigOptions{})
	for i, id := range []string{"e1", "e2", "e3", "e4", "e5", "e6"} {
		saveSearchable(t, r, id, "Book "+id, "book", "a slow read, honestly", 15+i, "2026-10-0"+string(rune('1'+i)))
	}
	result := callSearchLibrary(t, r, `{"query":"slow"}`)
	var details toolDetails
	if err := json.Unmarshal(result.Details, &details); err != nil {
		t.Fatalf("details: %v", err)
	}
	if details.Summary != "5 of 6 matches" {
		t.Fatalf("summary = %q, want the cap and the count", details.Summary)
	}
	if got := strings.Count(result.Content, "- Book "); got != maxSearchHits {
		t.Fatalf("lines = %d, want %d", got, maxSearchHits)
	}
}

// Sad: nothing matching is an answer, not an error: the model can then say she has not written about it.
func TestSearchLibrarySaysWhenNothingMatches(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow", 19, "2026-09-28")
	result := callSearchLibrary(t, r, `{"query":"zwift"}`)
	if result.IsError || !strings.Contains(result.Content, "Nothing in her library matches") {
		t.Fatalf("result = %+v", result)
	}
}

// Bad: arguments the tool cannot honour are refused with the reason, so the model corrects itself rather than the
// library being searched with a nonsense filter.
func TestSearchLibraryRefusesBadArguments(t *testing.T) {
	r := newRig(t, rigOptions{})
	for name, args := range map[string]string{
		"an unknown medium": `{"query":"slow","medium":"vinyl"}`,
		"a made-up day":     `{"query":"slow","from":"sometime in march"}`,
		"no words":          `{"query":"***"}`,
		"too many words":    `{"query":"one two three four five six seven eight nine"}`,
		"an unknown field":  `{"query":"slow","rating":4}`,
	} {
		result := callSearchLibrary(t, r, args)
		if !result.IsError {
			t.Fatalf("%s was accepted: %q", name, result.Content)
		}
	}
}
