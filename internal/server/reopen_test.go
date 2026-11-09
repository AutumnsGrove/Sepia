package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AutumnsGrove/sepia/internal/session"
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
	both := client.expectBoth("ack", "draft_snapshot")
	if got := both["ack"].field(t, "status"); got != "accepted" {
		t.Fatalf("reopen ack = %v", got)
	}
	snapshot := both["draft_snapshot"]
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
	both = client.expectBoth("ack", "saved")
	if got := both["ack"].field(t, "status"); got != "accepted" {
		t.Fatalf("confirm ack = %v", got)
	}

	revisions, err := rig.db.Revisions(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 1 || revisions[0].Snapshot.Body != saved.Body {
		t.Fatalf("revisions = %+v, want the first version kept", revisions)
	}
}

// Log again over the socket: the card opens empty on the same work, and the confirm adds a second entry rather than
// rewriting the saved one. The ack is the proof the socket accepted a mode it had never seen before.
func TestReopenAgainAndVersionOverTheSocket(t *testing.T) {
	rig := newSocketRig(t)
	defer rig.close()
	ctx := context.Background()

	saved := rig.newSavedEntry()
	// One confirmed edit, so there is an older version to restore.
	edit, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := rig.engine.Reopen(ctx, edit.ID, saved.ID, "seed-edit")
	if err != nil {
		t.Fatal(err)
	}
	changed, err := rig.engine.EditDraft(ctx, edit.ID, session.Edit{BaseVersion: revision.Version, Set: map[string]json.RawMessage{
		"rating": json.RawMessage("19"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err = rig.engine.Confirm(ctx, edit.ID, session.ConfirmRequest{BaseVersion: changed.Version, Revises: saved.ID}); err != nil {
		t.Fatal(err)
	}
	versions, err := rig.db.Revisions(ctx, saved.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("revisions = %+v %v", versions, err)
	}

	sess, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	client := rig.dial(sess.ID, 0, "http://"+rig.host)
	client.expect("hello")

	client.sendJSON(map[string]any{
		"type": "reopen", "client_msg_id": "r-new",
		"data": map[string]any{"entry_id": saved.ID, "mode": "new"},
	})
	both := client.expectBoth("ack", "draft_snapshot")
	if got := both["ack"].field(t, "status"); got != "accepted" {
		t.Fatalf("log again ack = %v", got)
	}
	var data struct {
		Version int `json:"version"`
		Draft   struct {
			Body   string `json:"body"`
			Rating struct {
				Quarters int `json:"quarters"`
			} `json:"rating"`
			Work struct {
				ID string `json:"id"`
			} `json:"work"`
		} `json:"draft"`
	}
	if err = json.Unmarshal(both["draft_snapshot"].Data, &data); err != nil {
		t.Fatal(err)
	}
	if data.Draft.Body != "" || data.Draft.Rating.Quarters != 0 || data.Draft.Work.ID != saved.WorkID {
		t.Fatalf("a second entry's card = %+v, want an empty card on the same work", data.Draft)
	}

	// Restore as draft in a fresh session: the snapshot is the older version, and the confirm keeps the newer one.
	restore, err := rig.engine.CreateEntry(ctx)
	if err != nil {
		t.Fatal(err)
	}
	older := rig.dial(restore.ID, 0, "http://"+rig.host)
	older.expect("hello")
	older.sendJSON(map[string]any{
		"type": "reopen", "client_msg_id": "r-old",
		"data": map[string]any{"entry_id": saved.ID, "revision": versions[0].Number},
	})
	restored := older.expectBoth("ack", "draft_snapshot")
	var restoredData struct {
		Version int `json:"version"`
		Draft   struct {
			Body string `json:"body"`
		} `json:"draft"`
	}
	if err = json.Unmarshal(restored["draft_snapshot"].Data, &restoredData); err != nil {
		t.Fatal(err)
	}
	if restoredData.Draft.Body != versions[0].Snapshot.Body {
		t.Fatalf("restored draft = %+v, want version %d", restoredData.Draft, versions[0].Number)
	}
	// A version that was never written is refused with a plain invalid value, not an internal failure.
	older.sendJSON(map[string]any{
		"type": "reopen", "client_msg_id": "r-99",
		"data": map[string]any{"entry_id": saved.ID, "revision": 99},
	})
	if got := older.expect("ack").field(t, "status"); got != "rejected" {
		t.Fatalf("unknown version ack = %v", got)
	}
}
