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
	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
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
	// gate, when non-nil, blocks Run until the test closes it — keeps a
	// submitted job in a non-terminal state so concurrency tests can
	// observe it as "active". Nil → Run completes immediately.
	gate chan struct{}
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
	if s.gate != nil {
		<-s.gate
	}
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

func TestStartDisc_PersistsMetadataBlob_TMDB(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)
	h.TMDB = &fakeTMDBForAPI{
		movieDetails: identify.DiscMetadata{
			Director:  "Jeff Tremaine",
			PosterURL: "https://image.tmdb.org/t/p/w342/abc.jpg",
		},
	}

	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)
	disc.Candidates = []state.Candidate{
		{Source: "TMDB", Title: "Jackass: The Movie", Year: 2002, TMDBID: 329865, MediaType: "movie"},
	}
	if err := h.Store.UpdateDiscCandidates(context.Background(), disc.ID, disc.Candidates); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/start", h.StartDisc)
	body := mustJSON(t, map[string]any{"profile_id": prof.ID, "candidate_index": 0})
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+disc.ID+"/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	got, err := h.Store.GetDisc(context.Background(), disc.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MetadataJSON == "" || got.MetadataJSON == "{}" {
		t.Fatalf("metadata_json empty after start: %q", got.MetadataJSON)
	}
	if !bytes.Contains([]byte(got.MetadataJSON), []byte(`"director":"Jeff Tremaine"`)) {
		t.Errorf("expected director in blob: %s", got.MetadataJSON)
	}
	if !bytes.Contains([]byte(got.MetadataJSON), []byte(`"poster_url":"https://image.tmdb.org/t/p/w342/abc.jpg"`)) {
		t.Errorf("expected poster_url in blob: %s", got.MetadataJSON)
	}

	// Drain the orchestrator's stub job so cleanup doesn't race.
	var j state.Job
	_ = json.Unmarshal(w.Body.Bytes(), &j)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		gj, err := h.Store.GetJob(context.Background(), j.ID)
		if err == nil && (gj.State == state.JobStateDone || gj.State == state.JobStateFailed) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartDisc_RefusesDuplicateWhenActiveJobExists(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)

	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)

	// Seed an already-active job for this disc so the handler sees the
	// guard condition without depending on orchestrator scheduling.
	existing := &state.Job{
		DiscID:    disc.ID,
		DriveID:   drv.ID,
		ProfileID: prof.ID,
	}
	if err := h.Store.CreateJob(context.Background(), existing); err != nil {
		t.Fatal(err)
	}

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

	if w.Code != http.StatusConflict {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
}

// TestStartDisc_ConcurrentRequestsCreateOneJob is the regression for
// duplicate rips: the dashboard mounts both the mobile and desktop
// component trees at once, so a disc's auto-confirm fires twice and
// two POST /start requests race within milliseconds. The handler's
// active-job check must be serialized with job submission so exactly
// one job is created — the rest get 409.
func TestStartDisc_ConcurrentRequestsCreateOneJob(t *testing.T) {
	reg := pipelines.NewRegistry()
	// Gated stub: the submitted job stays non-terminal until we close
	// the gate, so the racing requests observe it as an active job —
	// in production a rip runs for minutes, an instant stub would let
	// the job reach `done` before the slower requests re-check.
	stub := &stubDiscHandler{gate: make(chan struct{})}
	reg.Register(stub)
	h := apitestServerWithOrch(t, reg)

	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/start", h.StartDisc)

	body := mustJSON(t, map[string]any{"profile_id": prof.ID, "candidate_index": 0})

	const n = 8
	var wg sync.WaitGroup
	codes := make([]int, n)
	release := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/discs/"+disc.ID+"/start", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			<-release
			r.ServeHTTP(w, req)
			codes[i] = w.Code
		}(i)
	}
	close(release)
	wg.Wait()

	jobsList, err := h.Store.ListJobs(context.Background(), state.JobFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(jobsList) != 1 {
		t.Errorf("want exactly 1 job from %d concurrent StartDisc calls, got %d", n, len(jobsList))
	}
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		}
	}
	if ok != 1 || conflict != n-1 {
		t.Errorf("want 1×200 + %d×409, got %d×200 + %d×409 (codes=%v)", n-1, ok, conflict, codes)
	}

	// Release the gated job so the orchestrator cleanup doesn't hang.
	close(stub.gate)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		js, _ := h.Store.ListJobs(context.Background(), state.JobFilter{})
		if len(js) >= 1 && (js[0].State == state.JobStateDone || js[0].State == state.JobStateFailed) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartDisc_AllowsRestartAfterPreviousJobTerminal(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)

	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)

	// Done jobs don't block a re-start — the user can re-rip a disc
	// after a previous run finished or failed.
	done := &state.Job{
		DiscID:    disc.ID,
		DriveID:   drv.ID,
		ProfileID: prof.ID,
	}
	if err := h.Store.CreateJob(context.Background(), done); err != nil {
		t.Fatal(err)
	}
	if err := h.Store.UpdateJobState(context.Background(), done.ID, state.JobStateDone, ""); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/start", h.StartDisc)

	body := mustJSON(t, map[string]any{"profile_id": prof.ID})
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+disc.ID+"/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}

	// Same wait-for-terminal pattern as TestStartDisc_CreatesJob so the
	// orchestrator cleanup doesn't race with the running stub job.
	var got state.Job
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
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

func TestDeleteDisc_RemovesOrphanRow(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	disc := seedDisc(t, h, drv.ID)

	r := chi.NewRouter()
	r.Delete("/api/discs/{id}", h.DeleteDisc)
	req := httptest.NewRequest(http.MethodDelete, "/api/discs/"+disc.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status %d body=%s", w.Code, w.Body.String())
	}
	if _, err := h.Store.GetDisc(context.Background(), disc.ID); err == nil {
		t.Errorf("disc still present after delete")
	}
}

func TestDeleteDisc_RefusesWhenJobExists(t *testing.T) {
	reg := pipelines.NewRegistry()
	reg.Register(&stubDiscHandler{})
	h := apitestServerWithOrch(t, reg)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	disc := seedDisc(t, h, drv.ID)

	// Submit a job so the disc has history.
	if _, err := h.Orchestrator.Submit(context.Background(), disc.ID, prof.ID); err != nil {
		t.Fatal(err)
	}
	// Wait for the orchestrator stub to finish so cleanup doesn't race.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		jobs, _ := h.Store.ListJobs(context.Background(), state.JobFilter{})
		if len(jobs) == 1 && (jobs[0].State == state.JobStateDone || jobs[0].State == state.JobStateFailed) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	r := chi.NewRouter()
	r.Delete("/api/discs/{id}", h.DeleteDisc)
	req := httptest.NewRequest(http.MethodDelete, "/api/discs/"+disc.ID, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("want 409 Conflict for disc-with-job; got %d body=%s", w.Code, w.Body.String())
	}
	if _, err := h.Store.GetDisc(context.Background(), disc.ID); err != nil {
		t.Errorf("disc must remain after failed delete: %v", err)
	}
}

func TestDeleteDisc_NotFound(t *testing.T) {
	h := apitestServer(t)
	r := chi.NewRouter()
	r.Delete("/api/discs/{id}", h.DeleteDisc)
	req := httptest.NewRequest(http.MethodDelete, "/api/discs/nope", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", w.Code)
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

func TestIdentifyDisc_EmptyBodyReturnsCurrent(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	d := &state.Disc{
		DriveID: drv.ID, Type: state.DiscTypeDVD, Title: "Existing",
		Candidates: []state.Candidate{
			{Source: "TMDB", Title: "Existing", MediaType: "movie", TMDBID: 1, Confidence: 50},
		},
	}
	if err := h.Store.CreateDisc(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/identify", h.IdentifyDisc)
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+d.ID+"/identify", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status: %d", w.Code)
	}
}

func TestIdentifyDisc_ManualQueryHitsTMDBAndUpdates(t *testing.T) {
	h := apitestServer(t)
	fakeCands := []state.Candidate{
		{Source: "TMDB", Title: "Found", Year: 2020, MediaType: "movie", TMDBID: 99, Confidence: 80},
	}
	h.TMDB = &fakeTMDBForAPI{cands: fakeCands}

	drv := seedDrive(t, h)
	d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeDVD, Title: ""}
	if err := h.Store.CreateDisc(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/identify", h.IdentifyDisc)
	body := bytes.NewReader([]byte(`{"query":"Found","media_type":"movie"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+d.ID+"/identify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	var out struct {
		Candidates []state.Candidate `json:"candidates"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Candidates) != 1 || out.Candidates[0].TMDBID != 99 {
		t.Errorf("got %+v", out.Candidates)
	}

	got, _ := h.Store.GetDisc(context.Background(), d.ID)
	if len(got.Candidates) != 1 {
		t.Errorf("not persisted")
	}
}

type fakeTMDBForAPI struct {
	cands        []state.Candidate
	movieDetails identify.DiscMetadata
	tvDetails    identify.DiscMetadata
}

func (f *fakeTMDBForAPI) SearchMovie(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, nil
}
func (f *fakeTMDBForAPI) SearchTV(_ context.Context, _ string) ([]state.Candidate, error) {
	return nil, nil
}
func (f *fakeTMDBForAPI) SearchBoth(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, nil
}
func (f *fakeTMDBForAPI) MovieRuntime(_ context.Context, _ int) (int, error) { return 0, nil }
func (f *fakeTMDBForAPI) MovieDetails(_ context.Context, _ int) (identify.DiscMetadata, error) {
	return f.movieDetails, nil
}
func (f *fakeTMDBForAPI) TVDetails(_ context.Context, _ int) (identify.DiscMetadata, error) {
	return f.tvDetails, nil
}

type fakeMBForAPI struct {
	cands       []state.Candidate
	searchCalls []string
}

func (f *fakeMBForAPI) Lookup(_ context.Context, _ string) ([]state.Candidate, error) {
	return nil, nil
}
func (f *fakeMBForAPI) ReleaseDetails(_ context.Context, _ string) (identify.AudioCDMetadata, error) {
	return identify.AudioCDMetadata{}, nil
}
func (f *fakeMBForAPI) SearchByName(_ context.Context, query string) ([]state.Candidate, error) {
	f.searchCalls = append(f.searchCalls, query)
	return f.cands, nil
}

func TestIdentifyDisc_AudioCD_DispatchesToMusicBrainz(t *testing.T) {
	h := apitestServer(t)
	mb := &fakeMBForAPI{
		cands: []state.Candidate{
			{Source: "MusicBrainz", Title: "Fear and Bullets", Artist: "Trust Obey", Year: 1997, MBID: "r1", Confidence: 100},
		},
	}
	h.MusicBrainz = mb
	// Make TMDB fail loudly if it's reached — guards against the audio
	// path falling through to the video dispatch.
	h.TMDB = nil

	drv := seedDrive(t, h)
	d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, TOCHash: "x"}
	if err := h.Store.CreateDisc(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/identify", h.IdentifyDisc)
	body := bytes.NewReader([]byte(`{"query":"trust obey fear and bullets"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+d.ID+"/identify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if len(mb.searchCalls) != 1 || mb.searchCalls[0] != "trust obey fear and bullets" {
		t.Errorf("MusicBrainz.SearchByName not called with the query: %v", mb.searchCalls)
	}

	got, _ := h.Store.GetDisc(context.Background(), d.ID)
	if len(got.Candidates) != 1 || got.Candidates[0].MBID != "r1" {
		t.Errorf("candidates not persisted: %+v", got.Candidates)
	}
}

func TestIdentifyDisc_AudioCD_MBNotConfigured_Returns503(t *testing.T) {
	h := apitestServer(t)
	h.MusicBrainz = nil

	drv := seedDrive(t, h)
	d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, TOCHash: "x"}
	if err := h.Store.CreateDisc(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Post("/api/discs/{id}/identify", h.IdentifyDisc)
	body := bytes.NewReader([]byte(`{"query":"anything"}`))
	req := httptest.NewRequest(http.MethodPost, "/api/discs/"+d.ID+"/identify", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status: want 503, got %d body=%s", w.Code, w.Body.String())
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
