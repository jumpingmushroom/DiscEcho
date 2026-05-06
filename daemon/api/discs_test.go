package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/jobs"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// stubDiscHandler is the minimal Handler the orchestrator needs to
// dispatch a job. Run completes immediately so tests don't have to wait
// for any actual rip work.
type stubDiscHandler struct {
	mu    sync.Mutex
	calls int
}

func (s *stubDiscHandler) DiscType() state.DiscType { return state.DiscTypeAudioCD }
func (s *stubDiscHandler) Identify(_ context.Context, _ *state.Drive) (*state.Disc, []state.Candidate, error) {
	return nil, nil, nil
}
func (s *stubDiscHandler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	return nil
}
func (s *stubDiscHandler) Run(_ context.Context, _ *state.Drive, _ *state.Disc, _ *state.Profile, _ pipelines.EventSink) error {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return nil
}

// apitestServerWithOrch returns Handlers wired to a real Orchestrator
// + the supplied registry so endpoint tests can submit jobs.
func apitestServerWithOrch(t *testing.T, reg *pipelines.Registry) *api.Handlers {
	t.Helper()
	h := apitestServer(t)
	h.Pipelines = reg
	o := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store: h.Store, Broadcaster: h.Broadcaster, Pipelines: reg,
	})
	t.Cleanup(o.Close)
	h.Orchestrator = o
	return h
}

func TestStartDisc_CreatesJob(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)

	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/start", h.StartDisc)

	body := mustJSON(t, map[string]any{
		"profile_id":      prof.ID,
		"candidate_index": 0,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+disc.ID+"/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	var got state.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" || got.DiscID != disc.ID || got.ProfileID != prof.ID {
		t.Errorf("got %+v", got)
	}

	// Wait for orchestrator to drive the queued job to a terminal state
	// so the test's t.Cleanup(o.Close) doesn't race with active work.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		j, err := h.Store.GetJob(context.Background(), got.ID)
		if err == nil && (j.State == state.JobStateDone || j.State == state.JobStateFailed) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job did not finish")
}

func TestStartDisc_MissingProfileID(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)
	disc := seedDisc(t, h, seedDrive(t, h).ID)

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/start", h.StartDisc)
	body := mustJSON(t, map[string]any{"candidate_index": 0})
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+disc.ID+"/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status %d", w.Code)
	}
}

func TestStartDisc_DiscNotFound(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)
	prof := seedProfile(t, h)

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/start", h.StartDisc)
	body := mustJSON(t, map[string]any{"profile_id": prof.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/discs/nope/start", bytes.NewReader(body))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func TestIdentifyDisc_ReturnsCandidates(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	disc := seedDisc(t, h, drv.ID)

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/identify", h.IdentifyDisc)

	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+disc.ID+"/identify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var body struct {
		Disc       state.Disc        `json:"disc"`
		Candidates []state.Candidate `json:"candidates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Disc.ID != disc.ID {
		t.Errorf("disc id %s", body.Disc.ID)
	}
	if len(body.Candidates) != 1 || body.Candidates[0].MBID != "mbid-1" {
		t.Errorf("candidates %+v", body.Candidates)
	}
}

func TestIdentifyDisc_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Post("/api/discs/{id}/identify", h.IdentifyDisc)
	req := httptest.NewRequest(http.MethodPost, "/api/discs/nope/identify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d", w.Code)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
