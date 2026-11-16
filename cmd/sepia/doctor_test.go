package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The doctor is the post-install check: it reads the mounted .env for presence, mode and owner, proves the database's
// protections still work, and says whether the updater is alive. It must never print a value from .env, because its
// output goes in a journal and a terminal history.

func doctorEnv(t *testing.T) (environ []string, root, envFile string) {
	t.Helper()
	root = t.TempDir()
	envFile = filepath.Join(root, "env")
	environ = []string{
		"SEPIA_DATA_DIR=" + filepath.Join(root, "data"),
		"SEPIA_CONFIG_DIR=" + filepath.Join(root, "config"),
		"SEPIA_UPDATE_DIR=" + filepath.Join(root, "signal"),
	}
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "signal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "signal", "heartbeat"),
		[]byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		t.Fatal(err)
	}
	return environ, root, envFile
}

func writeEnvFile(t *testing.T, path, mode string) {
	t.Helper()
	body := "SEPIA_IMAGE=ghcr.io/autumnsgrove/sepia@sha256:" + strings.Repeat("a", 64) + "\n" +
		"SEPIA_UID=1000\nSEPIA_GID=1000\n" +
		"SEPIA_TMDB_API_KEY=\n" +
		"SEPIA_OPENROUTER_API_KEY=sk-or-secret-value\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	var perm os.FileMode
	switch mode {
	case "0644":
		perm = 0o644
	default:
		perm = 0o600
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

func TestDoctorHealthy(t *testing.T) {
	environ, _, envFile := doctorEnv(t)
	writeEnvFile(t, envFile, "0600")
	var out, errOut bytes.Buffer

	code := run(context.Background(), []string{"doctor", "--env-file", envFile}, environ, &out, &errOut)
	if code != 0 {
		t.Fatalf("doctor exit %d\n%s\n%s", code, out.String(), errOut.String())
	}
	for _, want := range []string{"database: schema", "protections: verified", "mode 0600", "updater: ready"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output is missing %q:\n%s", want, out.String())
		}
	}
	// The key is reported by name and presence, never by value.
	if strings.Contains(out.String(), "sk-or-secret-value") {
		t.Fatal("the doctor printed a secret")
	}
	if !strings.Contains(out.String(), "SEPIA_OPENROUTER_API_KEY: present") {
		t.Errorf("output does not report the key's presence:\n%s", out.String())
	}
}

// Bad: a .env anyone in the group can read is the one thing the doctor is meant to catch.
func TestDoctorRefusesAnOpenEnvFile(t *testing.T) {
	environ, _, envFile := doctorEnv(t)
	writeEnvFile(t, envFile, "0644")
	var out bytes.Buffer

	code := run(context.Background(), []string{"doctor", "--env-file", envFile}, environ, &out, &out)
	if code == 0 {
		t.Fatal("doctor passed with a 0644 .env")
	}
	if !strings.Contains(out.String(), "mode 0644") {
		t.Errorf("output does not name the mode:\n%s", out.String())
	}
	if strings.Contains(out.String(), "sk-or-secret-value") {
		t.Fatal("the doctor printed a secret")
	}
}

// Sad: a data directory the process cannot write to means no cover, no database and no review.
func TestDoctorRefusesAReadOnlyDataDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so a read-only directory proves nothing")
	}
	environ, root, envFile := doctorEnv(t)
	writeEnvFile(t, envFile, "0600")
	data := filepath.Join(root, "data")
	if err := os.MkdirAll(data, 0o555); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(data, 0o555); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run(context.Background(), []string{"doctor", "--env-file", envFile}, environ, &out, &out)
	if code == 0 {
		t.Fatal("doctor passed with a read-only data directory")
	}
	if !strings.Contains(out.String(), "data:") {
		t.Errorf("output does not mention the data directory:\n%s", out.String())
	}
}

// Happy and honest: a bare run has no updater, and that is a warning rather than a failure.
func TestDoctorWithoutAnUpdater(t *testing.T) {
	environ, root, envFile := doctorEnv(t)
	writeEnvFile(t, envFile, "0600")
	environ[2] = "SEPIA_UPDATE_DIR=" // an empty value is refused by config.Load, so drop the entry instead
	environ = environ[:2]
	if err := os.RemoveAll(filepath.Join(root, "signal")); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := run(context.Background(), []string{"doctor", "--env-file", envFile}, environ, &out, &out)
	if code != 0 {
		t.Fatalf("doctor exit %d with no updater\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "updater: not installed") {
		t.Errorf("output does not say the updater is missing:\n%s", out.String())
	}
}
