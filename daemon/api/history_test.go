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

func TestHistoryHandler_PaginatedResponse(t *testing.T) {
	h := apitestServer(t)
	ctx := context.Background()
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	for i := 0; i < 3; i++ {
		d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
		_ = h.Store.CreateDisc(ctx, d)
		j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
		_ = h.Store.CreateJob(ctx, j)
		_ = h.Store.UpdateJobState(ctx, j.ID, state.JobStateDone, "")
	}

	r := chi.NewRouter()
	r.Get("/api/history", h.ListHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/history?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}

	var body struct {
		Rows   []state.HistoryRow `json:"rows"`
		Total  int                `json:"total"`
		Limit  int                `json:"limit"`
		Offset int                `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 3 || len(body.Rows) != 3 || body.Limit != 10 || body.Offset != 0 {
		t.Errorf("body: %+v", body)
	}
}

func TestHistoryHandler_TypeFilter(t *testing.T) {
	h := apitestServer(t)
	ctx := context.Background()
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	for _, dt := range []state.DiscType{state.DiscTypeAudioCD, state.DiscTypeDVD, state.DiscTypeDVD} {
		d := &state.Disc{DriveID: drv.ID, Type: dt, Title: "x"}
		_ = h.Store.CreateDisc(ctx, d)
		j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
		_ = h.Store.CreateJob(ctx, j)
		_ = h.Store.UpdateJobState(ctx, j.ID, state.JobStateDone, "")
	}

	r := chi.NewRouter()
	r.Get("/api/history", h.ListHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/history?type=DVD", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		Total int `json:"total"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Total != 2 {
		t.Errorf("DVD total: want 2, got %d", body.Total)
	}
}

func TestHistoryHandler_BadParamsClamp(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Get("/api/history", h.ListHistory)

	req := httptest.NewRequest(http.MethodGet, "/api/history?limit=999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status: %d", w.Code)
	}
	var body struct {
		Limit int `json:"limit"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body.Limit != 200 {
		t.Errorf("limit clamp: got %d", body.Limit)
	}
}
