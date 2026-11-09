package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// The dev stack is the only place the real server and the world meet before CI, so it is booted here once: if
// this test passes, `go run ./cmd/sepia-dev` gives a working offline app.

func freeAddr(t *testing.T) string {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func TestDevStackBootsAndStops(t *testing.T) {
	addr := freeAddr(t)
	var stderr bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	environ := []string{
		"SEPIA_ADDR=" + addr, "SEPIA_ALLOWED_HOSTS=" + addr,
		"SEPIA_DATA_DIR=" + t.TempDir(), "SEPIA_CONFIG_DIR=../../config",
	}
	go func() { done <- run(ctx, environ, &stderr) }()

	// Wait for the port rather than sleeping a guessed amount.
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		reqCtx, reqCancel := context.WithTimeout(context.Background(), time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, "http://"+addr+"/healthz", nil)
		if err == nil {
			client := &http.Client{Timeout: time.Second}
			if resp, err := client.Do(req); err == nil {
				_ = resp.Body.Close()
				reqCancel()
				if resp.StatusCode == http.StatusOK {
					lastErr = nil
					break
				}
				lastErr = err
			} else {
				lastErr = err
			}
		}
		reqCancel()
		time.Sleep(20 * time.Millisecond)
	}
	if lastErr != nil {
		cancel()
		t.Fatalf("dev stack never answered: %v; log %s", lastErr, stderr.String())
	}

	if !strings.Contains(stderr.String(), "world ready") {
		t.Errorf("the world was not announced: %s", stderr.String())
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the dev stack did not stop")
	}
}

func TestDevStackRefusesBadConfig(t *testing.T) {
	var stderr bytes.Buffer
	err := run(context.Background(), []string{"SEPIA_ADDR=0.0.0.0:8893"}, &stderr)
	if err == nil {
		t.Fatal("run accepted a public address")
	}
	if !strings.Contains(err.Error(), "loopback") {
		t.Errorf("error = %v, want it to explain the loopback rule", err)
	}
}

// The seed switch is wired to the boot, not just to a function: the stack comes up with an invented library the REST
// routes can read, which is what every reading-screen test stands on.
func TestDevStackSeedsTheInventedLibrary(t *testing.T) {
	addr := freeAddr(t)
	var stderr bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{
			"SEPIA_ADDR=" + addr, "SEPIA_ALLOWED_HOSTS=" + addr,
			"SEPIA_DATA_DIR=" + t.TempDir(), "SEPIA_CONFIG_DIR=../../config",
			"SEPIA_DEV_SEED=library",
		}, &stderr)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("the dev stack did not stop")
		}
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(10 * time.Second)
	var body []byte
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/entries", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = addr
		resp, err := client.Do(req)
		if err == nil {
			raw := make([]byte, 0)
			buf := bytes.NewBuffer(raw)
			_, _ = buf.ReadFrom(resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body = buf.Bytes()
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	if body == nil {
		t.Fatalf("the seeded library never answered; log %s", stderr.String())
	}
	if !strings.Contains(string(body), "The Lantern Keeper") || !strings.Contains(string(body), "Bramble Hollow") {
		t.Fatalf("the library = %s, want the invented entries", body)
	}
	if !strings.Contains(stderr.String(), "invented reviews") {
		t.Errorf("the seed was not announced: %s", stderr.String())
	}
}
