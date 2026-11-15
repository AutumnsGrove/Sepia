package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/AutumnsGrove/sepia/internal/backup"
	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The backup routes. They read receipts the host script wrote and, for "Back up now", write one request file into
// the signal directory. The app never runs sqlite3 or rclone and never holds the R2 token
// (docs/slices/S14-backups.md, Decisions).

// backupsView is the JSON the screen draws. It is a view type rather than the service's own struct so the contract
// can name every field and a rename in one place cannot leak into the wire by accident.
type backupsView struct {
	Helper  backupsHelper   `json:"helper"`
	Pending bool            `json:"pending"`
	Last    *domain.Mirror  `json:"last"`
	Runs    []domain.Mirror `json:"runs"`
}

type backupsHelper struct {
	State       string `json:"state"`
	HeartbeatAt string `json:"heartbeat_at,omitempty"`
}

func (s *Server) serveBackups(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Backups are not set up on this server.")
		return
	}
	view, err := s.backups.View(r.Context())
	if err != nil {
		s.log.Error("read backups", "err", err.Error())
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong. Try again.")
		return
	}
	out := backupsView{
		Helper:  backupsHelper{State: view.Helper.State},
		Pending: view.Pending,
		Last:    view.Last,
		Runs:    view.Runs,
	}
	if out.Runs == nil {
		out.Runs = []domain.Mirror{}
	}
	if view.Helper.HeartbeatAt != nil {
		out.Helper.HeartbeatAt = view.Helper.HeartbeatAt.Format(time.RFC3339)
	}
	writeJSON(w, http.StatusOK, out)
}

// backupRun asks the host to run a backup now. A bare deployment has nothing to ask, which is a normal state and a
// different sentence from a failure.
func (s *Server) backupRun(w http.ResponseWriter, r *http.Request) {
	if s.backups == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Backups are not set up on this server.")
		return
	}
	switch err := s.backups.Request(); {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]any{"pending": true})
	case errors.Is(err, backup.ErrNotInstalled):
		writeError(w, http.StatusServiceUnavailable, "unavailable", "Backups are not set up on this server.")
	case errors.Is(err, backup.ErrPending):
		writeError(w, http.StatusConflict, "backup_in_progress", "A backup is already running.")
	default:
		s.log.Error("request backup", "err", err.Error())
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong. Try again.")
	}
}

// backupsService is the Services screen's row for backups. It never makes a live call: the receipt on disk is the
// evidence, which is the whole point of writing one. "ok" means the last run finished and read back clean, "degraded"
// means it ran and failed, and no receipt at all means the helper has not run yet on this machine.
func (s *Server) backupsService(ctx context.Context) map[string]any {
	row := map[string]any{"name": "r2", "state": "not_configured", "detail": "Nightly backup to R2"}
	if s.backups == nil {
		return row
	}
	last, err := s.backups.Last(ctx)
	if err != nil || last == nil {
		return row
	}
	switch last.Status {
	case domain.MirrorOK:
		row["state"] = "ok"
		row["detail"] = fmt.Sprintf("Last run %s", humanBytes(last.Bytes))
	case domain.MirrorFailed:
		row["state"] = "degraded"
		row["detail"] = "The last backup failed"
	case domain.MirrorSkipped:
		row["state"] = "ok"
		row["detail"] = "Last run had nothing new to copy"
	}
	return row
}

// humanBytes is the one place sizes are turned into the short form the screens use. A backup is megabytes, so the
// decimal units the file manager uses are close enough and the number never claims more precision than it has.
func humanBytes(n int64) string {
	const (
		kb = 1000
		mb = kb * 1000
		gb = mb * 1000
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.1f GB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.0f kB", float64(n)/kb)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
