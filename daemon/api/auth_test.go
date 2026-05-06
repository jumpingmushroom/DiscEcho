package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
)

func TestAuth_AcceptsValidToken(t *testing.T) {
	h := &api.Handlers{Token: "secret"}
	mux := http.NewServeMux()
	mux.Handle("/api/protected", h.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})))

	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer secret")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("got %d", w.Code)
	}
}

func TestAuth_RejectsBadToken(t *testing.T) {
	h := &api.Handlers{Token: "secret"}
	handler := h.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, hdr := range []string{"", "Bearer wrong", "Basic xyz", "secret"} {
		req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		if hdr != "" {
			req.Header.Set("Authorization", hdr)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("hdr=%q: want 401, got %d", hdr, w.Code)
		}
	}
}

func TestAuth_EmptyTokenAllowsAll(t *testing.T) {
	// When Token is empty (development), middleware is a passthrough.
	h := &api.Handlers{Token: ""}
	handler := h.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("empty token should allow, got %d", w.Code)
	}
}
