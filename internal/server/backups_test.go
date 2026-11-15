package server

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AutumnsGrove/sepia/internal/backup"
	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The backup routes: receipts the host script wrote, imported on read, and one request file for "Back up now". The
// app never takes a snapshot itself, so everything here is a file or a row.

type mirrorStore struct {
	rows   map[string]domain.Mirror
	now    time.Time
	broken bool
}

func newMirrorStore() *mirrorStore {
	return &mirrorStore{rows: map[string]domain.Mirror{}, now: time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)}
}

func (m *mirrorStore) RecordMirror(_ context.Context, mirror domain.Mirror) error {
	if m.broken {
		return os.ErrInvalid
	}
	if _, ok := m.rows[mirror.ID]; !ok {
		m.rows[mirror.ID] = mirror
	}
	return nil
}

func (m *mirrorStore) Mirrors(_ context.Context, limit int) ([]domain.Mirror, error) {
	if m.broken {
		return nil, os.ErrInvalid
	}
	out := make([]domain.Mirror, 0, len(m.rows))
	for _, row := range m.rows {
		out = append(out, row)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].RanAt.After(out[i].RanAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// backupsConfig builds a deployment with a data directory holding the script's receipts and a signal directory.
func backupsConfig(t *testing.T) (config.Config, string) {
	t.Helper()
	data := t.TempDir()
	signal := t.TempDir()
	cfg := testConfig()
	cfg.DataDir = data
	cfg.UpdateDir = signal
	if err := os.MkdirAll(filepath.Join(data, "backups"), 0o750); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}
	body := strings.Join([]string{
		`{"run":"run-1","ran_at":"2026-11-14T08:30:00Z","target":"r2sepia:sepia-backups/db","status":"ok","bytes":4000000,"detail":""}`,
		`{"run":"run-1","ran_at":"2026-11-14T08:30:00Z","target":"r2sepia:sepia-backups/covers","status":"failed","bytes":0,"detail":"AccessDenied"}`,
		`{"run":"run-2","ran_at":"2026-11-15T08:30:00Z","target":"r2sepia:sepia-backups/db","status":"ok","bytes":4200000,"detail":""}`,
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(data, "backups", "receipts.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatalf("write receipts: %v", err)
	}
	beat := filepath.Join(data, "backups", "heartbeat")
	if err := os.WriteFile(beat, []byte("2026-11-15T08:30:00Z\n"), 0o600); err != nil {
		t.Fatalf("write heartbeat: %v", err)
	}
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	if err := os.Chtimes(beat, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("age heartbeat: %v", err)
	}
	return cfg, signal
}

func backupsServer(t *testing.T, cfg config.Config, store *mirrorStore) http.Handler {
	t.Helper()
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, time.UTC)
	return New(cfg, Deps{
		Backups: backup.New(cfg.DataDir, cfg.UpdateDir, store, func() time.Time { return now }),
	}, slog.New(slog.DiscardHandler), assets())
}

// Happy: the receipts on disk become the screen's runs, the newest is the last, and the helper is reported alive.
func TestBackupsReadsReceiptsAndHelper(t *testing.T) {
	store := newMirrorStore()
	cfg, _ := backupsConfig(t)
	h := backupsServer(t, cfg, store)

	rec := do(t, h, http.MethodGet, "/api/backups", "localhost:8893", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Helper struct {
			State       string `json:"state"`
			HeartbeatAt string `json:"heartbeat_at"`
		} `json:"helper"`
		Pending bool `json:"pending"`
		Last    struct {
			ID     string `json:"id"`
			Bytes  int64  `json:"bytes"`
			Status string `json:"status"`
		} `json:"last"`
		Runs []struct {
			ID string `json:"id"`
		} `json:"runs"`
	}
	decode(t, rec, &got)

	if got.Helper.State != "ready" {
		t.Errorf("helper = %q, want ready", got.Helper.State)
	}
	if got.Helper.HeartbeatAt == "" {
		t.Error("heartbeat_at is empty")
	}
	if len(got.Runs) != 3 {
		t.Fatalf("runs = %d, want the three receipts: %+v", len(got.Runs), got.Runs)
	}
	if got.Last.ID != "run-2:r2sepia:sepia-backups/db" || got.Last.Bytes != 4200000 {
		t.Errorf("last = %+v, want the newest run with its size", got.Last)
	}
	if len(store.rows) != 3 {
		t.Errorf("store has %d receipts after the read, want 3", len(store.rows))
	}
	if got.Pending {
		t.Error("pending is true with no request")
	}
}

// Happy: Back up now writes the request the host path unit watches.
func TestBackupsRunWritesTheRequest(t *testing.T) {
	store := newMirrorStore()
	cfg, signal := backupsConfig(t)
	h := backupsServer(t, cfg, store)

	rec := do(t, h, http.MethodPost, "/api/backups/run", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	body, err := os.ReadFile(filepath.Join(signal, "backup-requested"))
	if err != nil {
		t.Fatalf("read request: %v", err)
	}
	if string(body) != "action=backup\n" {
		t.Errorf("request = %q", body)
	}
}

// Bad: a second tap joins the request already waiting rather than queueing another run.
func TestBackupsRunRefusedWhileOneIsPending(t *testing.T) {
	store := newMirrorStore()
	cfg, _ := backupsConfig(t)
	h := backupsServer(t, cfg, store)

	if rec := do(t, h, http.MethodPost, "/api/backups/run", "localhost:8893", "http://localhost:8893", nil); rec.Code != http.StatusAccepted {
		t.Fatalf("first request = %d: %s", rec.Code, rec.Body)
	}
	rec := do(t, h, http.MethodPost, "/api/backups/run", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("second request = %d, want 409: %s", rec.Code, rec.Body)
	}
	var envelope struct {
		Code string `json:"code"`
	}
	decode(t, rec, &envelope)
	if envelope.Code != "backup_in_progress" {
		t.Errorf("code = %q, want backup_in_progress", envelope.Code)
	}
}

// Bad: the same Origin rule as every other state change.
func TestBackupsRunChecksOrigin(t *testing.T) {
	store := newMirrorStore()
	cfg, signal := backupsConfig(t)
	h := backupsServer(t, cfg, store)

	rec := do(t, h, http.MethodPost, "/api/backups/run", "localhost:8893", "http://evil.example", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403: %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(signal, "backup-requested")); err == nil {
		t.Error("a request with a hostile Origin was written anyway")
	}
}

// Sad: a bare deployment has no helper to ask, and the screen can read an empty history.
func TestBackupsOnABareDeployment(t *testing.T) {
	store := newMirrorStore()
	cfg := testConfig()
	cfg.DataDir = t.TempDir()
	cfg.UpdateDir = ""
	h := backupsServer(t, cfg, store)

	rec := do(t, h, http.MethodGet, "/api/backups", "localhost:8893", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Helper struct {
			State string `json:"state"`
		} `json:"helper"`
		Runs []any `json:"runs"`
	}
	decode(t, rec, &got)
	if got.Helper.State != "not_installed" {
		t.Errorf("helper = %q, want not_installed", got.Helper.State)
	}
	if got.Runs == nil {
		t.Error("runs is null, want an empty array")
	}

	rec = do(t, h, http.MethodPost, "/api/backups/run", "localhost:8893", "http://localhost:8893", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("run on a bare deployment = %d, want 503: %s", rec.Code, rec.Body)
	}
}

// Sad: the database is the thing that failed, and the route says so without leaking it.
func TestBackupsReportsAStoreFailure(t *testing.T) {
	store := newMirrorStore()
	store.broken = true
	cfg, _ := backupsConfig(t)
	h := backupsServer(t, cfg, store)

	rec := do(t, h, http.MethodGet, "/api/backups", "localhost:8893", "", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500: %s", rec.Code, rec.Body)
	}
	if strings.Contains(rec.Body.String(), "invalid argument") {
		t.Errorf("body leaks the internal error: %s", rec.Body)
	}
}

// The Services row: it reads the newest receipt rather than making a live call to R2.
func TestServicesReportsTheLastBackup(t *testing.T) {
	store := newMirrorStore()
	cfg, _ := backupsConfig(t)
	h := backupsServer(t, cfg, store)

	rec := do(t, h, http.MethodGet, "/api/services", "localhost:8893", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Services []struct {
			Name   string `json:"name"`
			State  string `json:"state"`
			Detail string `json:"detail"`
		} `json:"services"`
	}
	decode(t, rec, &got)
	for _, service := range got.Services {
		if service.Name != "r2" {
			continue
		}
		if service.State != "ok" {
			t.Errorf("r2 state = %q, want ok", service.State)
		}
		if service.Detail != "Last run 4.2 MB" {
			t.Errorf("r2 detail = %q, want the last run's size", service.Detail)
		}
		return
	}
	t.Fatal("no r2 row on the services screen")
}
