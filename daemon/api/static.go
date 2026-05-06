package api

import (
	"io/fs"
	"net/http"
	"strings"
)

// StaticHandler serves files from the embedded SvelteKit build with an
// SPA fallback to /index.html for unknown paths (so client-side routes
// resolve correctly on hard refresh).
func StaticHandler(static fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API routes are handled before this; this branch shouldn't see
		// them, but guard anyway so a misconfigured router doesn't 404
		// API calls into the static fallback.
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			r.URL.Path = "/"
			fileServer.ServeHTTP(w, r)
			return
		}
		if _, err := fs.Stat(static, path); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback.
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	})
}
