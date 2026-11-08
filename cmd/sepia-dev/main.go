// Command sepia-dev runs the real service against the in-process world, with no network and no keys.
//
// It is the way to look at a screen or reproduce a bug without touching a provider: the server, the database and the
// agent are all the real ones, and only the outside services are fake. This command is never shipped
// (docs/BUILD.md 4).
//
// The script is written for one sentence, so the whole logging flow finishes without a key:
//
//	finished the lantern keeper last night, 4.5, the ending landed but it was too long
//
// Type that, or let the end-to-end suite type it, and a card appears (docs/slices/S09-server.md S09.5).
//
// SEPIA_DEV_SCENARIO=split swaps the script for one where the pre-read cannot tell the book from the film, so the
// flow asks the medium, the candidate, Describe and the questions before the card (S11's screen test).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/AutumnsGrove/sepia/internal/agent"
	"github.com/AutumnsGrove/sepia/internal/banks"
	"github.com/AutumnsGrove/sepia/internal/config"
	"github.com/AutumnsGrove/sepia/internal/decisions"
	"github.com/AutumnsGrove/sepia/internal/providers"
	"github.com/AutumnsGrove/sepia/internal/server"
	"github.com/AutumnsGrove/sepia/internal/session"
	"github.com/AutumnsGrove/sepia/internal/store"
	"github.com/AutumnsGrove/sepia/internal/version"
	"github.com/AutumnsGrove/sepia/internal/webui"
	"github.com/AutumnsGrove/sepia/internal/world"
)

// withoutDevKeys drops the switches that only this command knows, so the shipped config loader stays strict about
// the keys it does read.
func withoutDevKeys(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		if strings.HasPrefix(entry, "SEPIA_DEV_SCENARIO=") || strings.HasPrefix(entry, "SEPIA_DEV_FAULT=") {
			continue
		}
		out = append(out, entry)
	}
	return out
}

// faultOf reads the dev stack's fault switch: one outside service misbehaves, so a screen test can see the error
// and the way back. It is sticky, like every fault in the world, so a retry cannot quietly hide a bug.
func faultOf(environ []string) string {
	for _, entry := range environ {
		if value, ok := strings.CutPrefix(entry, "SEPIA_DEV_FAULT="); ok {
			return value
		}
	}
	return ""
}

// scenarioOf reads the dev stack's scenario switch. Anything but "split" is the plain sample, so a typo leaves the
// stack working rather than with no script at all.
func scenarioOf(environ []string) string {
	for _, entry := range environ {
		if value, ok := strings.CutPrefix(entry, "SEPIA_DEV_SCENARIO="); ok {
			return value
		}
	}
	return ""
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Environ(), os.Stderr); err != nil && !errors.Is(err, context.Canceled) {
		fmt.Fprintf(os.Stderr, "sepia-dev: %v\n", err)
		os.Exit(1)
	}
}

// run is main without the process, so a test can boot the whole dev stack in-process and stop it.
func run(ctx context.Context, environ []string, stderr io.Writer) error {
	cfg, err := config.Load(withoutDevKeys(environ))
	if err != nil {
		return err
	}
	// Development is explicitly on: the friendly error body and the dev origin are allowed, and the Host check is
	// not relaxed, because a bug in it must show up here rather than on the potato.
	cfg.Dev = true

	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	clock := world.RealClock{}

	scenario := scenarioOf(environ)

	chat := world.StartChat("openrouter", clock)
	defer chat.Close()
	if scenario == "split" {
		chat.Script(world.SampleSplitChatScript()...)
	} else {
		chat.Script(world.SampleChatScript()...)
	}

	tmdb := world.Start("tmdb", clock)
	defer tmdb.Close()
	tmdb.LoadBodies(world.SampleTMDB())

	ol := world.Start("openlibrary", clock)
	defer ol.Close()
	ol.LoadBodies(world.SampleOpenLibrary())

	reading := world.NewDecisions(clock)
	if scenario == "split" {
		world.ScriptSplitDecisions(reading)
	} else {
		world.ScriptSampleDecisions(reading)
	}

	if fault := faultOf(environ); fault != "" {
		// The chat fake answers whatever path the provider builds; the fault is set on both shapes so the switch
		// does not depend on that detail.
		switch fault {
		case "chat":
			chat.SetFault("/v1/chat/completions", world.FaultError)
			chat.SetFault("/chat/completions", world.FaultError)
		case "lookup":
			tmdb.SetFault("/search/movie", world.FaultError)
			tmdb.SetFault("/search/tv", world.FaultError)
			ol.SetFault("/search.json", world.FaultError)
		case "preread":
			reading.SetFault(world.DecisionError)
		}
		log.Info("dev fault", "service", fault)
	}

	log.Info("world ready", "chat", chat.URL(), "tmdb", tmdb.URL(), "openlibrary", ol.URL())

	db, err := store.Open(ctx, store.Options{Path: filepath.Join(cfg.DataDir, "sepia-dev.db")})
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err = db.VerifyTriggers(ctx); err != nil {
		return err
	}
	engine := session.New(session.Config{
		Store: db, Now: db.Now, NewID: db.NewID, ServerVersion: version.Version,
	})

	finder := providers.NewFinder(providers.Config{
		TMDBKey: "dev-key", TMDBBaseURL: tmdb.URL(), TMDBCoverBase: tmdb.URL() + "/t/p/w342",
		OpenLibraryBaseURL: ol.URL(), OpenLibraryCoverBase: ol.URL() + "/covers",
		// The far side is a fake in this process, so the provider pacing is off: one Open Library call a second
		// would serialise every parallel end-to-end test behind the last one.
		TMDBCallInterval: -1, OpenLibraryCallInterval: -1,
		DataDir: cfg.DataDir, Clock: clock, UserAgent: "sepia-dev (offline)",
	})

	// The shipped config is read from the checkout; a broken file is a logged warning, not a dead dev stack.
	set, err := banks.LoadDir(filepath.Join(cfg.ConfigDir, "questions"))
	if err != nil {
		log.Warn("question banks are not usable; the card will not ask questions", "err", err.Error())
		set = nil
	}
	registry, err := decisions.Load(filepath.Join(cfg.ConfigDir, "decisions"))
	if err != nil {
		log.Warn("decisions config is not usable; the pre-read is off", "err", err.Error())
		registry = nil
	}
	pre := decisions.New(decisions.Config{Registry: registry, Decider: reading})

	responder := server.NewResponder()
	live := server.NewLiveBus()
	a, err := agent.New(agent.Config{
		Store: db, Session: engine, Finder: finder, Banks: set, Decisions: pre,
		Model: cfg.Model, BaseURL: chat.URL(), APIKey: "dev", Responder: responder, Live: live.Publish,
		Logger: log, Now: db.Now, NewID: db.NewID,
	})
	if err != nil {
		return err
	}

	log.Info("dev stack up", "addr", cfg.Addr, "scenario", scenario, "note", "offline: every outside service is a fake")
	log.Info("dev sentence", "type", world.SampleMessage)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx, cfg, server.Deps{
			Engine: engine, Agent: a, Responder: responder, Live: live, Now: db.Now,
		}, log, webui.FS())
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		// Give the server's own drain a moment to finish before the world's listener is closed under it.
		select {
		case err := <-errCh:
			return err
		case <-time.After(15 * time.Second):
			return context.DeadlineExceeded
		}
	}
}
