package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/agent"
	"github.com/AutumnsGrove/sepia/internal/banks"
	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/decisions"
	"github.com/AutumnsGrove/sepia/internal/domain"
	"github.com/AutumnsGrove/sepia/internal/providers"
	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/store"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// The happy route from the slice plan, end to end: a scripted model, the real pipeline, a real socket, and her
// confirm. Nothing here is mocked except the model and the providers, which is exactly what the world is for.

func newFlowRig(t *testing.T) *socketRig { return newFlowRigGated(t, nil, world.SampleChatScript()...) }

// newFlowRigSplit is the same rig with the split scenario: two real media, so the flow asks the medium first.
func newFlowRigSplit(t *testing.T) *socketRig {
	return newFlowRigDecided(t, nil, world.ScriptSplitDecisions, world.SampleSplitChatScript()...)
}

// newFlowRigGated is the same rig with a wrapper around the model's HTTP handler, so a lifecycle test can hold a
// turn open at the exact moment the service is asked to stop, and with its own script, so a turn can finish without
// a person tapping anything. A nil wrapper and no script are the plain flow rig.
func newFlowRigGated(t *testing.T, wrap func(http.Handler) http.Handler, script ...world.ChatTurn) *socketRig {
	return newFlowRigDecided(t, wrap, world.ScriptSampleDecisions, script...)
}

// newFlowRigDecided is the rig with a chosen pre-read, so a test can script a split medium or a confident one.
func newFlowRigDecided(t *testing.T, wrap func(http.Handler) http.Handler, scriptDecisions func(*world.Decisions),
	script ...world.ChatTurn,
) *socketRig {
	t.Helper()
	ctx := context.Background()
	clock := world.NewFixedClock(time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC))

	db, err := store.Open(ctx, store.Options{Path: filepath.Join(t.TempDir(), "sepia.db"), Now: clock.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err = db.VerifyTriggers(ctx); err != nil {
		t.Fatal(err)
	}
	engine := session.New(session.Config{Store: db, Now: clock.Now, ServerVersion: "test"})

	chat := world.NewChat("openrouter", clock)
	chat.Script(script...)
	var chatHandler http.Handler = chat
	if wrap != nil {
		chatHandler = wrap(chat)
	}
	chatSrv := httptest.NewServer(chatHandler)
	t.Cleanup(chatSrv.Close)
	tmdb := world.Start("tmdb", clock)
	t.Cleanup(tmdb.Close)
	tmdb.LoadBodies(world.SampleTMDB())
	ol := world.Start("openlibrary", clock)
	t.Cleanup(ol.Close)
	ol.LoadBodies(world.SampleOpenLibrary())

	reading := world.NewDecisions(clock)
	scriptDecisions(reading)
	registry, err := decisions.Load(filepath.Join("..", "..", "config", "decisions"))
	if err != nil {
		t.Fatal(err)
	}
	banksSet, err := banks.LoadDir(filepath.Join("..", "..", "config", "questions"))
	if err != nil {
		t.Fatal(err)
	}
	finder := providers.NewFinder(providers.Config{
		TMDBKey: "test-key", TMDBBaseURL: tmdb.URL(), TMDBCoverBase: tmdb.URL() + "/t/p/w92",
		OpenLibraryBaseURL: ol.URL(), OpenLibraryCoverBase: ol.URL() + "/covers",
		TMDBCallInterval: -1, OpenLibraryCallInterval: -1,
		DataDir: t.TempDir(), Clock: clock,
	})

	responder := NewResponder()
	live := NewLiveBus()
	model, err := agent.New(agent.Config{
		Store: db, Session: engine, Finder: finder, Banks: banksSet,
		Decisions: decisions.New(decisions.Config{Registry: registry, Decider: reading}),
		Model:     agent.PinnedModel, BaseURL: chatSrv.URL + "/v1", APIKey: "test-key",
		Responder: responder, Live: live.Publish, Logger: slog.New(slog.DiscardHandler),
		Now: clock.Now,
	})
	if err != nil {
		t.Fatal(err)
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host := ln.Addr().String()
	cfg := config.Config{AllowedHosts: []string{host, "localhost:8893"}, Addr: host, DataDir: t.TempDir()}
	inner := newServer(cfg, Deps{
		Engine: engine, Agent: model, Responder: responder, Live: live, Now: clock.Now,
	}, slog.New(slog.DiscardHandler), nil)
	srv := &httptest.Server{
		Listener: ln,
		Config:   &http.Server{Handler: inner.routes()},
	}
	srv.Start()
	return &socketRig{t: t, server: srv, inner: inner, db: db, engine: engine, host: host}
}

func TestScriptedSessionFromMessageToSaved(t *testing.T) {
	rig := newFlowRig(t)
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

	// The turn streams, looks the work up, takes the candidate and builds the card.
	var version int
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		ev, ok := client.next()
		if !ok {
			t.Fatalf("socket closed during the turn (code %d), saw %v", ev.closeCode, seen)
		}
		seen[ev.Type] = true
		if ev.Type == "interaction_request" {
			// The card asks which work she meant. Answer it the way a person would, over the same socket.
			var data struct {
				InteractionID string `json:"interaction_id"`
				Kind          string `json:"kind"`
				Payload       struct {
					Candidates []struct {
						ID string `json:"id"`
					} `json:"candidates"`
				} `json:"payload"`
			}
			if err = json.Unmarshal(ev.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data.Kind != "choose_candidate" || len(data.Payload.Candidates) == 0 {
				t.Fatalf("unexpected interaction %s: %s", data.Kind, ev.Data)
			}
			client.sendJSON(map[string]any{
				"type": "interaction_response", "client_msg_id": "i1",
				"data": map[string]any{
					"interaction_id": data.InteractionID,
					"value":          map[string]any{"value": data.Payload.Candidates[0].ID},
				},
			})
			if got := client.expect("ack").field(t, "status"); got != "accepted" {
				t.Fatalf("interaction ack = %v", got)
			}
			continue
		}
		if ev.Type == "draft_snapshot" {
			var data struct {
				Version int `json:"version"`
				Draft   struct {
					Body   string `json:"body"`
					Rating struct {
						Quarters int `json:"quarters"`
					} `json:"rating"`
					Work struct {
						Title string `json:"title"`
					} `json:"work"`
					CanConfirm bool `json:"can_confirm"`
				} `json:"draft"`
			}
			if err = json.Unmarshal(ev.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data.Draft.CanConfirm {
				version = data.Version
				if data.Draft.Work.Title != "The Lantern Keeper" {
					t.Fatalf("work = %q", data.Draft.Work.Title)
				}
				if data.Draft.Rating.Quarters != 18 {
					t.Fatalf("rating = %d, want 18", data.Draft.Rating.Quarters)
				}
				if data.Draft.Body != "the ending landed but it was too long" {
					t.Fatalf("body = %q", data.Draft.Body)
				}
				break
			}
		}
	}
	if version == 0 {
		t.Fatalf("the card never became confirmable: %v", seen)
	}
	for _, want := range []string{"user_message", "assistant_start", "tool_start", "tool_end", "draft_snapshot"} {
		if !seen[want] {
			t.Errorf("the turn never sent %s", want)
		}
	}

	// Her confirm is the only commit.
	client.sendJSON(map[string]any{
		"type": "confirm", "client_msg_id": "c1", "data": map[string]any{"base_version": version},
	})
	both := client.expectBoth("ack", "saved")
	if got := both["ack"].field(t, "status"); got != "accepted" {
		t.Fatalf("confirm ack = %v", got)
	}
	saved := both["saved"]
	entryID, _ := saved.field(t, "entry_id").(string)
	if entryID == "" {
		t.Fatal("saved carried no entry id")
	}

	library, _, err := rig.engine.Library(ctx, domain.LibraryQuery{}, domain.Cursor{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(library) != 1 || library[0].ID != entryID {
		t.Fatalf("library = %+v", library)
	}
	// "last night" is the night before the phone's date, which is what makes the date hers and not the potato's.
	if library[0].Body != "the ending landed but it was too long" ||
		!strings.Contains(library[0].FinishedOn, "2026-10-31") {
		t.Errorf("saved entry = %+v", library[0])
	}
}

// The split scenario is the whole logging screen in one turn: the medium card, the candidate card, Describe, the
// questions, then a card she can confirm. It runs the real pipeline and a real socket, and only the model, the
// providers and the decisions backend are the world.
func TestSplitScenarioAsksEveryCardOverTheSocket(t *testing.T) {
	rig := newFlowRigSplit(t)
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

	answers := map[string]any{
		"choose_medium":    map[string]any{"value": "book"},
		"choose_candidate": map[string]any{"value": "openlibrary|OL1W"},
		"describe":         map[string]any{"selected": map[string][]string{"setting": {"lived-in"}}},
		"question_form":    map[string]any{"skip_all": true},
	}
	var asked []string
	var version int
	for i := 0; i < 400 && version == 0; i++ {
		ev, ok := client.next()
		if !ok {
			t.Fatalf("socket closed during the flow (code %d), asked %v", ev.closeCode, asked)
		}
		switch ev.Type {
		case "interaction_request":
			var data struct {
				InteractionID string `json:"interaction_id"`
				Kind          string `json:"kind"`
			}
			if err = json.Unmarshal(ev.Data, &data); err != nil {
				t.Fatal(err)
			}
			value, known := answers[data.Kind]
			if !known {
				t.Fatalf("unexpected interaction %s: %s", data.Kind, ev.Data)
			}
			asked = append(asked, data.Kind)
			client.sendJSON(map[string]any{
				"type": "interaction_response", "client_msg_id": "i-" + data.Kind,
				"data": map[string]any{"interaction_id": data.InteractionID, "value": value},
			})
			if got := client.expect("ack").field(t, "status"); got != "accepted" {
				t.Fatalf("%s ack = %v", data.Kind, got)
			}
		case "draft_snapshot":
			var data struct {
				Version int `json:"version"`
				Draft   struct {
					CanConfirm bool `json:"can_confirm"`
				} `json:"draft"`
			}
			if err = json.Unmarshal(ev.Data, &data); err != nil {
				t.Fatal(err)
			}
			if data.Draft.CanConfirm {
				version = data.Version
			}
		}
	}
	if version == 0 {
		t.Fatalf("the card never became confirmable, asked %v", asked)
	}
	want := []string{"choose_medium", "choose_candidate", "describe", "question_form"}
	if strings.Join(asked, ",") != strings.Join(want, ",") {
		t.Errorf("cards asked = %v, want %v", asked, want)
	}

	client.sendJSON(map[string]any{
		"type": "confirm", "client_msg_id": "c1", "data": map[string]any{"base_version": version},
	})
	both := client.expectBoth("ack", "saved")
	if got := both["ack"].field(t, "status"); got != "accepted" {
		t.Fatalf("confirm ack = %v", got)
	}
}
