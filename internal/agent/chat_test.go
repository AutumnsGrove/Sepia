package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/AutumnsGrove/nova/tool"

	"github.com/AutumnsGrove/sepia/internal/domain"
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
	revisions, err := r.store.Revisions(context.Background(), "e1")
	if err != nil {
		t.Fatalf("Revisions: %v", err)
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
