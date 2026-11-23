package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/config"
)

// The update API: it writes requests the host watcher acts on, so every route is checked for what it wrote as well
// as what it answered. Two things must never happen: a signal when no helper is installed, and a restart while a
// reply is streaming.

func updateConfig(t *testing.T) (config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := testConfig()
	cfg.UpdateDir = dir
	return cfg, dir
}

func writeSignal(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func readSignal(t *testing.T, dir, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// Happy: the version reply says how this install was made and whether the helper is alive.
func TestVersionReportsUpdater(t *testing.T) {
	cfg, dir := updateConfig(t)
	writeSignal(t, dir, "heartbeat", time.Now().UTC().Format(time.RFC3339))
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodGet, "/api/version", "localhost:8893", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Version    string `json:"version"`
		Deployment string `json:"deployment"`
		Updater    struct {
			State       string  `json:"state"`
			HeartbeatAt *string `json:"heartbeat_at"`
		} `json:"updater"`
	}
	decode(t, rec, &got)
	if got.Deployment != "docker" || got.Updater.State != "ready" || got.Updater.HeartbeatAt == nil {
		t.Errorf("version = %+v", got)
	}
}

// Happy: a check writes the request and reports it as pending.
func TestUpdateCheckWritesARequest(t *testing.T) {
	cfg, dir := updateConfig(t)
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodPost, "/api/update/check", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if body := readSignal(t, dir, "requested"); !strings.Contains(body, "action=check") {
		t.Errorf("requested = %q", body)
	}
	available := do(t, h, http.MethodGet, "/api/update/available", "localhost:8893", "", nil)
	var got struct {
		Available struct {
			Current string `json:"current"`
		} `json:"available"`
		CheckPending bool `json:"check_pending"`
	}
	decode(t, available, &got)
	if !got.CheckPending {
		t.Error("check_pending = false while the request waits")
	}
	if got.Available.Current != "dev" {
		t.Errorf("current = %q, want the running version", got.Available.Current)
	}
}

// Bad: a second request while one waits is a conflict, not a queued duplicate.
func TestUpdateCheckRefusesASecondRequest(t *testing.T) {
	cfg, dir := updateConfig(t)
	writeSignal(t, dir, "requested", "action=check\n")
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodPost, "/api/update/check", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	var envelope struct {
		Code string `json:"code"`
	}
	decode(t, rec, &envelope)
	if envelope.Code != "update_in_progress" {
		t.Errorf("code = %q, want update_in_progress", envelope.Code)
	}
}

// Bad: starting without a checked digest is refused, because the watcher would have to guess what to install.
func TestUpdateStartWithoutACheckedDigest(t *testing.T) {
	cfg, _ := updateConfig(t)
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodPost, "/api/update/start", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body)
	}
}

// Happy: start installs the digest the page was shown, not whatever the tag means later.
func TestUpdateStartWritesTheCheckedDigest(t *testing.T) {
	cfg, dir := updateConfig(t)
	digest := "ghcr.io/autumnsgrove/sepia@sha256:" + strings.Repeat("b", 64)
	writeSignal(t, dir, "available", `{"latest":"r219","latest_digest":"`+digest+`","update_available":true}`)
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodPost, "/api/update/start", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	body := readSignal(t, dir, "requested")
	if !strings.Contains(body, "action=update") || !strings.Contains(body, "target="+digest) {
		t.Errorf("requested = %q", body)
	}
}

// Bad: a reply in flight means a restart would kill it, so both start and restart are refused with the copy the
// panel shows.
func TestUpdateRefusedWhileAReplyStreams(t *testing.T) {
	cfg, dir := updateConfig(t)
	s := newServer(cfg, Deps{}, slog.New(slog.NewTextHandler(io.Discard, nil)), assets())
	h := s.routes()

	// A turn is running in some session: the hub's map is the truth the refusal reads.
	s.hub.mu.Lock()
	s.hub.turns["session-1"] = &turn{}
	s.hub.mu.Unlock()
	defer func() {
		s.hub.mu.Lock()
		delete(s.hub.turns, "session-1")
		s.hub.mu.Unlock()
	}()

	for _, target := range []string{"/api/update/start", "/api/restart"} {
		rec := do(t, h, http.MethodPost, target, "localhost:8893", "http://localhost:8893", nil)
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s = %d, want 409: %s", target, rec.Code, rec.Body)
		}
		var envelope struct {
			Code string `json:"code"`
		}
		decode(t, rec, &envelope)
		if envelope.Code != "reply_in_progress" {
			t.Errorf("%s code = %q, want reply_in_progress", target, envelope.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "requested")); err == nil {
		t.Error("a refused request was written anyway")
	}
}

// Happy: restart is its own action; it does not ask for an update.
func TestRestartWritesItsOwnRequest(t *testing.T) {
	cfg, dir := updateConfig(t)
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodPost, "/api/restart", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	if body := readSignal(t, dir, "requested"); !strings.Contains(body, "action=restart") {
		t.Errorf("requested = %q", body)
	}
}

// Happy: the status reply carries a result that belongs to the request on record.
func TestUpdateStatusReportsTheResult(t *testing.T) {
	cfg, dir := updateConfig(t)
	now := time.Now().UTC()
	writeSignal(t, dir, "heartbeat", now.Format(time.RFC3339))
	writeSignal(t, dir, "last-request", "action=update\nat="+now.Add(-time.Minute).Format(time.RFC3339)+"\n")
	writeSignal(t, dir, "result", `{"status":"ok","detail":"updated","target":"x","finished_at":"`+now.Format(time.RFC3339)+`"}`)
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodGet, "/api/update/status", "localhost:8893", "", nil)
	var got struct {
		Pending bool `json:"pending"`
		Result  *struct {
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"result"`
	}
	decode(t, rec, &got)
	if got.Pending {
		t.Error("pending = true with no request file")
	}
	if got.Result == nil || got.Result.Status != "ok" {
		t.Errorf("result = %+v", got.Result)
	}
}

// Sad: no helper installed is its own answer, not a generic failure, and nothing is written.
func TestUpdateWithoutAHelper(t *testing.T) {
	h := newTestServer(t, testConfig(), assets())

	version := do(t, h, http.MethodGet, "/api/version", "localhost:8893", "", nil)
	var info struct {
		Deployment string `json:"deployment"`
		Updater    struct {
			State string `json:"state"`
		} `json:"updater"`
	}
	decode(t, version, &info)
	if info.Deployment != "bare" || info.Updater.State != "not_installed" {
		t.Errorf("version = %+v", info)
	}

	rec := do(t, h, http.MethodPost, "/api/update/check", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503: %s", rec.Code, rec.Body)
	}
	var envelope struct {
		Code string `json:"code"`
	}
	decode(t, rec, &envelope)
	if envelope.Code != "unavailable" {
		t.Errorf("code = %q, want unavailable", envelope.Code)
	}
}

// Bad: the update routes sit behind the same Origin check as every state change.
func TestUpdateRoutesRefuseAHostileOrigin(t *testing.T) {
	cfg, dir := updateConfig(t)
	h := newTestServer(t, cfg, assets())

	for _, target := range []string{"/api/update/check", "/api/update/start", "/api/restart"} {
		rec := do(t, h, http.MethodPost, target, "localhost:8893", "https://evil.example", nil)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s = %d, want 403: %s", target, rec.Code, rec.Body)
		}
		var envelope struct {
			Code string `json:"code"`
		}
		decode(t, rec, &envelope)
		if envelope.Code != "bad_origin" {
			t.Errorf("%s code = %q, want bad_origin", target, envelope.Code)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "requested")); err == nil {
		t.Error("a hostile request wrote a signal")
	}
}

// The services screen is told what is configured and what it costs when it is not. It is never told "reachable",
// because this route makes no live call.
func TestServicesReportConfiguration(t *testing.T) {
	cfg, _ := updateConfig(t)
	cfg.OpenRouterKey = "test-key"
	cfg.Model = "vendor/model-pinned"
	h := newTestServer(t, cfg, assets())

	rec := do(t, h, http.MethodGet, "/api/services", "localhost:8893", "", nil)
	var got struct {
		Services []struct {
			Name   string `json:"name"`
			State  string `json:"state"`
			Detail string `json:"detail"`
		} `json:"services"`
	}
	decode(t, rec, &got)
	byName := map[string]struct{ State, Detail string }{}
	for _, svc := range got.Services {
		byName[svc.Name] = struct{ State, Detail string }{svc.State, svc.Detail}
	}
	if got, ok := byName["main_model"]; !ok || got.State != "ok" || got.Detail != "vendor/model-pinned" {
		t.Errorf("main_model = %+v", got)
	}
	if got, ok := byName["tmdb"]; !ok || got.State != "not_configured" {
		t.Errorf("tmdb = %+v, want not_configured without a key", got)
	}
	if got, ok := byName["polaris"]; !ok || got.State != "not_configured" {
		t.Errorf("polaris = %+v", got)
	}
}

// The refusal is for a reply being written, not for a turn that is waiting on her. A parked turn is doing no model
// work, and the restart kills nothing she is reading. Without this, a card left waiting blocks every update until she
// answers it on the very screen the update would refresh (issue #47).
func TestUpdateAllowedWhileATurnWaitsOnHer(t *testing.T) {
	cfg, dir := updateConfig(t)
	digest := "ghcr.io/autumnsgrove/sepia@sha256:" + strings.Repeat("c", 64)
	writeSignal(t, dir, "available", `{"latest":"r220","latest_digest":"`+digest+`","update_available":true}`)
	s := newServer(cfg, Deps{}, slog.New(slog.NewTextHandler(io.Discard, nil)), assets())
	h := s.routes()

	s.hub.mu.Lock()
	s.hub.turns["session-1"] = &turn{}
	s.hub.mu.Unlock()
	defer func() {
		s.hub.mu.Lock()
		delete(s.hub.turns, "session-1")
		s.hub.mu.Unlock()
	}()

	// The turn asks her a question and parks on it, which is what the responder's waiter means.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = s.hub.responder.Wait(ctx, "session-1", "interaction-1")
	}()
	time.Sleep(50 * time.Millisecond)

	rec := do(t, h, http.MethodPost, "/api/update/start", "localhost:8893", "http://localhost:8893", nil)
	cancel()
	<-done
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body)
	}
	if body := readSignal(t, dir, "requested"); !strings.Contains(body, "target="+digest) {
		t.Errorf("requested = %q", body)
	}
}

// A parked turn can have a message queued behind it: that message lives only in memory, was already acknowledged, and
// the client has dropped it from its outbox. A restart would lose her words, so the session stays busy.
func TestUpdateRefusedWhileAParkedTurnHasAQueuedMessage(t *testing.T) {
	cfg, dir := updateConfig(t)
	writeSignal(t, dir, "available", `{"latest":"r221","latest_digest":"ghcr.io/autumnsgrove/sepia@sha256:`+strings.Repeat("d", 64)+`","update_available":true}`)
	s := newServer(cfg, Deps{}, slog.New(slog.NewTextHandler(io.Discard, nil)), assets())
	h := s.routes()

	s.hub.mu.Lock()
	s.hub.turns["session-1"] = &turn{queued: &queuedMessage{text: "and the ending too"}}
	s.hub.mu.Unlock()
	defer func() {
		s.hub.mu.Lock()
		delete(s.hub.turns, "session-1")
		s.hub.mu.Unlock()
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = s.hub.responder.Wait(ctx, "session-1", "interaction-1")
	}()
	time.Sleep(50 * time.Millisecond)

	rec := do(t, h, http.MethodPost, "/api/update/start", "localhost:8893", "http://localhost:8893", nil)
	cancel()
	<-done
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(dir, "requested")); err == nil {
		t.Error("a refused request was written anyway")
	}
}
