package server

import (
	"errors"
	"net/http"

	"github.com/AutumnsGrove/sepia/internal/update"
)

// The update routes. Every one of them only writes a signal file for the host watcher (docs/UPDATE.md); none of
// them touches Docker, and none of them installs anything itself.

// signal answers a request to the watcher. A refusal is the contract's code, because the panel maps codes to its
// own sentences rather than showing this file's words.
func (s *Server) signal(w http.ResponseWriter, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusAccepted, map[string]any{"pending": true})
	case errors.Is(err, update.ErrNotInstalled):
		writeError(w, http.StatusServiceUnavailable, "unavailable",
			"Updates are not set up on this server.")
	case errors.Is(err, update.ErrPending):
		writeError(w, http.StatusConflict, "update_in_progress", "An update is already running.")
	case errors.Is(err, update.ErrNoTarget), errors.Is(err, update.ErrTargetInvalid):
		writeError(w, http.StatusBadRequest, "invalid_value",
			"There is no new version to install. Check for updates first.")
	default:
		s.log.Error("update signal failed", "err", err.Error())
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong. Try again.")
	}
}

func (s *Server) updateCheck(w http.ResponseWriter, r *http.Request) {
	s.signal(w, s.update.Check())
}

func (s *Server) serveUpdateAvailable(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.update.Available())
}

// updateStart is refused while a reply is being written: the watcher's restart would kill the turn. Drafts and
// entries live in SQLite and survive, but a reply she is reading would not, so the panel says "Finish the reply
// first" (docs/UPDATE.md, rules specific to Sepia).
func (s *Server) updateStart(w http.ResponseWriter, r *http.Request) {
	if s.hub.busy() {
		writeError(w, http.StatusConflict, "reply_in_progress", "Finish the reply first.")
		return
	}
	s.signal(w, s.update.Start(""))
}

func (s *Server) serveUpdateStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.update.Status())
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if s.hub.busy() {
		writeError(w, http.StatusConflict, "reply_in_progress", "Finish the reply first.")
		return
	}
	s.signal(w, s.update.Restart())
}
