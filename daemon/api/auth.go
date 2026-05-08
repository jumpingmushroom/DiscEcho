package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Authenticate returns middleware that requires Authorization: Bearer
// <Token> when Token is non-empty. An empty Token is the documented
// LAN-only default (set DISCECHO_TOKEN to opt into bearer auth for
// proxy/exposed deployments).
func (h *Handlers) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.Token == "" {
			next.ServeHTTP(w, r)
			return
		}
		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
		// Constant-time compare to avoid leaking token length / prefix
		// match progress through response timing.
		if subtle.ConstantTimeCompare([]byte(tok), []byte(h.Token)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
