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
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/bdmv"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/data"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/dreamcast"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/dvdvideo"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/ps2"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/psx"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/saturn"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/uhd"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/xbox"
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
	slog.Info("settings loaded",
		"addr", cfg.Addr,
		"library_movies", cfg.LibraryMovies,
		"library_tv", cfg.LibraryTV,
		"library_music", cfg.LibraryMusic,
		"library_games", cfg.LibraryGames,
		"library_data", cfg.LibraryData,
	)
	if cfg.Token != "" {
		slog.Info("bearer auth enabled")
	} else {
		slog.Info("auth disabled (LAN mode); set DISCECHO_TOKEN to enable bearer auth")
	}

	if n, err := drive.InitialScan(context.Background(), store); err != nil {
		slog.Warn("InitialScan", "err", err)
	} else {
		slog.Info("drives discovered", "count", n)
	}
	// Recover any drive left in `identifying` by a previous run. Without
	// this, ClaimDriveForIdentify (which only transitions from idle/error)
	// refuses every later uevent and the daemon stays deaf on that drive.
	if n, err := store.ResetIdentifyingDrives(context.Background()); err != nil {
		slog.Warn("ResetIdentifyingDrives", "err", err)
	} else if n > 0 {
		slog.Info("reset stuck identifying drives", "count", n)
	}

	// Tools — Whipper for ripping, Apprise for notifications, Eject
	// for the post-rip eject step.
	toolReg := tools.NewRegistry()
	toolReg.Register(tools.NewWhipper(cfg.WhipperBin))
	appriseTool := tools.NewApprise(cfg.AppriseBin)
	toolReg.Register(appriseTool)
	ejectTool := tools.NewEject(cfg.EjectBin)
	toolReg.Register(ejectTool)

	// Identify (TOC + MusicBrainz)
	tocReader := identify.NewCDParanoiaTOCReader(cfg.CDParanoiaBin)
	mbClient := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL:     cfg.MusicBrainzBaseURL,
		UserAgent:   cfg.MusicBrainzUserAgent,
		MinInterval: time.Second,
	})
	sysCNFProber := identify.NewSystemCNFProber(cfg.IsoInfoBin)
	classifier := identify.NewClassifier(identify.ClassifierConfig{
		CDInfoBin:       cfg.CDInfoBin,
		FSProber:        identify.NewFSProber(identify.FSProberConfig{IsoInfoBin: cfg.IsoInfoBin}),
		BDProber:        identify.NewBDProber(identify.BDProberConfig{BDInfoBin: cfg.BDInfoBin}),
		SystemCNFProber: sysCNFProber,
	})

	// urlsForTrigger is shared by every pipeline — looks up the
	// Apprise URLs configured for a given event trigger.
	urlsForTrigger := func(ctx context.Context, trigger string) []string {
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
	}

	// shouldEjectOnFinish is consulted by every pipeline at its rip-end
	// eject step. It reads operation.mode + rip.eject_on_finish at call
	// time so a settings change during a long rip takes effect on the
	// next job, not the next restart.
	shouldEjectOnFinish := func(ctx context.Context) bool {
		return pipelines.ShouldEjectOnFinish(ctx, store)
	}

	// Pipelines: register the audio-CD handler.
	pipeReg := pipelines.NewRegistry()
	pipeReg.Register(audiocd.New(audiocd.Deps{
		TOC:            tocReader,
		MB:             mbClient,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryMusic,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))

	// DVD-Video pipeline
	tmdbClient := identify.NewTMDBClient(identify.TMDBConfig{
		APIKey:   cfg.TMDBKey,
		Language: cfg.TMDBLang,
	})
	dvdProber := identify.NewDVDProber(identify.DVDProberConfig{IsoInfoBin: cfg.IsoInfoBin})
	handBrake := tools.NewHandBrake(cfg.HandBrakeBin)
	toolReg.Register(handBrake)
	nvencAvailable := tools.ProbeNVENC(cfg.HandBrakeBin)
	if nvencAvailable {
		slog.Info("NVENC detected; hardware transcoding available")
	} else {
		slog.Info("NVENC not detected; profiles requesting nvenc_* will fall back to software")
	}

	// MakeMKV is shared by BDMV and UHD (DVD uses dvdbackup since
	// v0.2.2 — see CHANGELOG; MakeMKV's rolling beta key plus the
	// upstream-expiration behaviour made it a poor fit for DVDs,
	// which libdvdcss handles fine).
	makeMKV := tools.NewMakeMKV(cfg.MakeMKVBin, cfg.MakeMKVDataDir)
	dvdBackup := tools.NewDVDBackup("")

	// DVD pipeline shares one root for movies and series. The
	// orchestrator can't yet differentiate at job time, so series land
	// under library.movies alongside films. Routing DVD-Series to
	// library.tv requires per-job profile lookup in the dispatcher —
	// tracked for branch 3.
	pipeReg.Register(dvdvideo.New(dvdvideo.Deps{
		Prober:           dvdProber,
		TMDB:             tmdbClient,
		DVDBackup:        dvdBackup,
		HandBrakeScanner: handBrake,
		Tools:            toolReg,
		LibraryRoot:      cfg.LibraryMovies,
		WorkRoot:         filepath.Join(cfg.DataPath, "work"),
		SubsLang:         cfg.SubsLang,
		URLsForTrigger:   urlsForTrigger,
		MetadataStore:    store,
		NVENCAvailable:   nvencAvailable,
		ShouldEject:      shouldEjectOnFinish,
	}))

	// BDMV + UHD pipelines (M3.1).

	pipeReg.Register(bdmv.New(bdmv.Deps{
		Prober:         dvdProber, // re-used for volume-label reading
		TMDB:           tmdbClient,
		MakeMKVScanner: makeMKV,
		MakeMKVRipper:  makeMKV,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryMovies,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		SubsLang:       cfg.SubsLang,
		URLsForTrigger: urlsForTrigger,
		NVENCAvailable: nvencAvailable,
		ShouldEject:    shouldEjectOnFinish,
	}))

	pipeReg.Register(uhd.New(uhd.Deps{
		Prober:         dvdProber,
		TMDB:           tmdbClient,
		MakeMKVScanner: makeMKV,
		MakeMKVRipper:  makeMKV,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryMovies,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		SubsLang:       cfg.SubsLang,
		AACS2KeyDB:     filepath.Join(cfg.MakeMKVDataDir, "KEYDB.cfg"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))

	// PSX + PS2 pipelines (M5.1).
	redumperTool := tools.NewRedumper(cfg.RedumperBin)
	chdmanTool := tools.NewCHDMan(cfg.CHDManBin)

	redumpDB, err := identify.LoadRedumpDir(cfg.RedumpDataDir)
	if err != nil {
		slog.Warn("redump dir not loaded", "dir", cfg.RedumpDataDir, "err", err)
		redumpDB = nil
	}

	pipeReg.Register(psx.New(psx.Deps{
		Redumper:       redumperTool,
		CHDMan:         chdmanTool,
		SystemCNF:      sysCNFProber,
		RedumpDB:       redumpDB,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryGames,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))
	pipeReg.Register(ps2.New(ps2.Deps{
		Redumper:       redumperTool,
		CHDMan:         chdmanTool,
		SystemCNF:      sysCNFProber,
		RedumpDB:       redumpDB,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryGames,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))
	pipeReg.Register(saturn.New(saturn.Deps{
		Redumper:       redumperTool,
		CHDMan:         chdmanTool,
		SaturnProber:   identify.NewDevSaturnProber(),
		RedumpDB:       redumpDB,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryGames,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))
	pipeReg.Register(dreamcast.New(dreamcast.Deps{
		Redumper:       redumperTool,
		CHDMan:         chdmanTool,
		RedumpDB:       redumpDB,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryGames,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))
	pipeReg.Register(xbox.New(xbox.Deps{
		Redumper:       redumperTool,
		XboxProber:     &xbox.IsoinfoXboxProber{Bin: cfg.IsoInfoBin},
		RedumpDB:       redumpDB,
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryGames,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
	}))
	pipeReg.Register(data.New(data.Deps{
		DD:             &tools.DD{Bin: cfg.DDBin},
		LabelProber:    &data.IsoinfoLabelProber{Bin: cfg.IsoInfoBin},
		Tools:          toolReg,
		LibraryRoot:    cfg.LibraryData,
		WorkRoot:       filepath.Join(cfg.DataPath, "work"),
		URLsForTrigger: urlsForTrigger,
		ShouldEject:    shouldEjectOnFinish,
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
		TMDB:         tmdbClient,
		MusicBrainz:  mbClient,
		Token:        cfg.Token,
		// ActiveSampler is started after the orchestrator's ctx is built (below).
		Apprise:        appriseTool,
		Settings:       cfg,
		NVENCAvailable: nvencAvailable,
		Ejector: func(ctx context.Context, devPath string) error {
			return ejectTool.Run(ctx, []string{devPath}, nil, "", tools.NopSink{})
		},
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

	// Active-jobs sampler maintains a 24-hour ring of active-job counts
	// in memory for the ACTIVE JOBS widget's delta + sparkline. Restart
	// loses history; acceptable for a dashboard widget.
	apiH.ActiveSampler = api.NewActiveJobsSampler(store)
	apiH.ActiveSampler.Start(ctx)

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

	sweeper := &state.Sweeper{
		Store:    store,
		Settings: store, // *Store satisfies SettingsReader via GetBool/GetInt
		Now:      time.Now,
		Logger:   slog.Default(),
	}
	sweeper.Start(ctx)

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
