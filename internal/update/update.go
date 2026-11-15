// Package update is the app's half of the update handshake. The container never touches Docker: it writes a small
// request into a directory the host watcher also sees, and reads back the watcher's heartbeat, availability and
// result (docs/UPDATE.md, compose/README.md).
//
// The package owns the judgement as well as the files: what a heartbeat older than three minutes means, when a
// result belongs to the request that is on screen, and which digest may be installed. Those rules are pure over an
// injected clock so every state is a test rather than a race with the potato's uptime.
package update

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// The states GET /api/version reports, from the contract's enum.
const (
	StateReady        = "ready"
	StateNotInstalled = "not_installed"
	StateStale        = "stale"
)

const (
	// HeartbeatFresh is how long a heartbeat speaks for. The timer writes one every minute, so three misses is a
	// watcher that is not running rather than one caught between ticks.
	HeartbeatFresh = 3 * time.Minute
	// ResultSkew is the allowance between the phone's clock and the potato's, which can differ by up to a minute.
	ResultSkew = time.Minute
	// maxSignalFile bounds every file this package reads. They are ours, written by one script, and a large one is
	// a bug or a hand edit, not a reason to read it all.
	maxSignalFile = 64 << 10

	requestedName   = "requested"
	lastRequestName = "last-request"
	heartbeatName   = "heartbeat"
	availableName   = "available"
	resultName      = "result"
)

// Errors the server maps to the contract's codes rather than a generic failure.
var (
	ErrNotInstalled  = errors.New("update: the update helper is not installed")
	ErrPending       = errors.New("update: a request is already waiting")
	ErrNoTarget      = errors.New("update: there is no digest to install")
	ErrTargetInvalid = errors.New("update: that is not a digest of this image")
)

// targetPattern is a digest reference to a GHCR image. A tag is refused: an update must install the build the page
// named, not whatever the tag means when the watcher runs.
var targetPattern = regexp.MustCompile(`^[a-z0-9.-]+/[a-z0-9._/-]+@sha256:[0-9a-f]{64}$`)

// Signal is the handshake directory. One instance per service; the mutex is what keeps two taps on Update from
// both seeing an empty request file.
type Signal struct {
	dir     string
	version string
	now     func() time.Time

	mu sync.Mutex
}

// New builds the handshake for a directory. An empty directory is a bare deployment: every action is refused with
// ErrNotInstalled, which the panel shows as "no update helper" rather than as an outage.
func New(dir, version string, now func() time.Time) *Signal {
	if now == nil {
		now = time.Now
	}
	return &Signal{dir: dir, version: version, now: now}
}

// Dir is the handshake directory, empty on a bare deployment.
func (s *Signal) Dir() string { return s.dir }

// Installed reports whether the directory is there. A mounted but missing directory is the same as no mount: there
// is nothing to signal.
func (s *Signal) Installed() bool {
	if s.dir == "" {
		return false
	}
	info, err := os.Stat(s.dir)
	return err == nil && info.IsDir()
}

// Updater is the helper's liveness, as GET /api/version reports it.
type Updater struct {
	State       string     `json:"state"`
	HeartbeatAt *time.Time `json:"heartbeat_at,omitempty"`
}

// VersionInfo is GET /api/version.
type VersionInfo struct {
	Version    string  `json:"version"`
	Deployment string  `json:"deployment"`
	Updater    Updater `json:"updater"`
}

// VersionInfo builds the reply. deployment is the config's answer, because only the composition root knows how the
// process was installed.
func (s *Signal) VersionInfo(deployment string) VersionInfo {
	return VersionInfo{Version: s.version, Deployment: deployment, Updater: s.Updater()}
}

// Updater reads the heartbeat. An unreadable one is not_installed: a helper that cannot say it is alive is not one
// the app should claim is running.
func (s *Signal) Updater() Updater {
	if !s.Installed() {
		return Updater{State: StateNotInstalled}
	}
	body, err := s.read(heartbeatName)
	if err != nil {
		return Updater{State: StateNotInstalled}
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(string(body)))
	if err != nil {
		return Updater{State: StateNotInstalled}
	}
	at = at.UTC()
	now := s.now()
	if now.Sub(at) > HeartbeatFresh || at.Sub(now) > ResultSkew {
		return Updater{State: StateStale, HeartbeatAt: &at}
	}
	return Updater{State: StateReady, HeartbeatAt: &at}
}

// Available is what the last check found. Current is this process's version, filled in here because the watcher
// cannot know it.
type Available struct {
	Current         string     `json:"current"`
	Latest          string     `json:"latest,omitempty"`
	LatestDigest    string     `json:"latest_digest,omitempty"`
	CheckedAt       *time.Time `json:"checked_at,omitempty"`
	UpdateAvailable bool       `json:"update_available"`
	Error           string     `json:"error,omitempty"`
}

// AvailableView is GET /api/update/available.
type AvailableView struct {
	Available    Available `json:"available"`
	CheckPending bool      `json:"check_pending"`
}

// availableFile is the watcher's on-disk shape. It is separate from the API shape so the two can differ without a
// decode accident: the file has no current, and the API always does.
type availableFile struct {
	Latest          string `json:"latest"`
	LatestDigest    string `json:"latest_digest"`
	CheckedAt       string `json:"checked_at"`
	UpdateAvailable bool   `json:"update_available"`
	Error           string `json:"error"`
}

// Available reads the last check. A missing file is "no check yet", which is not an error; a file that cannot be
// read is reported in Error, because the panel must not say "up to date" when it could not look.
func (s *Signal) Available() AvailableView {
	view := AvailableView{Available: Available{Current: s.version}, CheckPending: s.pending()}
	if !s.Installed() {
		return view
	}
	body, err := s.read(availableName)
	if errors.Is(err, os.ErrNotExist) {
		return view
	}
	if err != nil {
		view.Available.Error = "The update helper wrote something Sepia could not read."
		return view
	}
	var file availableFile
	if decodeErr := json.Unmarshal(body, &file); decodeErr != nil {
		view.Available.Error = "The update helper wrote something Sepia could not read."
		return view
	}
	view.Available.Latest = file.Latest
	view.Available.LatestDigest = file.LatestDigest
	view.Available.UpdateAvailable = file.UpdateAvailable
	view.Available.Error = file.Error
	if file.CheckedAt != "" {
		if at, err := time.Parse(time.RFC3339, file.CheckedAt); err == nil {
			at = at.UTC()
			view.Available.CheckedAt = &at
		}
	}
	return view
}

// Result is the last update's outcome, as the watcher recorded it.
type Result struct {
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
	Target     string    `json:"target,omitempty"`
	FinishedAt time.Time `json:"finished_at"`
}

// StatusView is GET /api/update/status.
type StatusView struct {
	Pending bool    `json:"pending"`
	Result  *Result `json:"result,omitempty"`
}

type resultFile struct {
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Target     string `json:"target"`
	FinishedAt string `json:"finished_at"`
}

// Status reads the last result and decides whether it answers the request on screen. A result is shown only when
// the helper is alive, a request was recorded, the result is not older than that request by more than the clock
// skew, and nothing new is pending. A leftover or hand-written file is not an answer.
func (s *Signal) Status() StatusView {
	view := StatusView{Pending: s.pending()}
	if !s.Installed() || view.Pending {
		return view
	}
	if s.Updater().State != StateReady {
		return view
	}
	last, err := s.lastRequest()
	if err != nil {
		return view
	}
	body, err := s.read(resultName)
	if err != nil {
		return view
	}
	var file resultFile
	if decodeErr := json.Unmarshal(body, &file); decodeErr != nil {
		return view
	}
	finished, err := time.Parse(time.RFC3339, file.FinishedAt)
	if err != nil {
		return view
	}
	finished = finished.UTC()
	now := s.now()
	if last.At.Sub(finished) > ResultSkew || finished.Sub(now) > ResultSkew {
		return view
	}
	view.Result = &Result{Status: file.Status, Detail: file.Detail, Target: file.Target, FinishedAt: finished}
	return view
}

// Check asks the helper to look for a newer image.
func (s *Signal) Check() error {
	return s.ask("check", "", nil)
}

// Start asks the helper to install a digest. An empty target means the digest of the last check, which is the one
// the page was shown; a target that is not a digest of that same image is refused here as well as in the watcher.
func (s *Signal) Start(target string) error {
	return s.ask("update", "", func() (string, error) { return s.resolveTarget(target) })
}

// resolveTarget picks and checks the digest to install. It runs under ask's lock, after the installed and pending
// checks, so a bare deployment answers ErrNotInstalled rather than "no digest".
func (s *Signal) resolveTarget(target string) (string, error) {
	available := s.Available().Available
	if target == "" {
		target = available.LatestDigest
	}
	switch {
	case target == "":
		return "", ErrNoTarget
	case !targetPattern.MatchString(target):
		return "", ErrTargetInvalid
	case available.LatestDigest != "" && repoOf(target) != repoOf(available.LatestDigest):
		return "", ErrTargetInvalid
	}
	return target, nil
}

// Restart recreates the container with the image already pinned. It pulls nothing and installs nothing.
func (s *Signal) Restart() error {
	return s.ask("restart", "", nil)
}

// ask writes the request and the record of when it was made. Both writes are atomic, because the path unit reacts
// to the file's appearance and must never read half a request.
func (s *Signal) ask(action, target string, resolve func() (string, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.Installed() {
		return ErrNotInstalled
	}
	if s.pending() {
		return ErrPending
	}
	if resolve != nil {
		var err error
		if target, err = resolve(); err != nil {
			return err
		}
	}
	request := "action=" + action + "\n"
	if target != "" {
		request += "target=" + target + "\n"
	}
	record := request + "at=" + s.now().UTC().Format(time.RFC3339) + "\n"
	// The record goes first: if the process dies between the two writes there is a record with no request, which is
	// harmless, rather than a request with no record, which the status could not judge.
	if err := s.write(lastRequestName, record); err != nil {
		return err
	}
	return s.write(requestedName, request)
}

type lastRequest struct {
	At time.Time
}

func (s *Signal) lastRequest() (lastRequest, error) {
	body, err := s.read(lastRequestName)
	if err != nil {
		return lastRequest{}, err
	}
	var last lastRequest
	for _, line := range strings.Split(string(body), "\n") {
		if value, ok := strings.CutPrefix(line, "at="); ok {
			at, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
			if err != nil {
				return lastRequest{}, err
			}
			last.At = at.UTC()
			return last, nil
		}
	}
	return lastRequest{}, errors.New("update: the last request has no instant")
}

func (s *Signal) pending() bool {
	if s.dir == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(s.dir, requestedName))
	return err == nil
}

func (s *Signal) read(name string) ([]byte, error) {
	file, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, maxSignalFile+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxSignalFile {
		return nil, errors.New("update: signal file is over the limit")
	}
	return body, nil
}

func (s *Signal) write(name, body string) error {
	tmp, err := os.CreateTemp(s.dir, name+".tmp.*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(body); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(s.dir, name))
}

// repoOf is the image without its tag or digest, so two references can be compared for being the same image.
func repoOf(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	if slash := strings.LastIndex(ref, "/"); strings.LastIndex(ref, ":") > slash {
		ref = ref[:strings.LastIndex(ref, ":")]
	}
	return ref
}
