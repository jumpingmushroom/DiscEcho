package api_test

import (
	"bytes"
	"context"
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

func TestCreateProfile_HappyPath(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Post("/api/profiles", h.CreateProfile)

	body := []byte(`{
		"disc_type": "AUDIO_CD",
		"name": "CD-FLAC-2",
		"engine": "whipper",
		"format": "FLAC",
		"preset": "AccurateRip",
		"options": {},
		"output_path_template": "{{.Title}}.flac",
		"enabled": true,
		"step_count": 6
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got state.Profile
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" {
		t.Errorf("expected generated ID, got empty")
	}
	if got.Name != "CD-FLAC-2" {
		t.Errorf("name = %q", got.Name)
	}
	stored, err := h.Store.GetProfile(context.Background(), got.ID)
	if err != nil {
		t.Fatalf("store missing: %v", err)
	}
	if stored.Name != "CD-FLAC-2" {
		t.Errorf("stored name mismatch: %q", stored.Name)
	}
}

func TestCreateProfile_ValidationError(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Post("/api/profiles", h.CreateProfile)

	body := []byte(`{
		"disc_type": "DVD",
		"name": "DVD-MP3",
		"engine": "HandBrake",
		"format": "MP3",
		"options": {},
		"enabled": true,
		"step_count": 7
	}`)
	req := httptest.NewRequest(http.MethodPost, "/api/profiles", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if _, ok := resp["format"]; !ok {
		t.Errorf("expected `format` field in error response, got %+v", resp)
	}
}

func TestUpdateProfile_HappyPath(t *testing.T) {
	h := apitestServer(t)
	p := seedProfile(t, h)
	r := chi.NewRouter()
	r.Put("/api/profiles/{id}", h.UpdateProfile)

	body, err := json.Marshal(map[string]any{
		"disc_type":            string(p.DiscType),
		"name":                 "CD-FLAC-renamed",
		"engine":               p.Engine,
		"format":               p.Format,
		"preset":               p.Preset,
		"options":              p.Options,
		"output_path_template": p.OutputPathTemplate,
		"enabled":              p.Enabled,
		"step_count":           p.StepCount,
	})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/profiles/"+p.ID, bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var got state.Profile
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Name != "CD-FLAC-renamed" {
		t.Errorf("name not updated: %q", got.Name)
	}
	if !got.UpdatedAt.After(p.UpdatedAt) && !got.UpdatedAt.Equal(p.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want >= %v", got.UpdatedAt, p.UpdatedAt)
	}
}

func TestUpdateProfile_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Put("/api/profiles/{id}", h.UpdateProfile)

	body := []byte(`{
		"disc_type": "AUDIO_CD",
		"name": "x",
		"engine": "whipper",
		"format": "FLAC",
		"options": {},
		"step_count": 6
	}`)
	req := httptest.NewRequest(http.MethodPut, "/api/profiles/no-such-id", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", w.Code)
	}
}

func TestDeleteProfile_HappyPath(t *testing.T) {
	h := apitestServer(t)
	p := seedProfile(t, h)
	r := chi.NewRouter()
	r.Delete("/api/profiles/{id}", h.DeleteProfile)

	req := httptest.NewRequest(http.MethodDelete, "/api/profiles/"+p.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	if _, err := h.Store.GetProfile(context.Background(), p.ID); err == nil {
		t.Errorf("profile still in store after delete")
	}
}

func TestDeleteProfile_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Delete("/api/profiles/{id}", h.DeleteProfile)
	req := httptest.NewRequest(http.MethodDelete, "/api/profiles/no-such-id", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", w.Code)
	}
}

func TestDeleteProfile_ActiveJobConflict(t *testing.T) {
	h := apitestServer(t)
	p := seedProfile(t, h)

	drv := &state.Drive{
		ID:      "drv-test",
		Model:   "X",
		Bus:     "USB",
		DevPath: "/dev/sr0",
		State:   state.DriveStateIdle,
	}
	if err := h.Store.UpsertDrive(context.Background(), drv); err != nil {
		t.Fatal(err)
	}
	disc := &state.Disc{
		Type:    state.DiscTypeAudioCD,
		DriveID: drv.ID,
	}
	if err := h.Store.CreateDisc(context.Background(), disc); err != nil {
		t.Fatal(err)
	}
	job := &state.Job{
		DiscID:    disc.ID,
		DriveID:   drv.ID,
		ProfileID: p.ID,
		State:     state.JobStateRunning,
	}
	if err := h.Store.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Delete("/api/profiles/{id}", h.DeleteProfile)
	req := httptest.NewRequest(http.MethodDelete, "/api/profiles/"+p.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("status %d, want 409 (active job)", w.Code)
	}
}
