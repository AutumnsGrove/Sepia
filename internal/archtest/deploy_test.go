package archtest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/AutumnsGrove/sepia/internal/config"
)

// The deployment is code too: a compose file that quietly publishes a port or mounts the Docker socket is a
// change nobody would notice in a Go diff. These checks are the ones docs/STANDARDS.md 6 and docs/BUILD.md 2
// promise, asserted against the file that actually ships.

func repoFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", name)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(body)
}

// composeServices parses docker-compose.yml into the map a check can look at.
func composeServices(t *testing.T) map[string]any {
	t.Helper()
	var doc struct {
		Services map[string]any `yaml:"services"`
	}
	if err := yaml.Unmarshal([]byte(repoFile(t, "docker-compose.yml")), &doc); err != nil {
		t.Fatalf("docker-compose.yml is not YAML: %v", err)
	}
	if len(doc.Services) == 0 {
		t.Fatal("docker-compose.yml has no services")
	}
	return doc.Services
}

func service(t *testing.T) map[string]any {
	t.Helper()
	svc, ok := composeServices(t)["sepia"]
	if !ok {
		t.Fatal("docker-compose.yml has no sepia service")
	}
	m, ok := svc.(map[string]any)
	if !ok {
		t.Fatal("the sepia service is not a mapping")
	}
	return m
}

func TestComposeIsHardened(t *testing.T) {
	svc := service(t)

	// Host networking is the S13 decision: the container's loopback is the potato's, so config.Load's loopback
	// rule stays literally true and the watcher reaches the same 127.0.0.1:8893 as every other app.
	if got := svc["network_mode"]; got != "host" {
		t.Errorf("network_mode = %v, want host", got)
	}
	// Bad: a published port alongside host networking is ignored by Docker and hides the fact that the host
	// boundary is the only one. One or the other, never both.
	if _, ok := svc["ports"]; ok {
		t.Error("the sepia service publishes ports; host networking already uses the host's loopback")
	}
	// Sad: a writable root filesystem turns a bug in the service into a change on the board.
	if got := svc["read_only"]; got != true {
		t.Errorf("read_only = %v, want true", got)
	}
	if caps, ok := svc["cap_drop"].([]any); !ok || !slices.Contains(caps, any("ALL")) {
		t.Errorf("cap_drop = %v, want [ALL]", svc["cap_drop"])
	}
	opts, _ := svc["security_opt"].([]any)
	if !slices.Contains(opts, any("no-new-privileges:true")) {
		t.Errorf("security_opt = %v, want no-new-privileges:true", svc["security_opt"])
	}
	// The container never touches Docker (docs/UPDATE.md). A socket mount here would undo the whole design.
	for _, vol := range mounts(t, svc) {
		if strings.Contains(vol, "docker.sock") {
			t.Errorf("volume %q mounts the Docker socket into the container", vol)
		}
	}
	// The image and the ids are pinned in .env, which is also what the watcher rewrites on an update.
	image, _ := svc["image"].(string)
	if !strings.Contains(image, "${SEPIA_IMAGE") {
		t.Errorf("image = %q, want ${SEPIA_IMAGE...} from .env", image)
	}
	user, _ := svc["user"].(string)
	if !strings.Contains(user, "SEPIA_UID") || !strings.Contains(user, "SEPIA_GID") {
		t.Errorf("user = %q, want the uid and gid .env carries", user)
	}
}

// mounts returns the service's volume lines as strings. Both the short string form and the long mapping form are
// accepted, because compose allows either and a check that misses one is not a check.
func mounts(t *testing.T, svc map[string]any) []string {
	t.Helper()
	raw, ok := svc["volumes"].([]any)
	if !ok {
		t.Fatal("the sepia service mounts nothing; the data directory and the signal directory must persist")
	}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		switch v := entry.(type) {
		case string:
			out = append(out, v)
		case map[string]any:
			source, _ := v["source"].(string)
			target, _ := v["target"].(string)
			readOnly, _ := v["read_only"].(bool)
			line := source + ":" + target
			if readOnly {
				line += ":ro"
			}
			out = append(out, line)
		default:
			t.Fatalf("volume entry %v is neither a string nor a mapping", entry)
		}
	}
	return out
}

func TestComposeMountsWhatTheAppNeeds(t *testing.T) {
	svc := service(t)
	got := mounts(t, svc)

	// Happy: the tree, the config and the signal directory are the three paths the service actually uses.
	for _, want := range []string{"./data:/data", "./config:/config:ro", ":/update-signal"} {
		found := slices.ContainsFunc(got, func(v string) bool { return strings.Contains(v, want) })
		if !found {
			t.Errorf("volumes %v are missing %q", got, want)
		}
	}

	// The config tree is read-only: it is shipped in the checkout and the app never writes to it.
	for _, v := range got {
		if strings.Contains(v, ":/config") && !strings.HasSuffix(v, ":ro") {
			t.Errorf("volume %q mounts config writable; it is a shipped tree", v)
		}
	}
}

// TestComposeEnvironmentIsKnownConfig feeds every SEPIA_ key the compose file passes to config.Load. A typo in
// the passthrough list would otherwise be a container that starts, ignores one setting, and reports healthy.
func TestComposeEnvironmentIsKnownConfig(t *testing.T) {
	svc := service(t)
	env, ok := svc["environment"].(map[string]any)
	if !ok || len(env) == 0 {
		t.Fatal("the sepia service passes no environment; the container cannot find its data directory")
	}
	var environ []string
	for key, value := range env {
		s := ""
		if text, ok := value.(string); ok {
			s = text
			// A compose placeholder is not a value config.Load can judge, and the defaults are what it
			// resolves to.
			if strings.Contains(text, "${") {
				s = ""
			}
		}
		environ = append(environ, key+"="+s)
	}
	if _, err := config.Load(environ); err != nil {
		t.Fatalf("the compose environment is not one config.Load accepts: %v", err)
	}
}

func TestImageRunsUnprivileged(t *testing.T) {
	dockerfile := repoFile(t, "Dockerfile")

	// Happy: the runtime stage is distroless and declares a non-root user, so a compose override is a pin rather
	// than the only thing standing between the service and root.
	if !strings.Contains(dockerfile, "gcr.io/distroless/static-debian12:nonroot") {
		t.Error("the final stage is not distroless static nonroot")
	}
	if !strings.Contains(dockerfile, "USER nonroot:nonroot") {
		t.Error("the final stage does not declare USER nonroot:nonroot")
	}
	// Bad: a shell in the image is a shell to keep patched, and Sepia needs none.
	if strings.Contains(dockerfile, "apk add") || strings.Contains(dockerfile, "apt-get") {
		t.Error("the image installs packages at runtime; the binary needs nothing")
	}
	// The frontend and Go stages build for the runner, not the target, so a multi-arch publish compiles twice.
	if strings.Count(dockerfile, "FROM --platform=$BUILDPLATFORM") != 2 {
		t.Error("the web and build stages must be pinned to $BUILDPLATFORM")
	}
}

// TestSignalFilesAreNeverCommitted: signal files carry a timestamp and an image digest, and a stale one in the
// repository would be picked up by a fresh checkout.
func TestSignalFilesAreNeverCommitted(t *testing.T) {
	ignore := repoFile(t, ".gitignore")
	for _, want := range []string{"update-signal/", ".env"} {
		if !slices.ContainsFunc(strings.Split(ignore, "\n"), func(line string) bool {
			// A leading slash anchors the path to the repository root and is still the same ignore rule.
			return strings.TrimPrefix(strings.TrimSpace(line), "/") == want
		}) {
			t.Errorf(".gitignore does not ignore %q", want)
		}
	}
}
