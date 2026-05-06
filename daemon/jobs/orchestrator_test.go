package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/jobs"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// stubHandler counts Run invocations and lets tests delay/cancel.
type stubHandler struct {
	delay     time.Duration
	failOnRun error
	calls     int
	mu        sync.Mutex
	startedCh chan struct{}
}

func (s *stubHandler) DiscType() state.DiscType { return state.DiscTypeAudioCD }
func (s *stubHandler) Identify(_ context.Context, _ *state.Drive) (*state.Disc, []state.Candidate, error) {
	return nil, nil, nil
}
func (s *stubHandler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	return nil
}
func (s *stubHandler) Run(ctx context.Context, _ *state.Drive, _ *state.Disc, _ *state.Profile, sink pipelines.EventSink) error {
	s.mu.Lock()
	s.calls++
	startCh := s.startedCh
	s.mu.Unlock()

	if startCh != nil {
		select {
		case startCh <- struct{}{}:
		default:
		}
	}

	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return s.failOnRun
}

func openOrch(t *testing.T) (*state.Store, *state.Broadcaster, *stubHandler) {
	t.Helper()
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "x.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := state.NewStore(db)
	return store, state.NewBroadcaster(), &stubHandler{}
}

func seedJobInputs(t *testing.T, store *state.Store) (*state.Drive, *state.Disc, *state.Profile) {
	t.Helper()
	ctx := context.Background()
	drv := &state.Drive{DevPath: "/dev/sr0", Model: "x", Bus: "y",
		State: state.DriveStateIdle, LastSeenAt: time.Now()}
	if err := store.UpsertDrive(ctx, drv); err != nil {
		t.Fatal(err)
	}
	prof := &state.Profile{DiscType: state.DiscTypeAudioCD, Name: "p",
		Engine: "x", Format: "y", Preset: "z",
		OutputPathTemplate: "{{.Title}}", Enabled: true, StepCount: 6}
	if err := store.CreateProfile(ctx, prof); err != nil {
		t.Fatal(err)
	}
	disc := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD,
		Candidates: []state.Candidate{{Source: "x", Title: "a", MBID: "1", Confidence: 90}}}
	if err := store.CreateDisc(ctx, disc); err != nil {
		t.Fatal(err)
	}
	return drv, disc, prof
}

func TestOrchestrator_SubmitRunsHandler(t *testing.T) {
	store, bc, h := openOrch(t)
	defer bc.Close()
	reg := pipelines.NewRegistry()
	reg.Register(h)

	o := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store: store, Broadcaster: bc, Pipelines: reg,
	})
	t.Cleanup(o.Close)

	_, disc, prof := seedJobInputs(t, store)

	job, err := o.Submit(context.Background(), disc.ID, prof.ID)
	if err != nil {
		t.Fatal(err)
	}
	if job.State != state.JobStateQueued {
		t.Errorf("submit state: %s", job.State)
	}

	// Wait for completion
	if err := waitJobState(store, job.ID, state.JobStateDone, 2*time.Second); err != nil {
		t.Fatal(err)
	}
	if h.calls != 1 {
		t.Errorf("handler calls: %d", h.calls)
	}
}

func TestOrchestrator_PerDriveSerialization(t *testing.T) {
	store, bc, h := openOrch(t)
	defer bc.Close()
	h.delay = 100 * time.Millisecond
	h.startedCh = make(chan struct{}, 4)
	reg := pipelines.NewRegistry()
	reg.Register(h)

	o := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store: store, Broadcaster: bc, Pipelines: reg,
	})
	t.Cleanup(o.Close)

	_, disc, prof := seedJobInputs(t, store)
	if _, err := o.Submit(context.Background(), disc.ID, prof.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Submit(context.Background(), disc.ID, prof.ID); err != nil {
		t.Fatal(err)
	}

	// First job starts; second should still be queued
	<-h.startedCh
	time.Sleep(20 * time.Millisecond)
	all, _ := store.ListJobs(context.Background(), state.JobFilter{})
	running := 0
	for _, j := range all {
		if j.State == state.JobStateRunning {
			running++
		}
	}
	if running != 1 {
		t.Errorf("at most 1 job per drive should be running, got %d", running)
	}

	// Wait for both
	for _, j := range all {
		_ = waitJobState(store, j.ID, state.JobStateDone, 2*time.Second)
	}
	if h.calls != 2 {
		t.Errorf("calls: %d", h.calls)
	}
}

func TestOrchestrator_Cancel(t *testing.T) {
	store, bc, h := openOrch(t)
	defer bc.Close()
	h.delay = 5 * time.Second // long-running
	h.startedCh = make(chan struct{}, 1)
	reg := pipelines.NewRegistry()
	reg.Register(h)

	o := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store: store, Broadcaster: bc, Pipelines: reg,
	})
	t.Cleanup(o.Close)

	_, disc, prof := seedJobInputs(t, store)
	job, err := o.Submit(context.Background(), disc.ID, prof.ID)
	if err != nil {
		t.Fatal(err)
	}
	<-h.startedCh

	if err := o.Cancel(job.ID); err != nil {
		t.Fatal(err)
	}
	if err := waitJobState(store, job.ID, state.JobStateCancelled, 1*time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestOrchestrator_HandlerFailureMarksFailed(t *testing.T) {
	store, bc, h := openOrch(t)
	defer bc.Close()
	h.failOnRun = errors.New("boom")
	reg := pipelines.NewRegistry()
	reg.Register(h)

	o := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store: store, Broadcaster: bc, Pipelines: reg,
	})
	t.Cleanup(o.Close)

	_, disc, prof := seedJobInputs(t, store)
	job, err := o.Submit(context.Background(), disc.ID, prof.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitJobState(store, job.ID, state.JobStateFailed, 1*time.Second); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetJob(context.Background(), job.ID)
	if got.ErrorMessage == "" {
		t.Errorf("error_message should be set")
	}
}

func TestOrchestrator_CrashRecoveryMarksInterrupted(t *testing.T) {
	store, bc, _ := openOrch(t)
	defer bc.Close()

	_, disc, prof := seedJobInputs(t, store)
	job := &state.Job{DiscID: disc.ID, ProfileID: prof.ID, State: state.JobStateRunning}
	if err := store.CreateJob(context.Background(), job); err != nil {
		t.Fatal(err)
	}

	// Constructing the orchestrator runs MarkInterruptedJobs
	reg := pipelines.NewRegistry()
	o := jobs.NewOrchestrator(jobs.OrchestratorConfig{
		Store: store, Broadcaster: bc, Pipelines: reg,
	})
	t.Cleanup(o.Close)

	got, _ := store.GetJob(context.Background(), job.ID)
	if got.State != state.JobStateInterrupted {
		t.Errorf("want interrupted, got %s", got.State)
	}
}

func waitJobState(store *state.Store, id string, want state.JobState, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		j, err := store.GetJob(context.Background(), id)
		if err == nil && j.State == want {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	j, _ := store.GetJob(context.Background(), id)
	return errors.New("timeout waiting for state " + string(want) + " (got " + string(j.State) + ")")
}
