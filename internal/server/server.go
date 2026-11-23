// Package server is Sepia's network face: the HTTP server, the session socket, the REST handlers and the Host
// and Origin guard in front of all of them (docs/slices/S09-server.md).
//
// It imports the session engine and the agent and nothing below them (docs/BUILD.md 4), so every fact a response
// carries comes through the engine. Nothing here writes a saved entry: the confirm is the engine's.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/AutumnsGrove/sepia/internal/agent"
	"github.com/AutumnsGrove/sepia/internal/backup"
	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/providers"
	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/update"
	"github.com/AutumnsGrove/sepia/internal/version"
)

// Server timeouts. Every one is bounded: a stalled peer must not hold a connection forever
// (docs/STANDARDS.md 3.2). WriteTimeout is deliberately zero so a socket is not cut off by the server; per
// request work carries its own deadline instead.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownGrace     = 10 * time.Second
	// maxHeaderBytes bounds a request head. Above it the server answers 431 rather than reading on.
	maxHeaderBytes = 16 << 10
)

// versionString is what the health reply and the update panel report.
func versionString() string { return version.Version }

// Deps is what the network face is wired to. Both may be nil in a unit test that only wants the S00 boundary; a
// route that needs one then answers a stable "not built yet" rather than panicking.
type Deps struct {
	Engine *session.Engine
	Agent  *agent.Agent
	// Base is the context a turn runs under. It is not a request's context: a turn continues when the client
	// disappears (docs/PROTOCOL.md 1). Empty means context.Background(), which is what a test wants.
	Base context.Context
	// Now is the clock. Empty means the real one.
	Now func() time.Time
	// Responder bridges an agent's question and the client's tap. The service makes one and passes it to both
	// the agent and this package; nil means the hub makes its own, which is only useful in a test.
	Responder *Responder
	// Live carries the agent's live-only events to the sockets. Nil means there is no bus, which a test may not
	// care about.
	Live *LiveBus
	// Backups is the app's half of the backup flow: receipts the host script wrote, and the request file for
	// "Back up now". Nil means a build with no database behind it, and the routes answer "not set up".
	Backups *backup.Service
}

// New builds the whole HTTP surface. assets is the built frontend, rooted at the directory that holds
// index.html; a test passes an in-memory filesystem.
func New(cfg config.Config, deps Deps, log *slog.Logger, assets fs.FS) http.Handler {
	return newServer(cfg, deps, log, assets).routes()
}

// newServer is New without the chain, so Run can keep the thing it needs to drain.
func newServer(cfg config.Config, deps Deps, log *slog.Logger, assets fs.FS) *Server {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if assets == nil {
		assets = emptyFS{}
	}
	now := deps.Now
	if now == nil {
		now = time.Now
	}
	base := deps.Base
	if base == nil {
		base = context.Background()
	}
	s := &Server{
		cfg: cfg, log: log, assets: assets, engine: deps.Engine, model: deps.Agent, now: now, base: base,
		responder: deps.Responder, update: update.New(cfg.UpdateDir, versionString(), now), backups: deps.Backups,
		coverRefresh: &inflight{keys: map[string]bool{}, tried: map[string]time.Time{}},
	}
	s.hub = newHub(s)
	if deps.Live != nil {
		deps.Live.attach(s.hub.publishLive)
	}
	return s
}

// Server holds what a handler needs. Its fields are read-only after newServer.
type Server struct {
	cfg       config.Config
	log       *slog.Logger
	assets    fs.FS
	engine    *session.Engine
	model     *agent.Agent
	hub       *hub
	now       func() time.Time
	base      context.Context
	responder *Responder
	// update is the app's half of the host watcher handshake. A bare deployment has no directory, and every
	// action on it is refused with a plain "no helper" rather than writing into the working tree.
	update *update.Signal
	// backups is the app's half of the backup flow (S14). It reads the host script's receipts and writes the
	// on-demand request; it never takes a snapshot itself.
	backups *backup.Service
	// coverRefresh is the set of covers this process is already refreshing, with when each was last tried. A page full
	// of stale posters asks the provider once per poster per retry window rather than once per request.
	coverRefresh *inflight
}

// inflight runs one background job per key, and remembers when each last ran.
//
// Guarding only the running set is not enough: a cover that can never be refreshed (a poster the provider has stopped
// serving, or a work row that is gone) would fail, end, and be retried by the next request, so a page holding a few of
// them turns every render into provider traffic. A key is therefore not tried again until the retry window has passed.
type inflight struct {
	mu    sync.Mutex
	keys  map[string]bool
	tried map[string]time.Time
}

// begin reports whether this caller should run the job: not already running, and not tried within the window.
func (i *inflight) begin(key string, now time.Time) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.keys[key] {
		return false
	}
	if at, ok := i.tried[key]; ok && now.Sub(at) < coverRetryAfter {
		return false
	}
	i.keys[key] = true
	i.tried[key] = now
	return true
}

func (i *inflight) done(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	delete(i.keys, key)
}

// coverRefreshTimeout bounds one background refresh. It is generous because nobody is waiting for it: the request
// that triggered it was already answered with a stand-in.
const coverRefreshTimeout = 30 * time.Second

// coverRetryAfter is how long a cover that failed to refresh is left alone. Long enough that a stale poster cannot
// turn reads into provider traffic, short enough that a provider outage does not lose the cover for a day.
const coverRetryAfter = 15 * time.Minute

// routes assembles the chain. Order matters: recover, then the access log, then headers, then the Host check on
// everything, then the Origin check on state changes, then the route.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.Handle("/healthz", getOnly(s.healthz))
	mux.Handle("/ws/sessions/{id}", http.HandlerFunc(s.socket))

	mux.Handle("/api/sessions", s.modeless(http.HandlerFunc(s.sessions)))
	mux.Handle("/api/sessions/{id}", getOnly(s.serveSession))
	mux.Handle("/api/sessions/{id}/media", postOnly(s.uploadMedia))
	mux.Handle("/api/entries", getOnly(s.listEntries))
	mux.Handle("/api/entries/{id}", getOnly(s.serveEntry))
	mux.Handle("/api/entries/{id}/revisions", getOnly(s.listRevisions))
	mux.Handle("/api/entries/{id}/delete", postOnly(s.deleteEntry))
	mux.Handle("/api/entries/{id}/restore", postOnly(s.restoreEntry))
	mux.Handle("/api/works/{id}", getOnly(s.serveWork))
	mux.Handle("/api/search", getOnly(s.search))
	mux.Handle("/api/export", postOnly(s.exportStub))
	mux.Handle("/api/version", getOnly(s.serveVersion))
	mux.Handle("/api/services", getOnly(s.serveServices))
	mux.Handle("/api/backups", getOnly(s.serveBackups))
	mux.Handle("/api/backups/run", postOnly(s.backupRun))
	mux.Handle("/api/settings", http.HandlerFunc(s.settings))
	mux.Handle("/api/update/check", postOnly(s.updateCheck))
	mux.Handle("/api/update/available", getOnly(s.serveUpdateAvailable))
	mux.Handle("/api/update/start", postOnly(s.updateStart))
	mux.Handle("/api/update/status", getOnly(s.serveUpdateStatus))
	mux.Handle("/api/restart", postOnly(s.restart))
	mux.Handle("/media/{id}", getOnly(s.notBuilt))
	mux.Handle("/covers/", getOnly(s.serveCover))
	mux.Handle("/api/thumb", getOnly(s.serveThumb))

	mux.Handle("/api/", getOnly(s.apiNotFound))
	mux.Handle("/", getOnly(s.static))

	var h http.Handler = mux
	h = s.guard(h)
	h = s.securityHeaders(h)
	h = s.compress(h)
	h = s.recover(h)
	h = s.accessLog(h)
	return h
}

// modeless answers a route that needs the engine in a build that has none. It is a stable code, not a panic.
func (s *Server) modeless(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.engine == nil {
			writeError(w, http.StatusServiceUnavailable, "unavailable", "Sepia is still starting. Try again in a moment.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// teardown closes every socket and waits for a running turn, bounded by the grace period. A hijacked connection is
// not tracked by http.Server, so the hub owns it. The close reason is fixed: from the client's side every stop is a
// restart, which is the one thing it can act on.
func (s *Server) teardown(grace time.Duration) {
	s.hub.closeAll(closeGoingAway, "the service is restarting")
	done := make(chan struct{})
	go func() {
		s.hub.turnsWG.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(grace):
		s.log.Warn("turns still running after the grace period", "grace", grace.String())
	}
}

// Run serves until ctx is cancelled or the listener fails, then drains.
func Run(ctx context.Context, cfg config.Config, deps Deps, log *slog.Logger, assets fs.FS) error {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	deps.Base = ctx
	s := newServer(cfg, deps, log, assets)

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Addr, err)
	}

	srv := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	log.Info("sepia listening", "addr", cfg.Addr, "version", versionString(), "dev", cfg.Dev)

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	select {
	case err := <-served:
		s.teardown(shutdownGrace)
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve on %s: %w", cfg.Addr, err)
	case <-ctx.Done():
		// A fresh context on purpose: the request's context is already cancelled (that is what stopped the
		// server), and the drain needs its own deadline.
		shutCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownGrace)
		defer cancel()
		log.Info("sepia draining", "grace", shutdownGrace.String())
		// Sockets first: a client that is waiting on a turn is told the service is going away, and a turn that
		// is already running is given the grace period to finish (docs/PROTOCOL.md 1).
		s.teardown(shutdownGrace)
		if err := srv.Shutdown(shutCtx); err != nil {
			return fmt.Errorf("drain: %w", err)
		}
		return nil
	}
}

// healthz is the one shape an outside monitor parses, so it is written by hand rather than through an encoder
// that might reorder or add a field. It is liveness, not readiness: the database was checked at startup.
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	body, err := json.Marshal(struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}{Status: "ok", Version: versionString()})
	if err != nil { // a struct of two strings cannot fail; this keeps the linter honest
		writeError(w, http.StatusInternalServerError, "internal", "Something went wrong.")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		s.log.Debug("healthz write", "err", err)
	}
}

// apiNotFound answers an unknown /api path with the envelope, so a client never has to parse HTML.
func (s *Server) apiNotFound(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotFound, "not_found", "No such endpoint.")
}

// notBuilt answers a route whose slice has not been built yet with a stable code, so the frontend can say one
// plain sentence (S16, S18, S19).
func (s *Server) notBuilt(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "not_implemented", "This part of Sepia is not built yet.")
}

// getOnly refuses everything but GET and HEAD with the envelope and an Allow header, so a wrong method looks
// like every other error to the client.
func getOnly(h http.HandlerFunc) http.Handler {
	return methodOnly(h, http.MethodGet, http.MethodHead)
}

func postOnly(h http.HandlerFunc) http.Handler {
	return methodOnly(h, http.MethodPost)
}

func methodOnly(h http.HandlerFunc, methods ...string) http.Handler {
	allow := strings.Join(methods, ", ")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, method := range methods {
			if r.Method == method {
				h(w, r)
				return
			}
		}
		w.Header().Set("Allow", allow)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "That method is not allowed here.")
	})
}

// serveCover serves a stored cover from the data directory. Only a bare file name is accepted, so a path
// traversal in a media or cover id is refused rather than read (S09 bad route).
func (s *Server) serveCover(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/covers/")
	if name == "" || name != path.Base(name) || strings.ContainsAny(name, `/\\`+"\x00") {
		writeError(w, http.StatusNotFound, "not_found", "No such cover.")
		return
	}
	f, info, err := openCover(s.cfg.DataDir, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No such cover.")
		return
	}
	defer func() { _ = f.Close() }()
	// G3's rule is that a cover older than 180 days is refreshed on its next use, and a use is a read. A stale file is
	// still served — the board is usually offline and the library should keep its posters — but it is served with
	// no-cache so the browser revalidates rather than holding a copy of its own, and the work is refreshed in the
	// background. When the provider answers that the cover is gone, the refresh purges the file instead: that is what
	// stops TMDB content outliving the cap, rather than withholding an image that is still on disk.
	if s.now().Sub(info.ModTime()) >= providers.CoverMaxAge {
		w.Header().Set("Cache-Control", "no-cache")
		s.refreshCover(r.Context(), path.Join(providers.CoverDir, name))
	} else {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Set("Content-Type", coverType(name))
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// openCover opens one stored cover by bare file name, resolving both sides of the directory check: a temp directory on
// macOS is a symlink and the real path is not the one os.TempDir handed out, and a name that is not under the cover
// directory is refused rather than read (S09 bad route).
func openCover(dataDir, name string) (*os.File, os.FileInfo, error) {
	dir := filepath.Join(dataDir, providers.CoverDir)
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, nil, err
	}
	resolvedFile, err := filepath.EvalSymlinks(filepath.Join(dir, name))
	if err != nil || !strings.HasPrefix(resolvedFile, resolvedDir+string(os.PathSeparator)) {
		return nil, nil, os.ErrNotExist
	}
	f, err := os.Open(resolvedFile) //nolint:gosec // G703: resolved under the cover directory and checked above
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > providers.MaxCoverBytes {
		_ = f.Close()
		return nil, nil, os.ErrNotExist
	}
	return f, info, nil
}

// serveThumb serves a candidate's cover from this origin. The page's CSP is img-src 'self' data:, and a candidate's
// cover lived on a provider's image host, so the picker drew its stand-in for every candidate (issue #53). The payload
// names this route instead, and the app fetches the provider's copy through the same cache, limits and rate limits a
// saved cover uses.
//
// Nothing here is proxied from a host the config does not name: the download refuses a host that is not one of the
// provider's image bases, and a provider name that is not one the finder was built with. A cover that cannot be
// fetched is a 404 and the card draws its stand-in, which is how every other cover failure is handled.
func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	provider, src := query.Get("provider"), query.Get("src")
	// No model means no finder in this build: the thumb is decoration, so it is a missing image rather than a refusal
	// the card would have to explain.
	if s.model == nil {
		writeError(w, http.StatusNotFound, "not_found", "No such cover.")
		return
	}
	cover, err := s.model.CandidateCover(r.Context(), provider, src)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No such cover.")
		return
	}
	name := path.Base(cover.Path)
	f, info, err := openCover(s.cfg.DataDir, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "No such cover.")
		return
	}
	defer func() { _ = f.Close() }()
	// A thumb is not a work's cover: there is no row to refresh from a path, and the provider's own URL came with the
	// request that fetched it. It is served for a day and fetched again when the cache window closes.
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("Content-Type", coverType(name))
	http.ServeContent(w, r, name, info.ModTime(), f)
}

// purgeCover deletes a cover the provider says it no longer has, and clears the row's reference to it. This is the
// half that enforces the cap: a file the provider has disowned must not stay in data/covers/ for the next six months.
// The context is the refresh's, which is already detached from the request that triggered it.
func (s *Server) purgeCover(ctx context.Context, coverPath string) {
	s.removeCoverFile(coverPath)
	if err := s.model.ForgetCoverPath(ctx, coverPath); err != nil && s.log != nil {
		s.log.Warn("cover reference not cleared", "cover", coverPath, "reason", err)
	}
}

// removeCoverFile deletes one stored cover, resolving it exactly the way the serve route does: the directory and the
// file are both resolved and the file must sit under the directory, so a name that is not a bare file name cannot
// reach outside it. The path arrives from the request that found the cover stale, so it is never trusted as a path.
func (s *Server) removeCoverFile(coverPath string) {
	name := path.Base(coverPath)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\\`+"\x00") {
		return
	}
	dir := filepath.Join(s.cfg.DataDir, providers.CoverDir)
	resolvedDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return
	}
	resolvedFile, err := filepath.EvalSymlinks(filepath.Join(dir, name))
	if err != nil || !strings.HasPrefix(resolvedFile, resolvedDir+string(os.PathSeparator)) {
		return
	}
	if err := os.Remove(resolvedFile); err != nil && !errors.Is(err, os.ErrNotExist) && s.log != nil {
		s.log.Warn("cover purge failed", "cover", coverPath, "reason", err)
	}
}

// refreshCover replaces a cover that has outlived G3's 180 days. It runs in the background on the server's base
// context, because the request that found it stale has already been answered, and a failure is only logged: the
// next request finds the file just as stale and tries again.
func (s *Server) refreshCover(requestCtx context.Context, coverPath string) {
	if s.model == nil || s.coverRefresh == nil {
		return
	}
	if !s.coverRefresh.begin(coverPath, s.now()) {
		return
	}
	go func() {
		defer s.coverRefresh.done(coverPath)
		// The request that found the cover stale has already been answered, so its cancellation must not abort the
		// refresh; its values are kept, which is what a trace or a logger in the chain would need.
		ctx, cancel := context.WithTimeout(context.WithoutCancel(requestCtx), coverRefreshTimeout)
		defer cancel()
		err := s.model.RefreshCoverByPath(ctx, coverPath)
		switch {
		case err == nil:
		case errors.Is(err, agent.ErrCoverGone):
			s.purgeCover(ctx, coverPath)
		case s.log != nil:
			s.log.Warn("cover refresh failed", "cover", coverPath, "reason", err)
		}
	}()
}

func coverType(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}

// requestID is short, random and not derived from anything the client sent, so it can be logged and shown
// without carrying a review or a title.
func requestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b[:])
}

// emptyFS stands in when a build has not been made, so the service still starts and the health check works.
type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
