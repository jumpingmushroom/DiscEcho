package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter returns the top-level HTTP handler.
// Static asset serving is wired in by the embed package; we expose a
// Mount point so this file stays free of webui-build coupling.
func NewRouter(static http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Route("/api", func(api chi.Router) {
		api.Get("/health", HealthHandler)
		api.Get("/version", VersionHandler)
	})

	if static != nil {
		r.Handle("/*", static)
	}
	return r
}
