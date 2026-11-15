package archtest

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The watcher is shell, but it is the piece that can take the board down, so it is tested like code: a temporary
// install tree, a scripted fake Docker, git and registry, and every route the slice names. The fakes are files on
// PATH rather than a mocking library, which is also how a future failure is reproduced by hand.

const (
	oldDigest    = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	newDigest    = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	brokenDigest = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	imageRepo    = "ghcr.io/autumnsgrove/sepia"
)

type watcherTree struct {
	root   string
	signal string
	env    string
	log    string
	bin    string
}

// newWatcher builds an install tree shaped like the potato's: the script under compose/watcher, a .env pinning an
// image, and the signal directory the app writes into.
func newWatcher(t *testing.T) *watcherTree {
	t.Helper()
	root := t.TempDir()
	w := &watcherTree{
		root:   root,
		signal: filepath.Join(root, "update-signal"),
		env:    filepath.Join(root, ".env"),
		log:    filepath.Join(root, "fake.log"),
		bin:    filepath.Join(root, "bin"),
	}
	for _, dir := range []string{
		filepath.Join(root, "compose", "watcher"), filepath.Join(root, "data"),
		filepath.Join(root, "config"), w.signal, w.bin,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	script, err := os.ReadFile(filepath.Join("..", "..", "compose", "watcher", "update.sh"))
	if err != nil {
		t.Fatalf("read update.sh: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "compose", "watcher", "update.sh"), script, 0o755); err != nil {
		t.Fatalf("copy update.sh: %v", err)
	}
	w.pin(t, oldDigest)
	for name, body := range fakes() {
		if err := os.WriteFile(filepath.Join(w.bin, name), []byte(body), 0o755); err != nil {
			t.Fatalf("write fake %s: %v", name, err)
		}
	}
	return w
}

func (w *watcherTree) pin(t *testing.T, digest string) {
	t.Helper()
	body := strings.Join([]string{
		"SEPIA_IMAGE=" + imageRepo + "@" + digest,
		"SEPIA_UID=1000",
		"SEPIA_GID=1000",
		"SEPIA_TMDB_API_KEY=",
		"SEPIA_OPENROUTER_API_KEY=",
		"",
	}, "\n")
	if err := os.WriteFile(w.env, []byte(body), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
}

func (w *watcherTree) pinned() string {
	body, err := os.ReadFile(w.env)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "SEPIA_IMAGE=") {
			return strings.TrimPrefix(line, "SEPIA_IMAGE=")
		}
	}
	return ""
}

func (w *watcherTree) request(t *testing.T, action, target string) {
	t.Helper()
	body := "action=" + action + "\n"
	if target != "" {
		body += "target=" + target + "\n"
	}
	if err := os.WriteFile(filepath.Join(w.signal, "requested"), []byte(body), 0o644); err != nil {
		t.Fatalf("write requested: %v", err)
	}
}

// run executes the watcher with the fakes first on PATH and the given environment overrides.
func (w *watcherTree) run(t *testing.T, extra map[string]string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(context.Background(), "bash",
		append([]string{filepath.Join(w.root, "compose", "watcher", "update.sh")}, args...)...)
	env := []string{
		"PATH=" + w.bin + ":" + os.Getenv("PATH"),
		"HOME=" + w.root,
		"SEPIA_INSTALL_DIR=" + w.root,
		"SEPIA_UPDATE_DIR=" + w.signal,
		"SEPIA_HEALTH_TRIES=2",
		"SEPIA_HEALTH_DELAY=0",
		"FAKE_LOG=" + w.log,
		"FAKE_DIGEST=" + newDigest,
		"FAKE_VERSION=r219.e4f5a6b",
	}
	for key, value := range extra {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (w *watcherTree) calls(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(w.log)
	if err != nil {
		if os.IsNotExist(err) {
			return ""
		}
		t.Fatalf("read fake log: %v", err)
	}
	return string(body)
}

func (w *watcherTree) heartbeat(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(w.signal, "heartbeat"))
	if err != nil {
		t.Fatalf("read heartbeat: %v", err)
	}
	return strings.TrimSpace(string(body))
}

type availableFile struct {
	Latest          string `json:"latest"`
	LatestDigest    string `json:"latest_digest"`
	CheckedAt       string `json:"checked_at"`
	UpdateAvailable bool   `json:"update_available"`
	Error           string `json:"error"`
}

type resultFile struct {
	Status     string `json:"status"`
	Detail     string `json:"detail"`
	Target     string `json:"target"`
	FinishedAt string `json:"finished_at"`
}

func (w *watcherTree) available(t *testing.T) availableFile {
	t.Helper()
	var got availableFile
	readJSON(t, filepath.Join(w.signal, "available"), &got)
	return got
}

func (w *watcherTree) result(t *testing.T) resultFile {
	t.Helper()
	var got resultFile
	readJSON(t, filepath.Join(w.signal, "result"), &got)
	return got
}

func readJSON(t *testing.T, path string, dst any) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("%s is not the JSON the contract names: %v\n%s", path, err, body)
	}
}

func (w *watcherTree) requestGone(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(w.signal, "requested")); !os.IsNotExist(err) {
		t.Error("requested was not cleared, so every later trigger would replay it")
	}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("%q is not an RFC 3339 instant: %v", value, err)
	}
	return parsed
}

// Happy: the heartbeat is fresh, so the app can say the helper is running.
func TestWatcherWritesHeartbeat(t *testing.T) {
	w := newWatcher(t)
	if _, err := w.run(t, nil, "--heartbeat"); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	beat := mustTime(t, w.heartbeat(t))
	if d := time.Since(beat); d > time.Minute || d < -time.Minute {
		t.Errorf("heartbeat is %v from now", d)
	}
}

// Happy: a check finds the newer digest and names its version, and leaves the request cleared.
func TestWatcherCheckFindsNewer(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "check", "")
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("check: %v", err)
	}
	got := w.available(t)
	if !got.UpdateAvailable {
		t.Error("update_available = false, want true")
	}
	if want := imageRepo + "@" + newDigest; got.LatestDigest != want {
		t.Errorf("latest_digest = %q, want %q", got.LatestDigest, want)
	}
	if got.Latest != "r219.e4f5a6b" {
		t.Errorf("latest = %q, want the version annotation", got.Latest)
	}
	mustTime(t, got.CheckedAt)
	w.requestGone(t)
	mustTime(t, w.heartbeat(t))
}

// Happy: a check with nothing new says so.
func TestWatcherCheckWhenAlreadyPinned(t *testing.T) {
	w := newWatcher(t)
	w.pin(t, newDigest)
	w.request(t, "check", "")
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("check: %v", err)
	}
	if got := w.available(t); got.UpdateAvailable {
		t.Error("update_available = true while .env already pins that digest")
	}
}

// Sad: an unreachable registry is recorded, not reported as up to date.
func TestWatcherCheckWhenRegistryIsDown(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "check", "")
	if _, err := w.run(t, map[string]string{"FAKE_REGISTRY_FAIL": "1"}); err == nil {
		t.Fatal("a failed check exited zero")
	}
	got := w.available(t)
	if got.Error == "" {
		t.Fatal("available carries no error; the panel would say up to date")
	}
	if !strings.Contains(got.Error, "GHCR") {
		t.Errorf("error %q does not name the registry", got.Error)
	}
	if got.UpdateAvailable {
		t.Error("update_available = true after a failed check")
	}
	w.requestGone(t)
}

// Happy: an update fast-forwards the checkout, pins, pulls, recreates and reports done.
func TestWatcherUpdateSucceeds(t *testing.T) {
	w := newWatcher(t)
	target := imageRepo + "@" + newDigest
	w.request(t, "update", target)
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	got := w.result(t)
	if got.Status != "ok" {
		t.Fatalf("status = %q, detail = %q", got.Status, got.Detail)
	}
	if got.Target != target {
		t.Errorf("target = %q, want %q", got.Target, target)
	}
	mustTime(t, got.FinishedAt)
	if want := target; w.pinned() != want {
		t.Errorf(".env pins %q, want %q", w.pinned(), want)
	}
	calls := w.calls(t)
	for _, want := range []string{"pull --ff-only origin main", "compose pull sepia", "compose up -d --force-recreate --no-deps sepia"} {
		if !strings.Contains(calls, want) {
			t.Errorf("commands did not run %q:\n%s", want, calls)
		}
	}
	w.requestGone(t)
}

// Happy: an update with no target resolves the latest digest itself.
func TestWatcherUpdateResolvesWithoutATarget(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "update", "")
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := w.result(t); got.Status != "ok" {
		t.Fatalf("status = %q, detail = %q", got.Status, got.Detail)
	}
	if want := imageRepo + "@" + newDigest; w.pinned() != want {
		t.Errorf(".env pins %q, want %q", w.pinned(), want)
	}
}

// Bad: a target from another repository (or not a digest at all) is refused before anything is touched.
func TestWatcherRefusesAForeignTarget(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "update", "ghcr.io/someone-else/sepia@"+brokenDigest)
	if _, err := w.run(t, nil); err == nil {
		t.Fatal("a foreign image was accepted")
	}
	got := w.result(t)
	if got.Status != "failed" || !strings.Contains(got.Detail, "refused") {
		t.Errorf("result = %+v, want a refusal", got)
	}
	if calls := w.calls(t); strings.Contains(calls, "compose") {
		t.Errorf("docker ran for a refused target:\n%s", calls)
	}
	if w.pinned() != imageRepo+"@"+oldDigest {
		t.Error(".env moved for a refused target")
	}
	w.requestGone(t)
}

// Bad: a checkout that cannot fast-forward fails the update before the image is touched.
func TestWatcherUpdateWhenGitFails(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "update", imageRepo+"@"+newDigest)
	if _, err := w.run(t, map[string]string{"FAKE_GIT_FAIL": "1"}); err == nil {
		t.Fatal("a failed install exited zero")
	}
	got := w.result(t)
	if got.Status != "failed" || !strings.Contains(got.Detail, "checkout") {
		t.Errorf("result = %+v, want a checkout failure", got)
	}
	if calls := w.calls(t); strings.Contains(calls, "compose pull") {
		t.Errorf("the image was pulled after the checkout failed:\n%s", calls)
	}
	if w.pinned() != imageRepo+"@"+oldDigest {
		t.Error(".env moved after a failed checkout")
	}
	w.requestGone(t)
}

// Sad: the new container never passes its health check, so the previous image is put back and the result says so.
func TestWatcherUpdateRollsBackOnAFailedHealthCheck(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "update", imageRepo+"@"+brokenDigest)
	_, err := w.run(t, map[string]string{
		"FAKE_HEALTH":         "by-env",
		"FAKE_HEALTHY_DIGEST": oldDigest,
	})
	if err == nil {
		t.Fatal("a failed health check exited zero")
	}
	got := w.result(t)
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if !strings.Contains(got.Detail, "previous version is running again") {
		t.Errorf("detail = %q, want the rollback sentence", got.Detail)
	}
	if want := imageRepo + "@" + oldDigest; w.pinned() != want {
		t.Errorf(".env pins %q, want the previous image %q", w.pinned(), want)
	}
	if ups := strings.Count(w.calls(t), "compose up"); ups != 2 {
		t.Errorf("docker compose up ran %d times, want 2 (new, then rollback)", ups)
	}
	w.requestGone(t)
}

// Sad, and worse: the rollback fails too, so the result does not claim the old version came back.
func TestWatcherUpdateRollbackCanFailToo(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "update", imageRepo+"@"+brokenDigest)
	if _, err := w.run(t, map[string]string{"FAKE_HEALTH": "fail"}); err == nil {
		t.Fatal("a failed health check exited zero")
	}
	got := w.result(t)
	if got.Status != "failed" {
		t.Fatalf("status = %q, want failed", got.Status)
	}
	if !strings.Contains(got.Detail, "may be down") {
		t.Errorf("detail = %q, want the rollback-also-failed sentence", got.Detail)
	}
}

// Happy, and honest: asking for the image that already runs installs nothing.
func TestWatcherUpdateSkipsTheSameImage(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "update", imageRepo+"@"+oldDigest)
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := w.result(t); got.Status != "skipped" {
		t.Errorf("status = %q, want skipped", got.Status)
	}
	if calls := w.calls(t); strings.Contains(calls, "compose up") {
		t.Errorf("docker ran for an image that already runs:\n%s", calls)
	}
	w.requestGone(t)
}

// Happy: a restart recreates the container without pulling anything.
func TestWatcherRestartDoesNotPull(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "restart", "")
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if got := w.result(t); got.Status != "ok" {
		t.Fatalf("status = %q, detail = %q", got.Status, got.Detail)
	}
	calls := w.calls(t)
	if !strings.Contains(calls, "compose up") {
		t.Errorf("restart did not recreate the container:\n%s", calls)
	}
	if strings.Contains(calls, "compose pull") || strings.Contains(calls, "git pull") {
		t.Errorf("restart pulled something:\n%s", calls)
	}
}

// Bad: a request that cannot be read is refused and cleared rather than replayed forever.
func TestWatcherRefusesAMalformedRequest(t *testing.T) {
	w := newWatcher(t)
	w.request(t, "explode", "whatever")
	if _, err := w.run(t, nil); err == nil {
		t.Fatal("a malformed request exited zero")
	}
	if got := w.result(t); got.Status != "failed" {
		t.Errorf("status = %q, want failed", got.Status)
	}
	w.requestGone(t)
}

// The timer's backstop: no request means a check, so availability stays fresh without installing anything.
func TestWatcherTimerChecksWhenNothingIsQueued(t *testing.T) {
	w := newWatcher(t)
	if _, err := w.run(t, nil); err != nil {
		t.Fatalf("backstop: %v", err)
	}
	if got := w.available(t); got.LatestDigest == "" {
		t.Error("the backstop did not write available")
	}
	if calls := w.calls(t); strings.Contains(calls, "compose up") {
		t.Errorf("the backstop installed something:\n%s", calls)
	}
}

// fakes returns the scripted commands the watcher calls. Every one records into FAKE_LOG so a test can assert what
// ran, and reads its outcome from an environment variable so a failure is one line away.
func fakes() map[string]string {
	return map[string]string{
		"timeout": "#!/usr/bin/env bash\nshift\nexec \"$@\"\n",
		"flock":   "#!/usr/bin/env bash\nexit 0\n",
		"git": "#!/usr/bin/env bash\nprintf 'git %s\\n' \"$*\" >>\"$FAKE_LOG\"\n" +
			"if [ \"${FAKE_GIT_FAIL:-0}\" = 1 ]; then echo 'fatal: not possible to fast-forward' >&2; exit 1; fi\nexit 0\n",
		"docker": "#!/usr/bin/env bash\nprintf 'docker %s\\n' \"$*\" >>\"$FAKE_LOG\"\ncase \"$*\" in\n" +
			"  'compose pull'*) [ \"${FAKE_PULL_FAIL:-0}\" = 1 ] && { echo 'pull access denied' >&2; exit 1; }; exit 0 ;;\n" +
			"  'compose up'*) [ \"${FAKE_UP_FAIL:-0}\" = 1 ] && { echo 'port is already allocated' >&2; exit 1; }; exit 0 ;;\n" +
			"  'compose logs'*) echo 'sepia: config: unknown key'; exit 0 ;;\n" +
			"esac\nexit 0\n",
		"curl": `#!/usr/bin/env bash
printf 'curl %s\n' "$*" >>"$FAKE_LOG"
url="${!#}"
head=0
for arg in "$@"; do case "$arg" in -*I*) head=1 ;; esac; done
case "$url" in
  *"/token?"*)
    if [ "${FAKE_REGISTRY_FAIL:-0}" = 1 ]; then echo 'curl: (6) Could not resolve host: ghcr.io' >&2; exit 6; fi
    printf '{"token":"fake-token"}'
    ;;
  *"/manifests/latest"*)
    if [ "$head" = 1 ]; then
      printf 'HTTP/2 200\r\ndocker-content-digest: %s\r\n\r\n' "${FAKE_DIGEST:?}"
    else
      # GHCR writes pretty-printed JSON, with a space after the colon. The watcher has to read that shape.
      printf '{\n  "annotations": {\n    "org.opencontainers.image.version": "%s"\n  }\n}' "${FAKE_VERSION:-r219.e4f5a6b}"
    fi
    ;;
  *"/healthz"*)
    case "${FAKE_HEALTH:-ok}" in
      ok) printf '{"status":"ok","version":"r1"}' ;;
      by-env) if grep -q "${FAKE_HEALTHY_DIGEST:?}" .env; then printf '{"status":"ok"}'; else exit 22; fi ;;
      *) exit 22 ;;
    esac
    ;;
  *) exit 22 ;;
esac
`,
	}
}
