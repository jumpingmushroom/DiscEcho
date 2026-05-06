package jobs_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/jobs"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// jobFixture builds enough Drive/Disc/Profile/Job rows for sink tests.
type jobFixture struct {
	store *state.Store
	bc    *state.Broadcaster
	job   *state.Job
}

func newJobFixture(t *testing.T) *jobFixture {
	t.Helper()
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := state.NewStore(db)

	ctx := context.Background()
	drv := &state.Drive{
		DevPath: "/dev/sr0", Model: "X", Bus: "Y",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := store.UpsertDrive(ctx, drv); err != nil {
		t.Fatal(err)
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Name: "p", Engine: "whipper",
		Format: "FLAC", Preset: "x", OutputPathTemplate: "{{.Title}}.flac",
		Enabled: true, StepCount: 6,
	}
	if err := store.CreateProfile(ctx, prof); err != nil {
		t.Fatal(err)
	}
	disc := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD}
	if err := store.CreateDisc(ctx, disc); err != nil {
		t.Fatal(err)
	}
	job := &state.Job{DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID}
	if err := store.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}
	return &jobFixture{store: store, bc: state.NewBroadcaster(), job: job}
}

func TestPersistentSink_StepStartTransitions(t *testing.T) {
	fx := newJobFixture(t)
	defer fx.bc.Close()
	ch, cancel := fx.bc.Subscribe(16)
	defer cancel()

	s := jobs.NewPersistentSink(fx.store, fx.bc, fx.job.ID)
	s.OnStepStart(state.StepRip)

	steps, err := fx.store.ListJobSteps(context.Background(), fx.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range steps {
		if st.Step == state.StepRip && st.State != state.JobStepStateRunning {
			t.Errorf("rip should be running, got %s", st.State)
		}
	}

	select {
	case ev := <-ch:
		if ev.Name != "job.step" {
			t.Errorf("event name: %q", ev.Name)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("no event broadcast")
	}
}

func TestPersistentSink_ProgressCoalesces(t *testing.T) {
	fx := newJobFixture(t)
	defer fx.bc.Close()
	ch, cancel := fx.bc.Subscribe(64)
	defer cancel()

	s := jobs.NewPersistentSink(fx.store, fx.bc, fx.job.ID)

	// 50 quick progress updates → broadcaster should see at most a handful.
	for i := 0; i < 50; i++ {
		s.OnProgress(state.StepRip, float64(i*2), "5×", 30)
	}

	// Drain available events without blocking
	count := 0
	for {
		select {
		case ev := <-ch:
			if ev.Name == "job.progress" {
				count++
			}
		case <-time.After(50 * time.Millisecond):
			goto done
		}
	}
done:
	if count > 5 {
		t.Errorf("progress events should coalesce to ~1Hz, got %d in tight loop", count)
	}
	if count == 0 {
		t.Errorf("at least one progress event should fire")
	}
}

func TestPersistentSink_LogPersistsAndBroadcasts(t *testing.T) {
	fx := newJobFixture(t)
	defer fx.bc.Close()
	ch, cancel := fx.bc.Subscribe(16)
	defer cancel()

	s := jobs.NewPersistentSink(fx.store, fx.bc, fx.job.ID)
	s.OnLog(state.LogLevelInfo, "hello %s", "world")

	tail, err := fx.store.TailLogLines(context.Background(), fx.job.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 1 || tail[0].Message != "hello world" {
		t.Errorf("log not persisted: %+v", tail)
	}

	select {
	case ev := <-ch:
		if ev.Name != "job.log" {
			t.Errorf("name: %q", ev.Name)
		}
	case <-time.After(100 * time.Millisecond):
		t.Errorf("no broadcast")
	}
}

func TestPersistentSink_StepDoneRecordsNotes(t *testing.T) {
	fx := newJobFixture(t)
	defer fx.bc.Close()

	s := jobs.NewPersistentSink(fx.store, fx.bc, fx.job.ID)
	s.OnStepStart(state.StepRip)
	s.OnStepDone(state.StepRip, map[string]any{"accurate_rip": "ok"})

	steps, _ := fx.store.ListJobSteps(context.Background(), fx.job.ID)
	for _, st := range steps {
		if st.Step == state.StepRip {
			if st.State != state.JobStepStateDone {
				t.Errorf("rip state: %s", st.State)
			}
			if st.Notes["accurate_rip"] != "ok" {
				t.Errorf("notes: %+v", st.Notes)
			}
		}
	}
}

func TestPersistentSink_StepFailedRecordsState(t *testing.T) {
	fx := newJobFixture(t)
	defer fx.bc.Close()

	s := jobs.NewPersistentSink(fx.store, fx.bc, fx.job.ID)
	s.OnStepStart(state.StepRip)
	s.OnStepFailed(state.StepRip, errors.New("boom"))

	steps, _ := fx.store.ListJobSteps(context.Background(), fx.job.ID)
	for _, st := range steps {
		if st.Step == state.StepRip && st.State != state.JobStepStateFailed {
			t.Errorf("rip should be failed, got %s", st.State)
		}
	}
}
