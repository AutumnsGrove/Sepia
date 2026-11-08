package server

import (
	"context"
	"encoding/json"
	"testing"
)

// Reopen is the path Edit on a saved card takes: one message opens a working copy of the entry, and her confirm
// keeps the previous version. It is also the regression test for the revision target belonging to the session
// rather than to the turn that opened the card, because the turn has finished long before she taps Save.
func TestReopenOverTheSocket(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()

	saved := rig.newSavedEntry()
	sess, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := rig.dial(sess.ID, 0, "http://"+rig.host)
	client.expect("hello")

	client.sendJSON(map[string]any{
		"type": "reopen", "client_msg_id": "r1", "data": map[string]any{"entry_id": saved.ID},
	})
	if got := client.expect("ack").field(t, "status"); got != "accepted" {
		t.Fatalf("reopen ack = %v", got)
	}
	snapshot := client.expect("draft_snapshot")
	var data struct {
		Version int `json:"version"`
		Draft   struct {
			Body   string `json:"body"`
			Rating struct {
				Quarters int `json:"quarters"`
			} `json:"rating"`
		} `json:"draft"`
	}
	if err = json.Unmarshal(snapshot.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Draft.Body != saved.Body || data.Draft.Rating.Quarters != saved.Rating {
		t.Errorf("working copy = %+v, want the saved card", data.Draft)
	}

	// A retry of the same message is a duplicate, not a second card.
	client.sendJSON(map[string]any{
		"type": "reopen", "client_msg_id": "r1", "data": map[string]any{"entry_id": saved.ID},
	})
	if got := client.expect("ack").field(t, "status"); got != "duplicate" {
		t.Fatalf("retry ack = %v", got)
	}

	// An entry that does not exist is a plain not_found, never an internal failure.
	client.sendJSON(map[string]any{
		"type": "reopen", "client_msg_id": "r2", "data": map[string]any{"entry_id": "e-missing"},
	})
	ack := client.expect("ack")
	if got := ack.field(t, "status"); got != "rejected" {
		t.Fatalf("unknown entry ack = %v", got)
	}
	if got := ack.field(t, "reason"); got != "not_found" {
		t.Errorf("unknown entry reason = %v, want not_found", got)
	}

	client.sendJSON(map[string]any{
		"type": "confirm", "client_msg_id": "c1", "data": map[string]any{"base_version": data.Version},
	})
	if got := client.expect("ack").field(t, "status"); got != "accepted" {
		t.Fatalf("confirm ack = %v", got)
	}
	client.expect("saved")

	revisions, err := rig.db.Revisions(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 || revisions[0].Snapshot.Body != saved.Body {
		t.Fatalf("revisions = %+v, want the first version kept", revisions)
	}
}
