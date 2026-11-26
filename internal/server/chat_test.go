package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// S16.4 over the socket: a general session answers a question, proposes a change, and writes it only when she
// confirms. The library's entries are real rows here, so the test reads the entry back after the confirm rather than
// trusting the event that said it happened.

// aChatScript is a chat whose model proposes one change and then says it is done.
func aChatScript(entryID string) []world.ChatTurn {
	return []world.ChatTurn{
		{ToolCalls: []world.ChatToolCall{{
			ID: "c1", Name: "edit_entry", Arguments: `{"entry_id":"` + entryID + `","rating":4.5}`,
		}}},
		{Text: "Ready."},
	}
}

func sendChatMessage(t *testing.T, c *wsClient, text string) {
	t.Helper()
	c.sendJSON(map[string]any{
		"type": "message", "client_msg_id": "m-" + text[:4],
		"data": map[string]any{"text": text, "client_date": "2026-11-26", "tz": "UTC"},
	})
}

// Happy: the chat proposes a rating change, the card shows what it would do, and her confirm writes it.
func TestTheLibraryChatProposesAnEditAndWritesItOnConfirm(t *testing.T) {
	rig := newFlowRigDecided(t, nil, world.ScriptSampleDecisions)
	defer rig.close()
	saved := aReviewedEntry(t, rig.engine, rig.db, "Hades II", "game",
		"the boss fights are the best kind of unfair", 17, "2026-10-12")
	rig.chat.ScriptChat(aChatScript(saved.ID)...)

	sess, err := rig.engine.CreateGeneral(context.Background(), "make it a 4.5")
	if err != nil {
		t.Fatalf("CreateGeneral: %v", err)
	}
	c := rig.dial(sess.ID, 0, "http://"+rig.host)
	c.expect("hello")

	sendChatMessage(t, c, "make it a 4.5 please")
	asked := c.expectWithin("interaction_request", 20*time.Second)

	var data struct {
		InteractionID string          `json:"interaction_id"`
		Kind          string          `json:"kind"`
		Payload       json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(asked.Data, &data); err != nil {
		t.Fatalf("interaction data: %v", err)
	}
	if data.Kind != "edit_entry" {
		t.Fatalf("interaction kind = %q, want edit_entry", data.Kind)
	}
	var payload struct {
		EntryID     string              `json:"entry_id"`
		Title       string              `json:"title"`
		BaseVersion string              `json:"base_version"`
		Changes     []domain.EditChange `json:"changes"`
	}
	if err := json.Unmarshal(data.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload.EntryID != saved.ID || len(payload.Changes) != 1 || payload.Changes[0].After != "4.5" {
		t.Fatalf("payload = %+v", payload)
	}
	if payload.Title != "Hades II" || payload.BaseVersion == "" {
		t.Fatalf("payload = %+v, want the work's title and the version it was drawn from", payload)
	}
	// Nothing is written while the card is up.
	before, beforeErr := rig.db.Entry(context.Background(), saved.ID)
	if beforeErr != nil {
		t.Fatalf("Entry: %v", beforeErr)
	}
	if before.Rating != saved.Rating {
		t.Fatalf("rating = %d before the confirm, want %d", before.Rating, saved.Rating)
	}

	c.sendJSON(map[string]any{
		"type": "interaction_response", "client_msg_id": "r1",
		"data": map[string]any{
			"interaction_id": data.InteractionID,
			"value":          map[string]any{"value": "confirm"},
		},
	})
	c.expectWithin("assistant_end", 20*time.Second)

	after, afterErr := rig.db.Entry(context.Background(), saved.ID)
	if afterErr != nil {
		t.Fatalf("Entry: %v", afterErr)
	}
	if after.Rating != 18 {
		t.Fatalf("rating = %d after the confirm, want 18", after.Rating)
	}
	if after.Body != saved.Body {
		t.Fatalf("body = %q, want her words untouched", after.Body)
	}
}

// Sad: a question about the library with no edit in it changes nothing, and the answer is just words.
func TestTheLibraryChatAnswersWithoutTouchingAnEntry(t *testing.T) {
	rig := newFlowRigDecided(t, nil, world.ScriptSampleDecisions)
	defer rig.close()
	saved := aReviewedEntry(t, rig.engine, rig.db, "Hades II", "game",
		"the boss fights are the best kind of unfair", 17, "2026-10-12")
	rig.chat.ScriptChat(world.ChatTurn{Text: "One book. Hades II."})

	sess, err := rig.engine.CreateGeneral(context.Background(), "what did i read")
	if err != nil {
		t.Fatalf("CreateGeneral: %v", err)
	}
	c := rig.dial(sess.ID, 0, "http://"+rig.host)
	c.expect("hello")
	sendChatMessage(t, c, "what did i read in october")
	c.expectWithin("assistant_end", 20*time.Second)

	after, err := rig.db.Entry(context.Background(), saved.ID)
	if err != nil {
		t.Fatalf("Entry: %v", err)
	}
	if after.Rating != saved.Rating || after.UpdatedAt != saved.UpdatedAt {
		t.Fatalf("the entry changed while answering a question: %+v", after)
	}
}

// Sad/happy boundary: a message the client retries after a reconnect carries the same client_msg_id, and the second
// copy must not be reported as a failed turn. The first turn stored it; the retry changes nothing and says nothing, so
// a phone that reconnected does not show an error for a review that saved perfectly well (docs/PROTOCOL.md 6).
func TestARetriedMessageIsNotATurnFailure(t *testing.T) {
	rig := newFlowRigDecided(t, nil, world.ScriptSampleDecisions)
	defer rig.close()
	rig.chat.ScriptChat(world.ChatTurn{Text: "One book. Hades II."})

	sess, err := rig.engine.CreateGeneral(context.Background(), "what did i read")
	if err != nil {
		t.Fatalf("CreateGeneral: %v", err)
	}
	first := rig.dial(sess.ID, 0, "http://"+rig.host)
	first.expect("hello")
	first.sendJSON(map[string]any{
		"type": "message", "client_msg_id": "m-retried",
		"data": map[string]any{"text": "what did i read", "client_date": "2026-11-26", "tz": "UTC"},
	})
	first.expect("assistant_end")

	// The same message again, as an outbox would send it after a reconnect: a fresh socket, the same id.
	second := rig.dial(sess.ID, 0, "http://"+rig.host)
	second.expect("hello")
	second.sendJSON(map[string]any{
		"type": "message", "client_msg_id": "m-retried",
		"data": map[string]any{"text": "what did i read", "client_date": "2026-11-26", "tz": "UTC"},
	})
	if ack := second.expect("ack"); ack.field(t, "status") != "duplicate" {
		t.Fatalf("ack = %v, want duplicate", ack.field(t, "status"))
	}
	// And nothing error-shaped follows it.
	for i := 0; i < 8; i++ {
		ev, ok := second.nextWithin(500 * time.Millisecond)
		if !ok {
			break
		}
		if ev.Type == "error" {
			t.Fatalf("a retried message produced an error event: %s", ev.Data)
		}
	}
	messages, err := rig.engine.UserMessages(context.Background(), sess.ID)
	if err != nil {
		t.Fatalf("UserMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %d, want the retry to have stored nothing twice", len(messages))
	}
}
