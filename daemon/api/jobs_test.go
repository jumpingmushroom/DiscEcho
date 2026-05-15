package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/api"
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
	var got api.JobDetail
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Job.ID != j.ID {
		t.Errorf("job id: got %s want %s", got.Job.ID, j.ID)
	}
	if len(got.Job.Steps) != 8 {
		t.Errorf("steps: %d", len(got.Job.Steps))
	}
	if got.Disc.ID != disc.ID {
		t.Errorf("disc id: got %s want %s", got.Disc.ID, disc.ID)
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

func TestListJobLogs_FiltersByStep(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)
	j := &state.Job{DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID}
	ctx := context.Background()
	if err := h.Store.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	lines := []state.LogLine{
		{JobID: j.ID, T: now, Step: state.StepRip, Level: state.LogLevelInfo, Message: "r1"},
		{JobID: j.ID, T: now.Add(time.Millisecond), Step: state.StepRip, Level: state.LogLevelInfo, Message: "r2"},
		{JobID: j.ID, T: now.Add(2 * time.Millisecond), Step: state.StepTranscode, Level: state.LogLevelInfo, Message: "t1"},
	}
	for _, l := range lines {
		if err := h.Store.AppendLogLine(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	r := chi.NewRouter()
	r.Get("/api/jobs/{id}/logs", h.ListJobLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/"+j.ID+"/logs?step=rip", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var got api.JobLogsResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || len(got.Lines) != 2 {
		t.Errorf("rip filter: total=%d len=%d", got.Total, len(got.Lines))
	}
	if got.Lines[0].Message != "r1" || got.Lines[1].Message != "r2" {
		t.Errorf("rip order: %+v", got.Lines)
	}

	// No filter returns everything, oldest first.
	req2 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+j.ID+"/logs", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	var all api.JobLogsResponse
	if err := json.Unmarshal(w2.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if all.Total != 3 || len(all.Lines) != 3 {
		t.Errorf("unfiltered: total=%d len=%d", all.Total, len(all.Lines))
	}
}

func TestListJobLogs_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Get("/api/jobs/{id}/logs", h.ListJobLogs)

	req := httptest.NewRequest(http.MethodGet, "/api/jobs/nope/logs", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func TestDeleteJob_TerminalSucceedsAndPrunesOrphanDisc(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)
	j := &state.Job{
		DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID,
		State: state.JobStateDone,
	}
	ctx := context.Background()
	if err := h.Store.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.UpdateJobState(ctx, j.ID, state.JobStateDone, ""); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Delete("/api/jobs/{id}", h.DeleteJob)

	req := httptest.NewRequest(http.MethodDelete, "/api/jobs/"+j.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}

	if _, err := h.Store.GetJob(ctx, j.ID); err == nil {
		t.Errorf("job still exists after delete")
	}
	if _, err := h.Store.GetDisc(ctx, disc.ID); err == nil {
		t.Errorf("orphan disc still exists after delete")
	}
}

func TestDeleteJob_RunningReturnsConflict(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)
	j := &state.Job{DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID}
	ctx := context.Background()
	if err := h.Store.CreateJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.UpdateJobState(ctx, j.ID, state.JobStateRunning, ""); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Delete("/api/jobs/{id}", h.DeleteJob)

	req := httptest.NewRequest(http.MethodDelete, "/api/jobs/"+j.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Errorf("status %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteJob_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Delete("/api/jobs/{id}", h.DeleteJob)

	req := httptest.NewRequest(http.MethodDelete, "/api/jobs/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}
