package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

func TestCardCarriesTheWorkTheAnswersAndWhatWasLeftOut(t *testing.T) {
	e, _, sess := draftSession(t)
	ctx := context.Background()
	message := "hey so i finished the lantern keeper last night, the ending landed but it was too long"
	if _, err := e.AppendUserMessage(ctx, sess.ID, message, "m-source"); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	quote := "the ending landed but it was too long"
	draft, err := e.SetExtractedBody(ctx, sess.ID, 0, message, []string{quote}, "the ending landed", "m-body")
	if err != nil {
		t.Fatalf("SetExtractedBody: %v", err)
	}
	if draft.Entry.WorkID != "w1" {
		t.Fatalf("work = %q, want w1", draft.Entry.WorkID)
	}
	card, err := e.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if card.Work.Title != "The Lantern Keeper" {
		t.Errorf("work = %+v", card.Work)
	}
	if len(card.LeftOut) == 0 || !strings.Contains(card.LeftOut[0].Text, "hey so i finished") {
		t.Errorf("left out = %+v, want the framing", card.LeftOut)
	}
	if card.SourceText != message {
		t.Errorf("source = %q", card.SourceText)
	}
}

func TestEntryViewReadsASavedEntry(t *testing.T) {
	e, _, sess := draftSession(t)
	ctx := context.Background()
	if _, err := e.EditDraft(ctx, sess.ID, Edit{BaseVersion: 0, Set: patch(map[string]any{
		"rating": 18, "body": "A quiet book about light and memory.",
	})}); err != nil {
		t.Fatalf("EditDraft: %v", err)
	}
	entry, _, _, err := e.Confirm(ctx, sess.ID, ConfirmRequest{BaseVersion: 1})
	if err != nil {
		t.Fatalf("Confirm: %v", err)
	}
	view, err := e.EntryView(ctx, entry.ID)
	if err != nil {
		t.Fatalf("EntryView: %v", err)
	}
	if view.Entry.Rating != 18 || view.Work.Title == "" {
		t.Errorf("view = %+v", view)
	}
	if _, err := e.EntryView(ctx, "no-such-entry"); err == nil {
		t.Error("a missing entry was read")
	}
}

func TestSessionsListsWithFilters(t *testing.T) {
	e, _, sess := draftSession(t)
	ctx := context.Background()
	general, err := e.CreateGeneral(ctx, "what should i read next")
	if err != nil {
		t.Fatal(err)
	}
	active, err := e.Sessions(ctx, domain.SessionActive, "", "", 10)
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(active) != 2 {
		t.Fatalf("active = %d, want the draft and the chat", len(active))
	}
	chats, err := e.Sessions(ctx, "", domain.SessionGeneral, "", 10)
	if err != nil || len(chats) != 1 || chats[0].ID != general.ID {
		t.Fatalf("chats = %+v, %v", chats, err)
	}
	one, err := e.Sessions(ctx, "", "", "", 1)
	if err != nil || len(one) != 1 {
		t.Fatalf("one = %+v, %v", one, err)
	}
	if one[0].ID == sess.ID && general.ID == "" {
		t.Error("the newest session was not first")
	}
}

// A session with two pre-reads (a second message, or Try again, which re-runs the pre-read by design) must still
// show one row per check. The why sheet used to key its list by the text it printed, so a repeated name and option
// took the whole session page down with "each_key_duplicate" (issue #38).
func TestWhySheetKeepsOneRowPerCheck(t *testing.T) {
	e, db, sess := draftSession(t)
	ctx := context.Background()
	record := func(probability float64) {
		t.Helper()
		odds, err := json.Marshal(map[string]map[string]float64{
			"intent": {"log_review": probability, "chitchat": 0.02},
			"medium": {"book": 0.90, "film": 0.03},
		})
		if err != nil {
			t.Fatalf("marshal odds: %v", err)
		}
		if err := db.RecordDecisionCall(ctx, domain.DecisionCall{
			SessionID: sess.ID, Odds: odds, Fired: []string{"intent", "medium"},
		}); err != nil {
			t.Fatalf("RecordDecisionCall: %v", err)
		}
	}
	record(0.94)
	record(0.99)

	card, err := e.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	seen := map[string]int{}
	for _, check := range card.Checks {
		seen[check.Name]++
	}
	if seen["intent"] != 1 || seen["medium"] != 1 {
		t.Fatalf("checks = %+v, want one row per name, got %v", card.Checks, seen)
	}
	// The newer pre-read wins: the sheet is "why this card says that", and an earlier read is stale, not a second
	// opinion.
	if card.Checks[0].Name != "intent" || card.Checks[0].Probability != 0.99 {
		t.Errorf("checks = %+v, want the newest intent first", card.Checks)
	}
	// The order is stable between reads. The odds are a JSON object, and Go's map iteration is random, so without
	// sorted names the sheet reshuffled itself on every render.
	again, err := e.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	for i := range again.Checks {
		if again.Checks[i] != card.Checks[i] {
			t.Fatalf("the sheet reordered between reads:\n%+v\n%+v", card.Checks, again.Checks)
		}
	}
}

// created_at is whole seconds, so two pre-reads in the same second tie. Production ids are random, so ordering the
// tie by id can hand the older row back last and the sheet would keep the stale odds. rowid is insertion order.
func TestWhySheetPrefersTheNewerPreReadWithinOneSecond(t *testing.T) {
	e, db, sess := draftSession(t)
	ctx := context.Background()
	at := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	record := func(id string, probability float64) {
		t.Helper()
		odds, err := json.Marshal(map[string]map[string]float64{
			"intent": {"log_review": probability, "chitchat": 0.01},
		})
		if err != nil {
			t.Fatalf("marshal odds: %v", err)
		}
		if err := db.RecordDecisionCall(ctx, domain.DecisionCall{
			ID: id, SessionID: sess.ID, Odds: odds, Fired: []string{"intent"}, CreatedAt: at,
		}); err != nil {
			t.Fatalf("RecordDecisionCall: %v", err)
		}
	}
	// The newer row's id sorts before the older one's, which is what a random id does for half the pairs.
	record("id-zzzz-first", 0.94)
	record("id-aaaa-second", 0.99)

	card, err := e.Card(ctx, sess.ID)
	if err != nil {
		t.Fatalf("Card: %v", err)
	}
	if len(card.Checks) != 1 {
		t.Fatalf("checks = %+v, want one row", card.Checks)
	}
	if card.Checks[0].Probability != 0.99 {
		t.Errorf("probability = %v, want 0.99: the older pre-read replaced the newer one", card.Checks[0].Probability)
	}
}
