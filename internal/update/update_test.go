package update

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fixed clock is the whole point of this package being pure about time: the heartbeat's staleness, the result's
// skew and the request's timestamp are all compared, and none of that should depend on when the suite ran.
var testNow = time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return testNow }

func newTestSignal(t *testing.T) *Signal {
	t.Helper()
	s := New(t.TempDir(), "r214.a1b2c3d", clock)
	return s
}

func writeSignal(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func stamp(at time.Time) string { return at.Format(time.RFC3339) }

// Happy: a fresh heartbeat is ready and the app can say so.
func TestUpdaterState(t *testing.T) {
	cases := []struct {
		name   string
		beat   string
		state  string
		hasAt  bool
		absent bool
	}{
		{name: "fresh", beat: stamp(testNow.Add(-30 * time.Second)), state: StateReady, hasAt: true},
		{name: "stale", beat: stamp(testNow.Add(-4 * time.Minute)), state: StateStale, hasAt: true},
		{name: "from the future", beat: stamp(testNow.Add(2 * time.Minute)), state: StateStale, hasAt: true},
		{name: "unreadable", beat: "yesterday", state: StateNotInstalled},
		{name: "missing", absent: true, state: StateNotInstalled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSignal(t)
			if !tc.absent {
				writeSignal(t, s.Dir(), "heartbeat", tc.beat)
			}
			got := s.Updater()
			if got.State != tc.state {
				t.Errorf("state = %q, want %q", got.State, tc.state)
			}
			if (got.HeartbeatAt != nil) != tc.hasAt {
				t.Errorf("heartbeat_at present = %v, want %v", got.HeartbeatAt != nil, tc.hasAt)
			}
		})
	}
}

// Bad: a bare deployment has no directory, so every action is refused rather than writing into the working tree.
func TestBareDeploymentRefusesEverything(t *testing.T) {
	s := New("", "dev", clock)
	if got := s.Updater(); got.State != StateNotInstalled {
		t.Errorf("state = %q", got.State)
	}
	for name, err := range map[string]error{"check": s.Check(), "start": s.Start(""), "restart": s.Restart()} {
		if !errors.Is(err, ErrNotInstalled) {
			t.Errorf("%s error = %v, want ErrNotInstalled", name, err)
		}
	}
}

// Happy: a check writes the request and the app can see it is pending.
func TestCheckWritesARequest(t *testing.T) {
	s := newTestSignal(t)
	if err := s.Check(); err != nil {
		t.Fatalf("Check: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(s.Dir(), "requested"))
	if err != nil {
		t.Fatalf("requested: %v", err)
	}
	if !strings.Contains(string(body), "action=check") {
		t.Errorf("requested = %q, want action=check", body)
	}
	if !s.Available().CheckPending {
		t.Error("check_pending = false after a check")
	}
	if !s.Status().Pending {
		t.Error("status.pending = false after a check")
	}
	// The request records when it was asked, so a result can be judged against it later.
	last, err := os.ReadFile(filepath.Join(s.Dir(), "last-request"))
	if err != nil {
		t.Fatalf("last-request: %v", err)
	}
	if !strings.Contains(string(last), "at="+stamp(testNow)) {
		t.Errorf("last-request = %q, want the request instant", last)
	}
}

// Bad: a second request while one waits is refused: the watcher would delete the first one's request unseen.
func TestSecondRequestIsRefused(t *testing.T) {
	s := newTestSignal(t)
	if err := s.Check(); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if err := s.Check(); !errors.Is(err, ErrPending) {
		t.Errorf("second check error = %v, want ErrPending", err)
	}
	if err := s.Start(""); !errors.Is(err, ErrPending) {
		t.Errorf("start during a check = %v, want ErrPending", err)
	}
}

// Happy: an update installs the digest the page was shown, from the available file.
func TestStartUsesTheCheckedDigest(t *testing.T) {
	s := newTestSignal(t)
	digest := "ghcr.io/autumnsgrove/sepia@sha256:" + strings.Repeat("b", 64)
	writeSignal(t, s.Dir(), "available", `{"latest":"r219.e4f5a6b","latest_digest":"`+digest+`","update_available":true}`)
	if err := s.Start(""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(s.Dir(), "requested"))
	if err != nil {
		t.Fatalf("requested: %v", err)
	}
	if !strings.Contains(string(body), "action=update") || !strings.Contains(string(body), "target="+digest) {
		t.Errorf("requested = %q, want an update for the checked digest", body)
	}
}

// Bad: nothing to install is a refusal, not a request the watcher has to guess about.
func TestStartWithoutADigest(t *testing.T) {
	s := newTestSignal(t)
	if err := s.Start(""); !errors.Is(err, ErrNoTarget) {
		t.Errorf("Start with no digest = %v, want ErrNoTarget", err)
	}
	writeSignal(t, s.Dir(), "available", `{"latest_digest":"ghcr.io/autumnsgrove/sepia@sha256:`+strings.Repeat("a", 64)+`"}`)
	// A hostile or malformed target is refused at the edge too, before the watcher ever sees it.
	for _, target := range []string{"sepia:latest", "ghcr.io/other/sepia@sha256:" + strings.Repeat("b", 64)} {
		if err := s.Start(target); !errors.Is(err, ErrTargetInvalid) {
			t.Errorf("Start(%q) = %v, want ErrTargetInvalid", target, err)
		}
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "requested")); err == nil {
		t.Error("a refused start wrote a request")
	}
}

// A digest whose host or path starts with a dash is shaped like a command line flag once a script reads it. The
// available file is the watcher's, and the repo check below passes when the file and the target agree, so the
// pattern is the only thing that refuses it.
func TestStartRefusesADigestThatStartsWithADash(t *testing.T) {
	for _, digest := range []string{
		"-sneaky/sepia@sha256:" + strings.Repeat("a", 64),
		"ghcr.io/-sneaky@sha256:" + strings.Repeat("a", 64),
	} {
		s := newTestSignal(t)
		writeSignal(t, s.Dir(), "available", `{"latest_digest":"`+digest+`","update_available":true}`)
		if err := s.Start(""); !errors.Is(err, ErrTargetInvalid) {
			t.Errorf("Start with %q = %v, want ErrTargetInvalid", digest, err)
		}
	}
}

// Happy: restart is its own action and installs nothing.
func TestRestartWritesItsOwnAction(t *testing.T) {
	s := newTestSignal(t)
	if err := s.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(s.Dir(), "requested"))
	if !strings.Contains(string(body), "action=restart") {
		t.Errorf("requested = %q", body)
	}
}

// The available file is passed through with the running version filled in, because only the app knows it.
func TestAvailableFillsCurrentAndError(t *testing.T) {
	s := newTestSignal(t)
	if got := s.Available().Available; got.Current != "r214.a1b2c3d" {
		t.Errorf("current = %q, want the running version", got.Current)
	}
	writeSignal(t, s.Dir(), "available",
		`{"checked_at":"`+stamp(testNow)+`","update_available":false,"error":"GHCR is unreachable"}`)
	got := s.Available().Available
	if got.Error != "GHCR is unreachable" {
		t.Errorf("error = %q", got.Error)
	}
	if got.CheckedAt == nil || !got.CheckedAt.Equal(testNow) {
		t.Errorf("checked_at = %v", got.CheckedAt)
	}
	if got.Current != "r214.a1b2c3d" {
		t.Errorf("current = %q", got.Current)
	}
}

// Happy: a result finished after the request is shown, with the skew allowance for the two clocks.
func TestStatusShowsAResultForTheRequest(t *testing.T) {
	s := newTestSignal(t)
	writeSignal(t, s.Dir(), "heartbeat", stamp(testNow))
	writeSignal(t, s.Dir(), "last-request", "action=update\nat="+stamp(testNow)+"\n")
	// The phone's clock may be a minute behind the potato's, so a result a little "before" the request is still
	// that request's.
	writeSignal(t, s.Dir(), "result",
		`{"status":"ok","detail":"updated","target":"x","finished_at":"`+stamp(testNow.Add(-50*time.Second))+`"}`)
	got := s.Status()
	if got.Result == nil || got.Result.Status != "ok" {
		t.Fatalf("result = %+v, want ok", got.Result)
	}
}

// Bad: a result from before the request is a leftover, not this update's answer.
func TestStatusIgnoresAStaleResult(t *testing.T) {
	s := newTestSignal(t)
	writeSignal(t, s.Dir(), "heartbeat", stamp(testNow))
	writeSignal(t, s.Dir(), "last-request", "action=update\nat="+stamp(testNow)+"\n")
	writeSignal(t, s.Dir(), "result",
		`{"status":"failed","detail":"last night","target":"x","finished_at":"`+stamp(testNow.Add(-10*time.Minute))+`"}`)
	if got := s.Status().Result; got != nil {
		t.Errorf("result = %+v, want it ignored as older than the request", got)
	}
}

// Bad: without a live heartbeat, a result file is just a file someone could have written. It is not trusted.
func TestStatusDistrustsAResultWithoutAHeartbeat(t *testing.T) {
	s := newTestSignal(t)
	writeSignal(t, s.Dir(), "last-request", "action=update\nat="+stamp(testNow)+"\n")
	writeSignal(t, s.Dir(), "result",
		`{"status":"ok","detail":"fabricated","target":"x","finished_at":"`+stamp(testNow)+`"}`)
	if got := s.Status().Result; got != nil {
		t.Errorf("result = %+v, want it distrusted without a heartbeat", got)
	}
}

// Bad: an unreadable signal file is an error, never a silent zero that would look like a fresh install.
func TestUnreadableSignalFileIsAnError(t *testing.T) {
	s := newTestSignal(t)
	big := make([]byte, maxSignalFile+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(s.Dir(), "available"), big, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if view := s.Available(); view.Available.Error == "" {
		t.Fatal("an oversized available file was read as a clean answer")
	}
}

// The JSON shapes are the contract's, not the package's. This is the one place they are written out, so a rename
// in the schema is a failure here rather than a panel that quietly sees nothing.
func TestJSONShapes(t *testing.T) {
	s := newTestSignal(t)
	writeSignal(t, s.Dir(), "heartbeat", stamp(testNow))
	writeSignal(t, s.Dir(), "available", `{"latest":"r219","latest_digest":"d","checked_at":"`+stamp(testNow)+`","update_available":true}`)
	writeSignal(t, s.Dir(), "last-request", "action=update\nat="+stamp(testNow)+"\n")
	writeSignal(t, s.Dir(), "result", `{"status":"ok","detail":"d","target":"t","finished_at":"`+stamp(testNow)+`"}`)

	version, err := json.Marshal(s.VersionInfo("docker"))
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	available, err := json.Marshal(s.Available())
	if err != nil {
		t.Fatalf("available: %v", err)
	}
	status, err := json.Marshal(s.Status())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for name, body := range map[string][]byte{"version": version, "available": available, "status": status} {
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("%s is not JSON: %v (%s)", name, err, body)
		}
	}
	var v struct {
		Version    string `json:"version"`
		Deployment string `json:"deployment"`
		Updater    struct {
			State       string  `json:"state"`
			HeartbeatAt *string `json:"heartbeat_at"`
		} `json:"updater"`
	}
	if err := json.Unmarshal(version, &v); err != nil {
		t.Fatalf("version: %v", err)
	}
	if v.Version != "r214.a1b2c3d" || v.Deployment != "docker" || v.Updater.State != StateReady || v.Updater.HeartbeatAt == nil {
		t.Errorf("version = %s", version)
	}
}

// check_pending means a check, not just any request. The panel branches on it, and the whole point of the distinction
// is that a queued update must not render as "Checking" or a queued check as "Updating" (S13 review).
func TestCheckPendingIsOnlyACheck(t *testing.T) {
	s := newTestSignal(t)
	digest := "ghcr.io/autumnsgrove/sepia@sha256:" + strings.Repeat("b", 64)
	writeSignal(t, s.Dir(), "available", `{"latest_digest":"`+digest+`","update_available":true}`)
	if err := s.Start(""); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.Available().CheckPending {
		t.Error("check_pending = true for a queued update")
	}
	if !s.Status().Pending {
		t.Error("status.pending = false for a queued update")
	}

	// And a queued check is both pending and a check.
	other := newTestSignal(t)
	if err := other.Check(); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !other.Available().CheckPending {
		t.Error("check_pending = false for a queued check")
	}
	if !other.Status().Pending {
		t.Error("status.pending = false for a queued check")
	}
}
