package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestListDrives_ReturnsSeeded(t *testing.T) {
	h := apitestServer(t)
	seedDrive(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/drives", nil)
	w := httptest.NewRecorder()
	h.ListDrives(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var body []state.Drive
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body) != 1 || body[0].DevPath != "/dev/sr0" {
		t.Errorf("got %+v", body)
	}
}

func TestGetDrive_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Get("/api/drives/{id}", h.GetDrive)

	req := httptest.NewRequest(http.MethodGet, "/api/drives/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func TestGetDrive_OK(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Get("/api/drives/{id}", h.GetDrive)

	req := httptest.NewRequest(http.MethodGet, "/api/drives/"+d.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got state.Drive
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != d.ID {
		t.Errorf("got %s want %s", got.ID, d.ID)
	}
}

func TestEjectDrive_TransitionsState(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Post("/api/drives/{id}/eject", h.EjectDrive)

	req := httptest.NewRequest(http.MethodPost, "/api/drives/"+d.ID+"/eject", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d", w.Code)
	}
	got, err := h.Store.GetDrive(context.Background(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != state.DriveStateEjecting {
		t.Errorf("state %s", got.State)
	}
}

func TestEjectDrive_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Post("/api/drives/{id}/eject", h.EjectDrive)

	req := httptest.NewRequest(http.MethodPost, "/api/drives/nope/eject", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}
