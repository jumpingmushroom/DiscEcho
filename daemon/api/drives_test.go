package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/api"
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

func TestEjectDrive_FiresEjectorAndReturnsToIdle(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)

	called := false
	gotDev := ""
	h.Ejector = func(_ context.Context, dev string) error {
		called = true
		gotDev = dev
		return nil
	}

	r := chi.NewRouter()
	r.Post("/api/drives/{id}/eject", h.EjectDrive)

	req := httptest.NewRequest(http.MethodPost, "/api/drives/"+d.ID+"/eject", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d", w.Code)
	}
	if !called {
		t.Fatal("ejector not called")
	}
	if gotDev != d.DevPath {
		t.Errorf("ejector got dev %q want %q", gotDev, d.DevPath)
	}
	got, err := h.Store.GetDrive(context.Background(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != state.DriveStateIdle {
		t.Errorf("post-eject state %s want idle", got.State)
	}
}

func TestEjectDrive_NoEjectorReturns503(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	// h.Ejector is nil by default in apitestServer.
	r := chi.NewRouter()
	r.Post("/api/drives/{id}/eject", h.EjectDrive)

	req := httptest.NewRequest(http.MethodPost, "/api/drives/"+d.ID+"/eject", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d want 503", w.Code)
	}
}

func TestEjectDrive_EjectorFailureRestoresIdle(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	h.Ejector = func(_ context.Context, _ string) error { return errors.New("boom") }
	r := chi.NewRouter()
	r.Post("/api/drives/{id}/eject", h.EjectDrive)

	req := httptest.NewRequest(http.MethodPost, "/api/drives/"+d.ID+"/eject", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status %d want 500", w.Code)
	}
	got, _ := h.Store.GetDrive(context.Background(), d.ID)
	if got.State != state.DriveStateIdle {
		t.Errorf("post-failed-eject state %s want idle", got.State)
	}
}

func TestEjectDrive_NotFound(t *testing.T) {
	h := apitestServer(t)
	h.Ejector = func(_ context.Context, _ string) error { return nil }
	r := chi.NewRouter()
	r.Post("/api/drives/{id}/eject", h.EjectDrive)

	req := httptest.NewRequest(http.MethodPost, "/api/drives/nope/eject", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

// Compile-time use of the api package import for the Ejector type when
// the file would otherwise not need it directly.
var _ api.Ejector = func(context.Context, string) error { return nil }

func TestPatchDriveOffset_HappyPath(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)

	req := httptest.NewRequest(http.MethodPatch, "/api/drives/"+d.ID+"/offset",
		strings.NewReader(`{"read_offset": 667}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d (%s)", w.Code, w.Body.String())
	}
	var got state.Drive
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ReadOffset != 667 || got.ReadOffsetSource != "manual" {
		t.Errorf("got offset=%d source=%q want 667/manual", got.ReadOffset, got.ReadOffsetSource)
	}
}

func TestPatchDriveOffset_NegativeRoundTrips(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)

	req := httptest.NewRequest(http.MethodPatch, "/api/drives/"+d.ID+"/offset",
		strings.NewReader(`{"read_offset": -1164}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	stored, _ := h.Store.GetDrive(context.Background(), d.ID)
	if stored.ReadOffset != -1164 {
		t.Errorf("stored offset: want -1164, got %d", stored.ReadOffset)
	}
}

func TestPatchDriveOffset_OutOfRange(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)

	for _, body := range []string{
		`{"read_offset": 9001}`,
		`{"read_offset": -9001}`,
	} {
		req := httptest.NewRequest(http.MethodPatch, "/api/drives/"+d.ID+"/offset",
			strings.NewReader(body))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnprocessableEntity {
			t.Errorf("body %s: status %d want 422", body, w.Code)
		}
	}
}

func TestPatchDriveOffset_MissingField(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)

	req := httptest.NewRequest(http.MethodPatch, "/api/drives/"+d.ID+"/offset",
		strings.NewReader(`{}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status %d want 422", w.Code)
	}
}

func TestPatchDriveOffset_InvalidJSON(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)

	req := httptest.NewRequest(http.MethodPatch, "/api/drives/"+d.ID+"/offset",
		strings.NewReader(`{not-json`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d want 400", w.Code)
	}
}

func TestPatchDriveOffset_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)

	req := httptest.NewRequest(http.MethodPatch, "/api/drives/ghost/offset",
		strings.NewReader(`{"read_offset": 0}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d want 404", w.Code)
	}
}

func TestPatchDriveOffset_ConflictWhenActiveJob(t *testing.T) {
	h := apitestServer(t)
	d := seedDrive(t, h)
	p := seedProfile(t, h)
	disc := seedDisc(t, h, d.ID)
	if err := h.Store.CreateJob(context.Background(), &state.Job{
		ID:        "job-active",
		DiscID:    disc.ID,
		DriveID:   d.ID,
		ProfileID: p.ID,
		State:     state.JobStateRunning,
	}); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Patch("/api/drives/{id}/offset", h.PatchDriveOffset)
	req := httptest.NewRequest(http.MethodPatch, "/api/drives/"+d.ID+"/offset",
		strings.NewReader(`{"read_offset": 0}`))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("status %d want 409", w.Code)
	}
}
