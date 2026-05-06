package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestListProfiles_ReturnsSeeded(t *testing.T) {
	h := apitestServer(t)
	p := seedProfile(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/profiles", nil)
	w := httptest.NewRecorder()
	h.ListProfiles(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var ps []state.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &ps); err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].ID != p.ID {
		t.Errorf("got %+v", ps)
	}
}

func TestGetProfile_OK(t *testing.T) {
	h := apitestServer(t)
	p := seedProfile(t, h)
	r := chi.NewRouter()
	r.Get("/api/profiles/{id}", h.GetProfile)

	req := httptest.NewRequest(http.MethodGet, "/api/profiles/"+p.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got state.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != p.ID {
		t.Errorf("got %s", got.ID)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Get("/api/profiles/{id}", h.GetProfile)

	req := httptest.NewRequest(http.MethodGet, "/api/profiles/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}
