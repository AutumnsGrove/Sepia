package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/nova/tool"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/store"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// The library chat: one read tool, one proposal, and nothing that writes an entry or deletes one. These tests drive
// the tools directly, because what they prove is the rule rather than the plumbing: a change is written only when she
// confirms it, and no tool in a chat can delete anything.

func chatState(t *testing.T, r *rig, sessionID string) *RunState {
	t.Helper()
	return &RunState{Agent: r.agent, SessionID: sessionID, Chat: true}
}

// aChatSession is a general session, which is what a chat turn runs against.
func aChatSession(t *testing.T, r *rig, title string) domain.Session {
	t.Helper()
	sess, err := r.session.CreateGeneral(context.Background(), title)
	if err != nil {
		t.Fatalf("CreateGeneral: %v", err)
	}
	return sess
}

func callEdit(t *testing.T, r *rig, state *RunState, args string) tool.Result {
	t.Helper()
	result, err := editEntry(context.Background(), json.RawMessage(args), &tool.Context{Data: state})
	if err != nil {
		t.Fatalf("editEntry(%s) returned a Go error: %v", args, err)
	}
	return result
}

// Happy: "make hades 2 a 4.5" becomes a card, and her confirm is what writes it. The version it replaced is kept,
// which is the same promise a typed edit makes.
func TestAChatEditIsWrittenOnlyOnConfirm(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Hades II", "game", "the boss fights are the best kind of unfair", 17, "2026-10-12")
	sess := aChatSession(t, r, "make hades 2 a 4.5")
	r.responder.script("edit_entry", "confirm")

	result := callEdit(t, r, chatState(t, r, sess.ID), `{"entry_id":"e1","rating":4.5}`)
	if result.IsError {
		t.Fatalf("the edit failed: %s", result.Content)
	}

	after, err := r.store.Entry(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if after.Rating != 18 {
		t.Fatalf("rating = %d, want 18 quarters for 4.5", after.Rating)
	}
	// The proposal the card showed, as the protocol records it: the diff, the version it was drawn from and the title.
	var payload struct {
		EntryID     string              `json:"entry_id"`
		Title       string              `json:"title"`
		BaseVersion string              `json:"base_version"`
		Changes     []domain.EditChange `json:"changes"`
	}
	var asked domain.Interaction
	for _, seen := range r.responder.seen {
		if seen.Kind == "edit_entry" {
			asked = seen
		}
	}
	if asked.ID == "" {
		t.Fatal("no edit_entry interaction was asked")
	}
	if err := json.Unmarshal(asked.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.EntryID != "e1" || payload.Title != "Hades II" || payload.BaseVersion == "" {
		t.Fatalf("payload = %+v", payload)
	}
	if len(payload.Changes) != 1 || payload.Changes[0].Field != "rating" ||
		payload.Changes[0].Before != "4.25" || payload.Changes[0].After != "4.5" {
		t.Fatalf("changes = %+v, want the rating 4.25 to 4.5", payload.Changes)
	}
	// Her words are untouched: the chat cannot write a body, so an edit that does not mention them leaves them be.
	if after.Body != "the boss fights are the best kind of unfair" {
		t.Fatalf("body = %q, want her words exactly", after.Body)
	}
	revisions, revErr := r.store.Revisions(context.Background(), "e1")
	if revErr != nil {
		t.Fatalf("Revisions: %v", revErr)
	}
	if len(revisions) == 0 {
		t.Fatal("no revision was kept for the change")
	}
}

// Sad: a change she cancels, or one she never confirms, changes nothing. This is the rule, not a nicety.
func TestAChatEditChangesNothingWithoutAConfirm(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow but then it grabbed me", 19, "2026-09-28")
	sess := aChatSession(t, r, "make piranesi a 3")
	r.responder.script("edit_entry", "cancel")

	result := callEdit(t, r, chatState(t, r, sess.ID), `{"entry_id":"e1","rating":3}`)
	if result.IsError {
		t.Fatalf("a cancelled edit is not an error: %s", result.Content)
	}
	after, err := r.store.Entry(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if after.Rating != 19 {
		t.Fatalf("rating = %d, want 19: a cancelled change was written", after.Rating)
	}
}

// Bad: what the chat may not do, and what it refuses rather than guessing.
func TestAChatEditRefusesWhatItCannotDo(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow", 19, "2026-09-28")
	sess := aChatSession(t, r, "questions")

	cases := map[string]struct{ args, want string }{
		"a rating outside the scale": {`{"entry_id":"e1","rating":9}`, "not a rating"},
		"a shelf Sepia lacks":        {`{"entry_id":"e1","completion":"halfway"}`, "not a shelf"},
		"nothing at all":             {`{"entry_id":"e1"}`, "asks for nothing"},
		"the value it already has":   {`{"entry_id":"e1","rating":4.75}`, "already has"},
		"an entry that is not there": {`{"entry_id":"nope","rating":4}`, "no entry with that id"},
		"no entry named":             {`{"rating":4}`, "say which entry"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			result := callEdit(t, r, chatState(t, r, sess.ID), tc.args)
			if !result.IsError {
				t.Fatalf("accepted %s", tc.args)
			}
			if !strings.Contains(result.Content, tc.want) {
				t.Fatalf("content = %q, want it to mention %q", result.Content, tc.want)
			}
		})
	}
	// Moving a rated entry with words on it to the stopped shelf is allowed, because the guard that makes it legal
	// (an abandoned entry needs a body) is the one the card uses. She cancels, so nothing is written here.
	r.responder.script("edit_entry", "cancel")
	noRating := callEdit(t, r, chatState(t, r, sess.ID), `{"entry_id":"e1","rating":5,"completion":"abandoned"}`)
	if noRating.IsError {
		t.Fatalf("moving a rated entry with words to abandoned should be allowed: %s", noRating.Content)
	}
}

// Bad: the chat's whole vocabulary is one read and one proposal. A tool that could delete or write an entry directly
// is not registered, so no prompt can reach one.
func TestTheChatHasNoToolThatDeletesOrSaves(t *testing.T) {
	r := newRig(t, rigOptions{})
	names := r.agent.chatTools.Names()
	want := map[string]bool{"search_library": true, "edit_entry": true}
	if len(names) != len(want) {
		t.Fatalf("chat tools = %v, want exactly %v", names, want)
	}
	for _, name := range names {
		if !want[name] {
			t.Fatalf("chat tool %q is not in the chat vocabulary", name)
		}
		for _, forbidden := range []string{"delete", "save", "merge", "lookup", "ask_reviewer"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("chat tool %q contains %q", name, forbidden)
			}
		}
	}
}

// Sad: a change nobody can confirm is refused rather than written. A turn with no client attached still runs, so the
// tool must say so instead of waiting forever.
func TestAChatEditWithNobodyToConfirm(t *testing.T) {
	r := newRig(t, rigOptions{noResponder: true})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow", 19, "2026-09-28")
	sess := aChatSession(t, r, "questions")

	result := callEdit(t, r, chatState(t, r, sess.ID), `{"entry_id":"e1","rating":2}`)
	if !result.IsError || !strings.Contains(result.Content, "nobody to confirm") {
		t.Fatalf("result = %+v, want a refusal", result)
	}
	after, err := r.store.Entry(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if after.Rating != 19 {
		t.Fatalf("rating = %d, want 19", after.Rating)
	}
}

// Sad: another screen saved the entry while the card was up. The change was proposed from a version she can no longer
// see, so it is refused rather than written over the top of the other screen's save.
func TestAChatEditRefusesAVersionSheHasNotSeen(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Hades II", "game", "the boss fights are the best kind of unfair", 17, "2026-10-12")
	sess := aChatSession(t, r, "make hades a 5")
	r.responder.script("edit_entry", "confirm")
	r.responder.beforeAnswer = func() {
		ctx := context.Background()
		entry, err := r.store.Entry(ctx, "e1")
		if err != nil {
			t.Errorf("Entry: %v", err)
			return
		}
		entry.Rating = 12
		r.clock.Advance(time.Minute)
		if _, err := r.store.SaveEntry(ctx, entry); err != nil {
			t.Errorf("the other screen's save: %v", err)
		}
	}

	result := callEdit(t, r, chatState(t, r, sess.ID), `{"entry_id":"e1","rating":5}`)
	if !result.IsError || !strings.Contains(result.Content, "stale_version") {
		t.Fatalf("result = %+v, want a stale-version refusal", result)
	}
	after, err := r.store.Entry(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if after.Rating != 12 {
		t.Fatalf("rating = %d, want 12: the other screen's save was overwritten", after.Rating)
	}
}

// A chat answer carries the entries it is about, as rows, and they come after the sentence: the mockup puts the answer
// first and the entries under it. A reload finds the rows in the log rather than asking again.
func TestAChatAnswerCarriesTheEntriesItFound(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow but then it grabbed me", 19, "2026-09-28")
	sess := aChatSession(t, r, "what did i read that was slow")
	r.chat.ScriptChat(
		world.ChatTurn{ToolCalls: []world.ChatToolCall{{
			ID: "c1", Name: "search_library", Arguments: `{"query":"slow"}`,
		}}},
		world.ChatTurn{Text: "One book. Piranesi."},
	)

	if _, err := r.agent.HandleMessage(context.Background(), TurnInput{
		SessionID: sess.ID, Text: "what did i read that was slow",
	}); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	events, err := r.store.EventsSince(context.Background(), sess.ID, 0)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	resultsAt, answerAt := -1, -1
	for i, event := range events {
		switch event.Kind {
		case store.EventChatResults:
			resultsAt = i
		case store.EventAssistantEnd:
			answerAt = i
		}
	}
	if resultsAt < 0 {
		t.Fatal("no chat_results event: the answer has no rows under it")
	}
	if answerAt < 0 || resultsAt < answerAt {
		t.Fatalf("rows at %d, answer ended at %d: the rows must come after the answer", resultsAt, answerAt)
	}
	var payload struct {
		Results []chatResult `json:"results"`
	}
	if err := json.Unmarshal(events[resultsAt].Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("results = %+v, want one row", payload.Results)
	}
	row := payload.Results[0]
	if row.EntryID != "e1" || row.Title != "Piranesi" || row.Medium != "book" ||
		row.Completion != "finished" || row.RatingQ != 19 || row.FinishedOn != "2026-09-28" {
		t.Fatalf("row = %+v", row)
	}
}

// Bad: a turn that searches twice keeps both sets of rows, in the order the searches ran, with an entry that both found
// drawn once. A search that finds nothing adds nothing, so an answer about the earlier hits keeps its rows.
func TestTwoSearchesInOneTurnKeepBothSetsOfRows(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow but then it grabbed me", 19, "2026-09-28")
	saveSearchable(t, r, "e2", "Grey Harbour", "tv", "a slow burn that earns its ending", 18, "2026-09-14")
	ctx := context.Background()
	state := &RunState{Agent: r.agent, SessionID: "s_chat", Chat: true}
	toolCtx := &tool.Context{Data: state}

	if _, err := searchLibrary(ctx, json.RawMessage(`{"query":"slow"}`), toolCtx); err != nil {
		t.Fatalf("first search: %v", err)
	}
	first := len(state.ChatResults)
	if first != 2 {
		t.Fatalf("rows after one search = %d, want 2", first)
	}
	// The same words again: the same two entries, drawn once.
	if _, err := searchLibrary(ctx, json.RawMessage(`{"query":"slow"}`), toolCtx); err != nil {
		t.Fatalf("second search: %v", err)
	}
	if got := len(state.ChatResults); got != first {
		t.Fatalf("rows after a repeated search = %d, want %d", got, first)
	}
	// A search that finds nothing adds nothing and removes nothing.
	if _, err := searchLibrary(ctx, json.RawMessage(`{"query":"zzzznothing"}`), toolCtx); err != nil {
		t.Fatalf("empty search: %v", err)
	}
	if got := len(state.ChatResults); got != first {
		t.Fatalf("rows after a search with no hits = %d, want the %d from before", got, first)
	}
	// A different search adds its own, after the ones already found.
	if _, err := searchLibrary(ctx, json.RawMessage(`{"query":"Piranesi"}`), toolCtx); err != nil {
		t.Fatalf("third search: %v", err)
	}
	if got := len(state.ChatResults); got != 2 {
		t.Fatalf("rows after a narrower search = %d, want 2", got)
	}
	if state.ChatResults[0].EntryID != "e1" && state.ChatResults[1].EntryID != "e1" {
		t.Fatalf("rows = %+v, want Piranesi among them", state.ChatResults)
	}
}

// Sad: a turn whose only search found nothing draws no rows at all, rather than an empty list under the answer.
func TestAChatAnswerWithNoMatchesHasNoRows(t *testing.T) {
	r := newRig(t, rigOptions{})
	saveSearchable(t, r, "e1", "Piranesi", "book", "started slow", 19, "2026-09-28")
	sess := aChatSession(t, r, "what did i read about hares")
	r.chat.ScriptChat(
		world.ChatTurn{ToolCalls: []world.ChatToolCall{{
			ID: "c1", Name: "search_library", Arguments: `{"query":"zzzznothing"}`,
		}}},
		world.ChatTurn{Text: "Nothing I can find."},
	)
	if _, err := r.agent.HandleMessage(context.Background(), TurnInput{
		SessionID: sess.ID, Text: "what did i read about hares",
	}); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	events, err := r.store.EventsSince(context.Background(), sess.ID, 0)
	if err != nil {
		t.Fatalf("EventsSince: %v", err)
	}
	for _, event := range events {
		if event.Kind == store.EventChatResults {
			t.Fatalf("a search with no hits produced rows: %s", event.Payload)
		}
	}
}
