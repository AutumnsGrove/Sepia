package server

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// waitIdle waits until the session has no turn in flight, so a retry is not racing the turn it retries.
func waitIdle(t *testing.T, rig *socketRig, sessionID string) {
	t.Helper()
	for i := 0; i < 400; i++ {
		rig.inner.hub.mu.Lock()
		_, running := rig.inner.hub.turns[sessionID]
		rig.inner.hub.mu.Unlock()
		if !running {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the turn never finished")
}

// A retry runs the turn again for the stored message. The transcript keeps one copy, and a second retry with the
// same client message is a duplicate rather than a third turn.
func TestRetryRunsTheTurnWithoutAppendingHerMessage(t *testing.T) {
	rig := newFlowRigGated(t, nil, world.ChatTurn{Text: "Ready."})
	defer rig.close()
	ctx := context.Background()
	sess, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := rig.dial(sess.ID, 0, "http://"+rig.host)
	client.expect("hello")

	client.sendJSON(map[string]any{
		"type": "message", "client_msg_id": "m1",
		"data": map[string]any{
			"text": world.SampleMessage, "client_date": "2026-11-01", "tz": "Europe/London",
		},
	})
	if got := client.expect("ack").field(t, "status"); got != "accepted" {
		t.Fatalf("message ack = %v", got)
	}
	waitIdle(t, rig, sess.ID)

	client.sendJSON(map[string]any{"type": "retry", "client_msg_id": "r1", "data": map[string]any{}})
	if got := client.expect("ack").field(t, "status"); got != "accepted" {
		t.Fatalf("retry ack = %v", got)
	}
	waitIdle(t, rig, sess.ID)

	messages, err := rig.engine.UserMessages(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 {
		t.Fatalf("messages = %v, want her words once", messages)
	}

	// The same retry again is a duplicate, not a third turn.
	client.sendJSON(map[string]any{"type": "retry", "client_msg_id": "r1", "data": map[string]any{}})
	if got := client.expect("ack").field(t, "status"); got != "duplicate" {
		t.Fatalf("repeat retry ack = %v", got)
	}
}

// A saved session keeps its transcript on screen, so the old Try again is still tappable. A retry there must be
// refused, not start another model turn over a finished entry.
func TestRetryIsRefusedOnASavedSession(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()
	sess := rig.newDraft()
	draft, err := rig.engine.Draft(ctx, sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	edited, err := rig.engine.EditDraft(ctx, sess.ID, session.Edit{
		BaseVersion: draft.Version,
		Set: map[string]json.RawMessage{
			"rating": json.RawMessage("18"),
			"body":   json.RawMessage(`"the ending landed but it was too long"`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rig.engine.AppendUserMessage(ctx, sess.ID, world.SampleMessage, "m1"); err != nil {
		t.Fatalf("AppendUserMessage: %v", err)
	}
	if _, _, _, err = rig.engine.Confirm(ctx, sess.ID, session.ConfirmRequest{BaseVersion: edited.Version}); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	client := rig.dial(sess.ID, 0, "http://"+rig.host)
	client.expect("hello")
	client.sendJSON(map[string]any{"type": "retry", "client_msg_id": "r1", "data": map[string]any{}})
	ack := client.expect("ack")
	if ack.field(t, "status") != "rejected" || ack.field(t, "reason") != "session_closed" {
		t.Fatalf("retry on a saved session = %+v, want rejected session_closed", ack)
	}
}
