package backup

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/AutumnsGrove/sepia/internal/domain"
)

// The files the host script and the app share. They live under the data directory because they describe the data,
// and in the signal directory because a request has to be seen by a unit that has no database.
const (
	receiptsName  = "receipts.jsonl"
	heartbeatName = "heartbeat"
	requestName   = "backup-requested"
	backupsDir    = "backups"
)

// StaleAfter is when a nightly timer that has not run is not idle but broken: the screen says the helper went quiet
// rather than showing a stale "last run" as if it were current. A day plus two hours, so one slow morning is not
// treated as an outage.
const StaleAfter = 26 * time.Hour

// PendingFor is when a request nobody picked up stops being "pending". Ten minutes is far longer than a snapshot and
// a mirror of a few megabytes take; past it the honest answer is that the helper did not answer, which is a
// different sentence from "waiting".
const PendingFor = 10 * time.Minute

// ErrNotInstalled is the bare deployment: there is no signal directory, so there is nothing to ask.
var ErrNotInstalled = errors.New("backup: there is no signal directory")

// ErrPending is a request already waiting to be picked up.
var ErrPending = errors.New("backup: a backup is already requested")

// Store is the slice of the database this package needs. It is defined here, where it is used, and the real store
// satisfies it without knowing about this package.
type Store interface {
	RecordMirror(ctx context.Context, m domain.Mirror) error
	Mirrors(ctx context.Context, limit int) ([]domain.Mirror, error)
}

// Service answers the Backups screen and asks the host to run a backup now.
type Service struct {
	dataDir   string
	signalDir string
	store     Store
	now       func() time.Time
}

// New builds the service. signalDir is empty in a bare deployment, which is a normal state: the screen says there is
// no helper instead of failing.
func New(dataDir, signalDir string, store Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{dataDir: dataDir, signalDir: signalDir, store: store, now: now}
}

// View is everything the Backups screen draws.
type View struct {
	// Helper says whether the nightly run is installed and alive, which is a different question from what the last
	// run did.
	Helper Helper
	// Pending is true while a request written by the button has not been picked up yet.
	Pending bool
	// Last is the newest receipt, or nil when no backup has ever been recorded.
	Last *domain.Mirror
	// Runs are the newest receipts, most recent first.
	Runs []domain.Mirror
}

// Helper is the state of the host-side timer.
type Helper struct {
	// State is one of "ready", "not_installed" or "stale".
	State       string
	HeartbeatAt *time.Time
}

// Helper states.
const (
	HelperReady        = "ready"
	HelperNotInstalled = "not_installed"
	HelperStale        = "stale"
)

// View imports whatever the script has recorded since the last look, then answers the screen. Importing on read
// rather than on a timer keeps one code path: the only reader is the screen, so it is also the only importer.
func (s *Service) View(ctx context.Context) (View, error) {
	if _, err := s.Import(ctx); err != nil {
		return View{}, err
	}
	runs, err := s.store.Mirrors(ctx, MaxReceipts)
	if err != nil {
		return View{}, err
	}
	view := View{
		Helper:  s.helper(),
		Pending: s.pending(),
		Runs:    runs,
	}
	if len(runs) > 0 {
		last := runs[0]
		view.Last = &last
	}
	return view, nil
}

// Last returns the newest receipt, which is what the Services screen shows as the backups row's detail. It imports
// first for the same reason View does: the Services screen must not report yesterday's run because the Backups
// screen has not been opened yet.
func (s *Service) Last(ctx context.Context) (*domain.Mirror, error) {
	if _, err := s.Import(ctx); err != nil {
		return nil, err
	}
	runs, err := s.store.Mirrors(ctx, 1)
	if err != nil {
		return nil, err
	}
	if len(runs) == 0 {
		return nil, nil
	}
	return &runs[0], nil
}

// Import reads the receipt file and writes anything new to the mirrors table. A missing file is not an error: it
// means the helper has never run. It returns how many receipts were written.
func (s *Service) Import(ctx context.Context) (int, error) {
	parsed, err := s.parse()
	if err != nil {
		return 0, err
	}
	written := 0
	for _, mirror := range parsed.Items {
		if err := s.store.RecordMirror(ctx, mirror); err != nil {
			return written, fmt.Errorf("backup: import receipt %s: %w", mirror.ID, err)
		}
		written++
	}
	return written, nil
}

// parse reads the newest end of the receipt file.
func (s *Service) parse() (Parsed, error) {
	path := filepath.Join(s.dataDir, backupsDir, receiptsName)
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Parsed{}, nil
		}
		return Parsed{}, fmt.Errorf("backup: open receipts: %w", err)
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return Parsed{}, fmt.Errorf("backup: stat receipts: %w", err)
	}
	var reader io.Reader = f
	offset := info.Size() - MaxReceiptFileBytes
	if offset > 0 {
		// Append-only, so the newest runs are at the end. Seek, then drop the half line the seek landed in.
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return Parsed{}, fmt.Errorf("backup: seek receipts: %w", err)
		}
		buffered := bufio.NewReader(f)
		if _, err := buffered.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
			return Parsed{}, fmt.Errorf("backup: read receipts: %w", err)
		}
		reader = buffered
	}
	parsed := Parse(io.LimitReader(reader, MaxReceiptFileBytes))
	parsed.Truncated = parsed.Truncated || offset > 0
	return parsed, nil
}

// Request asks the host to run a backup now. It only writes a file: the app never runs rclone, and a bare
// deployment has no helper to ask.
func (s *Service) Request() error {
	if s.signalDir == "" {
		return ErrNotInstalled
	}
	info, err := os.Stat(s.signalDir)
	if err != nil || !info.IsDir() {
		return ErrNotInstalled
	}
	request := filepath.Join(s.signalDir, requestName)
	if existing, statErr := os.Stat(request); statErr == nil && s.now().Sub(existing.ModTime()) < PendingFor {
		return ErrPending
	}
	tmp, err := os.CreateTemp(s.signalDir, requestName+".tmp.")
	if err != nil {
		return fmt.Errorf("backup: write request: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.WriteString("action=backup\n"); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return fmt.Errorf("backup: write request: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("backup: write request: %w", err)
	}
	// A rename is the only way to publish a file the host may read at any moment.
	if err := os.Rename(name, request); err != nil {
		_ = os.Remove(name)
		return fmt.Errorf("backup: write request: %w", err)
	}
	return nil
}

// helper reads the heartbeat the script writes at the start of every run.
func (s *Service) helper() Helper {
	info, err := os.Stat(filepath.Join(s.dataDir, backupsDir, heartbeatName))
	if err != nil {
		return Helper{State: HelperNotInstalled}
	}
	at := info.ModTime().UTC()
	state := HelperReady
	if s.now().Sub(at) > StaleAfter {
		state = HelperStale
	}
	return Helper{State: state, HeartbeatAt: &at}
}

// pending is true while a request exists and is young enough to still be served.
func (s *Service) pending() bool {
	info, err := os.Stat(filepath.Join(s.signalDir, requestName))
	if err != nil {
		return false
	}
	return s.now().Sub(info.ModTime()) < PendingFor
}
