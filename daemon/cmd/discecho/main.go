package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/drive"
	"github.com/jumpingmushroom/DiscEcho/daemon/embed"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	addr := os.Getenv("DISCECHO_ADDR")
	if addr == "" {
		addr = ":8088"
	}

	// Static handler comes from the embedded SvelteKit build.
	embedFS, err := embed.FS()
	if err != nil {
		slog.Error("embed FS", "err", err)
		os.Exit(1)
	}
	static := api.StaticHandler(embedFS)
	router := api.NewRouter(static)
	server := api.NewServer(addr, router)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		err := drive.Watch(ctx, func(ev drive.Uevent) {
			slog.Info("disc media change",
				"dev", ev.DevName,
				"action", ev.Action,
				"path", ev.DevPath,
			)
		})
		if err != nil {
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
