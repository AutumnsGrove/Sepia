package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/store"
	"github.com/AutumnsGrove/sepia/internal/update"
	"github.com/AutumnsGrove/sepia/internal/version"
)

// The doctor is the post-install check, run inside the container:
//
//	docker compose exec sepia /sepia doctor
//
// It answers four questions that a person cannot answer by looking at a green container: is the bound address still
// loopback, can the process write its data directory, is the mounted .env private to its owner, and do the
// database's protections still refuse what they must. It reports keys by presence and never prints a value.
const defaultEnvFile = "/run/sepia/env"

// maxEnvFile bounds the .env read. It is a handful of lines the operator wrote; anything larger is a mistake.
const maxEnvFile = 64 << 10

// doctorEnvKeys are the lines the deployment needs. The two provider keys may be empty: a missing key is a degraded
// feature, not a broken install.
var doctorEnvKeys = []string{"SEPIA_IMAGE", "SEPIA_UID", "SEPIA_GID", "SEPIA_TMDB_API_KEY", "SEPIA_OPENROUTER_API_KEY"}

// doctorArgs reads --env-file. It is a flag rather than a config key because it is a path to a secret, not a setting
// the service behaves differently for.
func doctorArgs(args []string) (string, error) {
	path := defaultEnvFile
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--env-file" && i+1 < len(args):
			i++
			path = args[i]
		case strings.HasPrefix(args[i], "--env-file="):
			path = strings.TrimPrefix(args[i], "--env-file=")
		default:
			return "", fmt.Errorf("unknown argument %q", args[i])
		}
	}
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("--env-file is empty")
	}
	return path, nil
}

// runDoctor prints one line per check and returns the exit code: 1 when something is broken, 0 with warnings when
// something is merely not set up.
func runDoctor(ctx context.Context, cfg config.Config, envFile string, stdout io.Writer) int {
	fmt.Fprintln(stdout, "sepia doctor")
	failed := false
	fail := func(format string, args ...any) {
		failed = true
		fmt.Fprintf(stdout, "  fail  %s\n", fmt.Sprintf(format, args...))
	}
	warn := func(format string, args ...any) {
		fmt.Fprintf(stdout, "  warn  %s\n", fmt.Sprintf(format, args...))
	}
	ok := func(format string, args ...any) {
		fmt.Fprintf(stdout, "  ok    %s\n", fmt.Sprintf(format, args...))
	}

	// The listener. config.Load already refuses anything but loopback; this line is the proof the running container
	// got that far, and it fails loudly if a future path around the loader appears.
	if host, _, err := net.SplitHostPort(cfg.Addr); err != nil {
		fail("address %q is not host:port: %v", cfg.Addr, err)
	} else if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
		fail("address %q is not loopback; Sepia is loopback only", cfg.Addr)
	} else if ip == nil && !strings.EqualFold(host, "localhost") {
		fail("address %q is not a loopback address", cfg.Addr)
	} else {
		ok("listener: %s (loopback)", cfg.Addr)
	}

	dataOK := true
	if err := probeWritable(cfg.DataDir); err != nil {
		dataOK = false
		fail("data: %s is not writable: %v", cfg.DataDir, err)
	} else {
		ok("data: %s is writable", cfg.DataDir)
	}

	checkEnvFile(envFile, ok, warn, fail)

	if dataOK {
		checkStore(ctx, cfg, ok, fail)
	}

	signal := update.New(cfg.UpdateDir, version.Version, time.Now)
	updater := signal.Updater()
	switch updater.State {
	case update.StateReady:
		ok("updater: ready (heartbeat %s ago)", roundAge(time.Since(*updater.HeartbeatAt)))
	case update.StateStale:
		warn("updater: gone quiet (last heartbeat %s ago); journalctl -u sepia-update.service", roundAge(time.Since(*updater.HeartbeatAt)))
	default:
		warn("updater: not installed; this is a bare deployment, or ../compose/systemd was not installed")
	}

	if failed {
		fmt.Fprintln(stdout, "doctor: something is wrong. Fix the fail lines above.")
		return 1
	}
	fmt.Fprintln(stdout, "doctor: all checks passed.")
	return 0
}

// checkStore proves the database protects what it must, both by presence (Doctor) and by trying the forbidden
// statements (VerifyTriggers). Presence alone can be satisfied by a trigger that does nothing.
func checkStore(ctx context.Context, cfg config.Config, ok func(string, ...any), fail func(string, ...any)) {
	path := filepath.Join(cfg.DataDir, "sepia.db")
	db, err := store.Open(ctx, store.Options{Path: path})
	if err != nil {
		fail("database: could not be opened: %v", err)
		return
	}
	defer func() { _ = db.Close() }()

	health, err := db.Doctor(ctx)
	if err != nil {
		fail("database: %v", err)
		return
	}
	if !health.OK() {
		if health.Integrity != "ok" {
			fail("database: integrity check says %q", health.Integrity)
		}
		if len(health.MissingTriggers) > 0 {
			fail("database: missing protections: %s", strings.Join(health.MissingTriggers, ", "))
		}
		if !health.PermissionsOK {
			fail("database: %s is readable by others (%s)", path, health.Permissions)
		}
		return
	}
	ok("database: schema %d, permissions %s", health.Version, health.Permissions)

	if err := db.VerifyTriggers(ctx); err != nil {
		fail("protections: a forbidden write was accepted: %v", err)
		return
	}
	ok("protections: verified")
}

// checkEnvFile reports the .env's presence, mode, owner and keys. A missing file is a warning, because a bare
// developer run has none; a file readable by others is a failure, because that is the one thing this check is for.
func checkEnvFile(path string, ok, warn, fail func(string, ...any)) {
	// The path is the operator's own flag and this is the tool that checks it; there is no untrusted caller here.
	info, err := os.Stat(path) //nolint:gosec // G703: the path is the doctor's --env-file flag
	if os.IsNotExist(err) {
		warn("env: no file at %s (a bare run keeps its keys in the environment)", path)
		return
	}
	if err != nil {
		fail("env: %s: %v", path, err)
		return
	}

	mode := info.Mode().Perm()
	if st, isStat := info.Sys().(*syscall.Stat_t); isStat && int(st.Uid) != os.Geteuid() {
		fail("env: %s is owned by %d, but this process runs as %d", path, st.Uid, os.Geteuid())
		return
	}
	if mode&0o077 != 0 {
		fail("env: %s mode %04o is readable by group or other; it must be 0600", path, mode)
		return
	}
	if st, isStat := info.Sys().(*syscall.Stat_t); isStat {
		ok("env: %s mode %04o, owner %d:%d", path, mode, st.Uid, st.Gid)
	} else {
		ok("env: %s mode %04o", path, mode)
	}

	present := envKeys(path)
	for _, key := range doctorEnvKeys {
		value, listed := present[key]
		switch {
		case !listed:
			warn("%s is missing from the file", key)
		case !value:
			warn("%s is empty", key)
		default:
			ok("%s: present", key)
		}
	}
}

// envKeys returns the keys whose value is non-empty. It never returns or logs a value.
func envKeys(path string) map[string]bool {
	out := map[string]bool{}   // key to "has a value"; values are never kept
	file, err := os.Open(path) //nolint:gosec // G703: the path is the doctor's --env-file flag
	if err != nil {
		return out
	}
	defer func() { _ = file.Close() }()
	body, err := io.ReadAll(io.LimitReader(file, maxEnvFile+1))
	if err != nil || len(body) > maxEnvFile {
		return out
	}
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		out[key] = strings.TrimSpace(value) != ""
	}
	return out
}

// probeWritable creates and removes a file in the data directory, which is the only honest test of a mount's
// permissions.
func probeWritable(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return err
	}
	name := file.Name()
	_ = file.Close()
	return os.Remove(name)
}

// roundAge keeps an age readable without a duration's unnecessary nanoseconds.
func roundAge(d time.Duration) time.Duration {
	if d < 0 {
		d = 0
	}
	return d.Round(time.Second)
}
