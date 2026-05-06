// Command discecho is the DiscEcho daemon entrypoint. It opens the
// SQLite store, loads settings, wires the tool/pipeline registries and
// the orchestrator, then serves the HTTP API while listening for udev
// optical-media-change events.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/drive"
	"github.com/jumpingmushroom/DiscEcho/daemon/embed"
	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/jobs"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/audiocd"
	"github.com/jumpingmushroom/DiscEcho/daemon/settings"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
	"github.com/jumpingmushroom/DiscEcho/daemon/version"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	dataPath := firstEnv("DISCECHO_DATA", "/var/lib/discecho")
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		slog.Error("mkdir data", "err", err)
		os.Exit(1)
	}
	dbPath := filepath.Join(dataPath, "discecho.sqlite")
	db, err := state.Open(dbPath)
	if err != nil {
		slog.Error("state.Open", "err", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	store := state.NewStore(db)
	bc := state.NewBroadcaster()
	defer bc.Close()

	cfg, err := settings.Load(os.Getenv, store, version.Info().Version)
	if err != nil {
		slog.Error("settings.Load", "err", err)
		os.Exit(1)
	}
	slog.Info("settings loaded", "addr", cfg.Addr, "library", cfg.LibraryPath)
	slog.Info("bearer token", "token", maskToken(cfg.Token))

	if n, err := drive.InitialScan(context.Background(), store); err != nil {
		slog.Warn("InitialScan", "err", err)
	} else {
		slog.Info("drives discovered", "count", n)
	}

	// Tools — Whipper for ripping, Apprise for notifications, Eject
	// for the post-rip eject step.
	toolReg := tools.NewRegistry()
	toolReg.Register(tools.NewWhipper(cfg.WhipperBin))
	toolReg.Register(tools.NewApprise(cfg.AppriseBin))
	toolReg.Register(tools.NewEject(cfg.EjectBin))

	// Identify (TOC + MusicBrainz)
	tocReader := identify.NewCDParanoiaTOCReader(cfg.CDParanoiaBin)
	mbClient := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL:     cfg.MusicBrainzBaseURL,
		UserAgent:   cfg.MusicBrainzUserAgent,
		MinInterval: time.Second,
	})
	classifier := identify.NewClassifier(identify.ClassifierConfig{CDInfoBin: cfg.CDInfoBin})

	// Pipelines: register the audio-CD handler.
	pipeReg := pipelines.NewRegistry()
	pipeReg.Register(audiocd.New(audiocd.Deps{
		TOC:         tocReader,
		MB:          mbClient,
		Tools:       toolReg,
		LibraryRoot: cfg.LibraryPath,
		WorkRoot:    filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: func(ctx context.Context, trigger string) []string {
			ns, err := store.ListNotificationsForTrigger(ctx, trigger)
			if err != nil {
				slog.Warn("notifications lookup", "trigger", trigger, "err", err)
				return nil
			}
			urls := make([]string, 0, len(ns))
			for _, n := range ns {
				urls = append(urls, n.URL)
			}
			return urls
		},
	}))

	// Orchestrator drives jobs through the pipeline.
	orch := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store:       store,
		Broadcaster: bc,
		Pipelines:   pipeReg,
	})
	defer orch.Close()

	// HTTP API.
	apiH := &api.Handlers{
		Store:        store,
		Broadcaster:  bc,
		Orchestrator: orch,
		Pipelines:    pipeReg,
		Classifier:   classifier,
		Token:        cfg.Token,
	}

	embedFS, err := embed.FS()
	if err != nil {
		slog.Error("embed FS", "err", err)
		os.Exit(1)
	}
	staticHandler := api.StaticHandler(embedFS)

	router := api.NewRouter(apiH, staticHandler)
	server := api.NewServer(cfg.Addr, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// disc-flow: listen for udev optical-media-change events and run
	// classify → identify → persist.
	df := &discFlow{
		store:       store,
		bc:          bc,
		classifier:  classifier,
		pipelines:   pipeReg,
		identifyDur: 30 * time.Second,
	}
	go func() {
		if err := drive.Watch(ctx, df.handle); err != nil {
			slog.Error("udev watcher exited", "err", err)
		}
	}()

	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		slog.Info("shutdown requested")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "err", err)
			os.Exit(1)
		}
	}
}

func firstEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func maskToken(t string) string {
	if len(t) <= 8 {
		return "***"
	}
	return t[:4] + "…" + t[len(t)-4:]
}
