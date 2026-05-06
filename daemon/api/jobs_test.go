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

func TestListJobs_Empty(t *testing.T) {
	h := apitestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/jobs", nil)
	w := httptest.NewRecorder()
	h.ListJobs(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	if got := w.Body.String(); got != "null\n" && got != "[]\n" {
		t.Errorf("body=%q", got)
	}
}

func TestListJobs_FilterByState(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)
	j := &state.Job{
		DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID,
		State: state.JobStateQueued,
	}
	if err := h.Store.CreateJob(context.Background(), j); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/jobs?state=queued", nil)
	w := httptest.NewRecorder()
	h.ListJobs(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var jobs []state.Job
	if err := json.Unmarshal(w.Body.Bytes(), &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 {
		t.Errorf("count: %d", len(jobs))
	}

	// Filter that excludes
	req2 := httptest.NewRequest(http.MethodGet, "/api/jobs?state=done", nil)
	w2 := httptest.NewRecorder()
	h.ListJobs(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status %d", w2.Code)
	}
	var jobs2 []state.Job
	if err := json.Unmarshal(w2.Body.Bytes(), &jobs2); err != nil {
		t.Fatal(err)
	}
	if len(jobs2) != 0 {
		t.Errorf("count: %d", len(jobs2))
	}
}

func TestGetJob_OK(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)
	j := &state.Job{DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID}
	if err := h.Store.CreateJob(context.Background(), j); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/api/jobs/{id}", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+j.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got state.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != j.ID {
		t.Errorf("got %s want %s", got.ID, j.ID)
	}
	if len(got.Steps) != 8 {
		t.Errorf("steps: %d", len(got.Steps))
	}
}

func TestGetJob_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Get("/api/jobs/{id}", h.GetJob)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func TestPauseJob_Returns501(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Post("/api/jobs/{id}/pause", h.PauseJob)

	req := httptest.NewRequest(http.MethodPost, "/api/jobs/x/pause", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Errorf("status %d", w.Code)
	}
}
