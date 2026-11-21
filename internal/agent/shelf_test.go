package agent

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// The two new shelves end to end: her message, the fake model, the real tools, the real card. These are the flows
// issue #30 asked for, and the first two are the reason the rating rule was relaxed.

const abandonedMessage = "i couldn't get into the lantern keeper, about a hundred pages in. the start was so slow i stopped caring"

// abandonedScript is a dropped book: no rating is read, so the card never asks for one.
func abandonedScript(progressQuote string) []world.ChatTurn {
	return []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{toolCall("c1", "media_lookup", `{"title":"The Lantern Keeper","medium":"book"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c2", "merge_candidates", `{"candidate_id":"openlibrary|OL1W"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c3", "save_entry",
			`{"body_quotes":["the start was so slow i stopped caring"],"progress_quote":"`+progressQuote+`"}`)}},
		{Text: "The card is ready."},
	}
}

func TestPipelineAbandonedFlow(t *testing.T) {
	r := newRig(t, rigOptions{script: abandonedScript("about a hundred pages in")})
	scriptBook(r.fake)

	ctx := context.Background()
	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.agent.HandleMessage(ctx, TurnInput{
		SessionID: sess.ID, Text: abandonedMessage, ClientMsgID: "m1",
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("no draft card")
	}
	entry := out.Draft.Entry
	if entry.Completion != domain.CompletionAbandoned {
		t.Errorf("completion = %q, want %q", entry.Completion, domain.CompletionAbandoned)
	}
	if entry.Rating != 0 {
		t.Errorf("rating = %d; a work she stopped has no stars", entry.Rating)
	}
	if entry.Progress != "about a hundred pages in" {
		t.Errorf("progress = %q", entry.Progress)
	}
	if entry.Body != "the start was so slow i stopped caring" {
		t.Errorf("body = %q", entry.Body)
	}
	// The card can be confirmed without a rating, which is the rule change this slice is for.
	card, err := r.session.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if !card.Confirmable {
		t.Errorf("an abandoned card is not confirmable: missing %v", card.Missing)
	}
}

func TestPipelineAbandonedRefusesAProgressSheNeverWrote(t *testing.T) {
	r := newRig(t, rigOptions{script: abandonedScript("I gave up around page 250")})
	scriptBook(r.fake)

	ctx := context.Background()
	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.agent.HandleMessage(ctx, TurnInput{
		SessionID: sess.ID, Text: abandonedMessage, ClientMsgID: "m1",
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out.Draft != nil && out.Draft.Entry.Progress != "" {
		t.Errorf("a progress line the model wrote was stored: %q", out.Draft.Entry.Progress)
	}
}

const wantedMessage = "want to read the lantern keeper, everyone says the ending is wild"

func wantedScript() []world.ChatTurn {
	return []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{toolCall("c1", "media_lookup", `{"title":"The Lantern Keeper","medium":"book"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c2", "merge_candidates", `{"candidate_id":"openlibrary|OL1W"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c3", "save_entry", `{}`)}},
		{Text: "Added to your list."},
	}
}

func TestPipelineWantedFlow(t *testing.T) {
	r := newRig(t, rigOptions{script: wantedScript()})
	scriptBook(r.fake)

	ctx := context.Background()
	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.agent.HandleMessage(ctx, TurnInput{
		SessionID: sess.ID, Text: wantedMessage, ClientMsgID: "m1",
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("no draft card")
	}
	entry := out.Draft.Entry
	if entry.Completion != domain.CompletionWantTo {
		t.Errorf("completion = %q, want %q", entry.Completion, domain.CompletionWantTo)
	}
	if entry.Rating != 0 || entry.Body != "" {
		t.Errorf("a wish carries no review: rating %d body %q", entry.Rating, entry.Body)
	}
	card, err := r.session.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if !card.Confirmable {
		t.Errorf("a wanted card is not confirmable: missing %v", card.Missing)
	}
}

// A work on her wanted list is not a revisit. The wish is how the entry started, so writing the review has to revise
// that row rather than open a second one beside it, and "a new entry or the old one" is not a question about a book
// she has not read yet (S21.8).
func TestAWishedWorkIsRevisedIntoTheEntry(t *testing.T) {
	r := newRig(t, rigOptions{script: []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{toolCall("c1", "media_lookup", `{"title":"The Lantern Keeper","medium":"book"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c2", "merge_candidates", `{"candidate_id":"openlibrary|OL1W"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c3", "save_entry",
			`{"body_quotes":["the middle dragged a bit but the end stayed with me"],"verdict_quote":"stayed with me"}`)}},
		{Text: "The card is ready."},
	}})
	scriptBook(r.fake)
	ctx := context.Background()

	// The wish, on the same provider id the lookup will return, so the merge reuses this work.
	if err := r.store.SaveWork(ctx, domain.Work{
		ID: "w-wish", Medium: "book", Title: "The Lantern Keeper", Verified: true,
		PrimaryProvider: "openlibrary", PrimaryID: "OL1W",
	}); err != nil {
		t.Fatalf("SaveWork: %v", err)
	}
	wish := domain.Entry{
		ID: "e-wish", WorkID: "w-wish", Status: domain.StatusDraft, Completion: domain.CompletionWantTo,
		BodyOrigin: domain.BodyTyped, RatingScale: domain.RatingScale,
	}
	if err := r.store.CreateDraft(ctx, wish); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := r.store.SaveEntry(ctx, wish); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.agent.HandleMessage(ctx, TurnInput{
		SessionID: sess.ID, Text: bookMessage, ClientMsgID: "m1",
	})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("no draft card")
	}
	// The card is a revision of the wish, not a second entry.
	if out.Revises != "e-wish" {
		t.Errorf("Revises = %q, want the wished row", out.Revises)
	}
	// She said she finished it, so the wish becomes a review without her having to find the shelf control.
	if out.Draft.Entry.Completion != domain.CompletionFinished {
		t.Errorf("completion = %q, want %q", out.Draft.Entry.Completion, domain.CompletionFinished)
	}
	if out.Draft.Entry.Rating != 18 {
		t.Errorf("rating = %d, want 18", out.Draft.Entry.Rating)
	}

	// And there is still one saved wish, unchanged, until she confirms.
	still, err := r.store.Entry(ctx, "e-wish")
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if still.Completion != domain.CompletionWantTo || still.Status != domain.StatusSaved {
		t.Errorf("the wished row changed before the confirm: %+v", still)
	}
}

// The model naming the shelf has to decide what the card asks for. If the cues miss her words but the model says
// "abandoned", asking for a rating is the exact nudge this slice removed.
func TestSaveEntryAppliesTheModelsShelfBeforeAsking(t *testing.T) {
	// A message whose cues name nothing: no "couldn't", no "gave up", no "want to", no "finished", and nothing that
	// looks like one. "put it aside" would be an abandoned cue, and then the card would already know its shelf and
	// this test would pass with the fix reverted.
	const flat = "the lantern keeper was slow in the middle"
	r := newRig(t, rigOptions{script: []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{toolCall("c1", "media_lookup", `{"title":"The Lantern Keeper","medium":"book"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c2", "merge_candidates", `{"candidate_id":"openlibrary|OL1W"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c3", "save_entry",
			`{"body_quotes":["the lantern keeper was slow in the middle"],"completion":"abandoned"}`)}},
		{Text: "The card is ready."},
	}})
	scriptBook(r.fake)
	ctx := context.Background()

	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := r.agent.HandleMessage(ctx, TurnInput{SessionID: sess.ID, Text: flat, ClientMsgID: "m1"})
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if out.Draft == nil {
		t.Fatal("save_entry failed before it could apply the model's shelf")
	}
	if out.Draft.Entry.Completion != domain.CompletionAbandoned {
		t.Fatalf("completion = %q, want %q", out.Draft.Entry.Completion, domain.CompletionAbandoned)
	}
	if out.Draft.Entry.Body == "" {
		t.Error("no body: the card was asked for a rating before the shelf was applied")
	}
	card, err := r.session.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if !card.Confirmable {
		t.Errorf("an abandoned card with her words is not confirmable: missing %v", card.Missing)
	}
}

// The revisit card describes the entry already on the work, and an abandoned row has no rating and a date she stopped
// on. The payload has to carry the shelf, or the card says "finished <date> · 0 stars" about a work she put down.
// This guards the key itself: revisitMeta falls back to "finished" when it is missing, so a dropped field would
// silently restore the old sentence with every other test still green.
func TestTheRevisitCardCarriesTheShelf(t *testing.T) {
	r := newRig(t, rigOptions{script: []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{toolCall("c1", "media_lookup", `{"title":"The Lantern Keeper","medium":"book"}`)}},
		{ToolCalls: []world.ChatToolCall{toolCall("c2", "merge_candidates", `{"candidate_id":"openlibrary|OL1W"}`)}},
		{Text: "The card is ready."},
	}})
	scriptBook(r.fake)
	r.responder.script("revisit_choice", "new_entry")
	ctx := context.Background()

	// A work she stopped, already saved, on the provider id the lookup returns.
	if err := r.store.SaveWork(ctx, domain.Work{
		ID: "w-ab", Medium: "book", Title: "The Lantern Keeper", Verified: true,
		PrimaryProvider: "openlibrary", PrimaryID: "OL1W",
	}); err != nil {
		t.Fatalf("SaveWork: %v", err)
	}
	dropped := domain.Entry{
		ID: "e-ab", WorkID: "w-ab", Status: domain.StatusDraft, Completion: domain.CompletionAbandoned,
		Body: "the middle lost me", BodyOrigin: domain.BodyTyped, Progress: "two episodes",
		RatingScale: domain.RatingScale, FinishedOn: "2026-11-12",
	}
	if err := r.store.CreateDraft(ctx, dropped); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	if _, err := r.store.SaveEntry(ctx, dropped); err != nil {
		t.Fatalf("SaveEntry: %v", err)
	}

	sess, err := r.session.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.agent.HandleMessage(ctx, TurnInput{
		SessionID: sess.ID, Text: "the lantern keeper was slow in the middle", ClientMsgID: "m1",
	}); err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}

	var previous map[string]any
	for _, question := range r.responder.seen {
		if question.Kind != "revisit_choice" {
			continue
		}
		var payload struct {
			Previous map[string]any `json:"previous"`
		}
		if err := json.Unmarshal(question.Payload, &payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		previous = payload.Previous
	}
	if previous == nil {
		t.Fatal("no revisit_choice was asked, so the payload cannot be checked")
	}
	if previous["completion"] != string(domain.CompletionAbandoned) {
		t.Errorf("previous.completion = %v, want %q; without it the card calls this a finish",
			previous["completion"], domain.CompletionAbandoned)
	}
	// A dropped work has no rating, and the card must not be handed a zero to print as stars.
	if previous["rating_q"] != float64(0) {
		t.Errorf("previous.rating_q = %v, want 0", previous["rating_q"])
	}
}
