package agent

import (
	"context"
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
