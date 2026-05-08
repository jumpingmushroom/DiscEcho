package api

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// Authenticate returns middleware that requires Authorization: Bearer
// <Token>. If Token is empty, the middleware is a passthrough — useful
// during development before the token is bootstrapped.
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
