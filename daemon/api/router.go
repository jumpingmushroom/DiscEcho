package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewRouter returns the top-level HTTP handler. h supplies endpoint
// dependencies and the bearer token; static is the embedded SvelteKit
// build (nil during tests). Public endpoints (/api/health, /api/version)
// stay unauthenticated; everything else lives behind h.Authenticate.
func NewRouter(h *Handlers, static http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// 30s request timeout for everything except /api/events. SSE streams
	// are long-lived; chi's middleware.Timeout wraps the response in
	// http.TimeoutHandler which closes the connection at the deadline.
	withTimeout := middleware.Timeout(30 * time.Second)

	r.Route("/api", func(api chi.Router) {
		// Unauthenticated
		api.With(withTimeout).Get("/health", HealthHandler)
		api.With(withTimeout).Get("/version", VersionHandler)

		// Authenticated SSE — no timeout middleware (long-lived stream).
		api.With(h.Authenticate).Get("/events", h.StreamEvents)

		// Authenticated subset (with request timeout).
		api.Group(func(authed chi.Router) {
			authed.Use(h.Authenticate)
			authed.Use(withTimeout)

			authed.Get("/state", h.GetState)
			authed.Get("/stats", h.Stats)

			authed.Get("/drives", h.ListDrives)
			authed.Get("/drives/{id}", h.GetDrive)
			authed.Post("/drives/{id}/eject", h.EjectDrive)

			authed.Get("/jobs", h.ListJobs)
			authed.Get("/jobs/{id}", h.GetJob)
			authed.Delete("/jobs/{id}", h.DeleteJob)
			authed.Get("/jobs/{id}/logs", h.ListJobLogs)
			authed.Post("/jobs/{id}/cancel", h.CancelJob)

			authed.Post("/discs/{id}/identify", h.IdentifyDisc)
			authed.Post("/discs/{id}/start", h.StartDisc)
			authed.Delete("/discs/{id}", h.DeleteDisc)

			authed.Get("/profiles", h.ListProfiles)
			authed.Get("/profiles/{id}", h.GetProfile)
			authed.Post("/profiles", h.CreateProfile)
			authed.Put("/profiles/{id}", h.UpdateProfile)
			authed.Delete("/profiles/{id}", h.DeleteProfile)

			authed.Get("/history", h.ListHistory)
			authed.Post("/history/clear", h.ClearHistory)

			authed.Get("/notifications", h.ListNotifications)
			authed.Post("/notifications", h.CreateNotification)
			authed.Put("/notifications/{id}", h.UpdateNotification)
			authed.Delete("/notifications/{id}", h.DeleteNotification)
			authed.Post("/notifications/{id}/validate", h.ValidateNotification)
			authed.Post("/notifications/{id}/test", h.TestNotification)

			authed.Get("/settings", h.GetSettings)
			authed.Put("/settings", h.PutSettings)

			authed.Get("/system/host", h.GetSystemHost)
			authed.Get("/system/integrations", h.GetSystemIntegrations)
		})
	})

	if static != nil {
		r.Handle("/*", static)
	}
	return r
}
