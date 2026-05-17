package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func newJob(t *testing.T, s *state.Store, drv *state.Drive, prof *state.Profile, disc *state.Disc) *state.Job {
	t.Helper()
	j := &state.Job{
		DiscID:    disc.ID,
		DriveID:   drv.ID,
		ProfileID: prof.ID,
	}
	if err := s.CreateJob(context.Background(), j); err != nil {
		t.Fatalf("create job: %v", err)
	}
	return j
}

func newDisc(t *testing.T, s *state.Store, drv *state.Drive) *state.Disc {
	t.Helper()
	d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
	if err := s.CreateDisc(context.Background(), d); err != nil {
		t.Fatalf("create disc: %v", err)
	}
	return d
}

func TestStore_Job_CreateMaterializesEightSteps(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	j := &state.Job{
		DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID,
		Steps: []state.JobStep{
			{Step: state.StepTranscode, State: state.JobStepStateSkipped},
			{Step: state.StepCompress, State: state.JobStepStateSkipped},
		},
	}
	if err := s.CreateJob(ctx, j); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Steps) != 8 {
		t.Fatalf("want 8 steps, got %d", len(got.Steps))
	}

	skipped := map[state.StepID]bool{}
	pending := map[state.StepID]bool{}
	for _, st := range got.Steps {
		switch st.State {
		case state.JobStepStateSkipped:
			skipped[st.Step] = true
		case state.JobStepStatePending:
			pending[st.Step] = true
		}
	}
	if !skipped[state.StepTranscode] || !skipped[state.StepCompress] {
		t.Errorf("skipped set wrong: %v", skipped)
	}
	if len(pending) != 6 {
		t.Errorf("want 6 pending, got %d", len(pending))
	}
}

func TestStore_Job_GetIncludesSteps(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 8 {
		t.Errorf("want 8 steps, got %d", len(got.Steps))
	}
}

func TestStore_Job_ListJobs_Filters(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	for i := 0; i < 3; i++ {
		newJob(t, s, drv, prof, disc)
	}
	all, _ := s.ListJobs(ctx, state.JobFilter{Limit: 10})
	if err := s.UpdateJobState(ctx, all[0].ID, state.JobStateDone, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateJobState(ctx, all[1].ID, state.JobStateFailed, "boom"); err != nil {
		t.Fatal(err)
	}

	done, err := s.ListJobs(ctx, state.JobFilter{State: state.JobStateDone})
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 1 {
		t.Errorf("want 1 done, got %d", len(done))
	}

	failed, err := s.ListJobs(ctx, state.JobFilter{State: state.JobStateFailed})
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].ErrorMessage != "boom" {
		t.Errorf("failed mismatch: %+v", failed)
	}
}

func TestStore_Job_ListActiveAndRecent(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	jobs := make([]*state.Job, 5)
	for i := 0; i < 5; i++ {
		jobs[i] = newJob(t, s, drv, prof, disc)
	}
	for i := 0; i < 3; i++ {
		if err := s.UpdateJobState(ctx, jobs[i].ID, state.JobStateDone, ""); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListActiveAndRecentJobs(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	active, recent := 0, 0
	for _, j := range got {
		switch j.State {
		case state.JobStateDone, state.JobStateFailed, state.JobStateCancelled:
			recent++
		default:
			active++
		}
	}
	if active != 2 || recent != 2 {
		t.Errorf("want 2 active + 2 recent, got %d active + %d recent", active, recent)
	}
}

// Regression: ListActiveAndRecentJobs hydrates Steps for every returned
// job from a single batched query. Without batching, the dashboard's
// SSE bootstrap fanned out one query per job; the steps must match
// what ListJobSteps would return per-id.
func TestStore_Job_ListActiveAndRecent_HydratesSteps(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	jobs := make([]*state.Job, 4)
	for i := 0; i < 4; i++ {
		jobs[i] = newJob(t, s, drv, prof, disc)
	}

	got, err := s.ListActiveAndRecentJobs(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 4 {
		t.Fatalf("want 4 jobs, got %d", len(got))
	}
	for _, j := range got {
		if len(j.Steps) == 0 {
			t.Errorf("job %s has no steps after hydration", j.ID)
		}
		want, err := s.ListJobSteps(ctx, j.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(want) != len(j.Steps) {
			t.Errorf("job %s: hydrated %d steps, per-id query returned %d", j.ID, len(j.Steps), len(want))
			continue
		}
		for i := range want {
			if want[i].Step != j.Steps[i].Step || want[i].State != j.Steps[i].State {
				t.Errorf("job %s step %d: hydrated %+v vs per-id %+v", j.ID, i, j.Steps[i], want[i])
			}
		}
	}
}

func TestStore_Job_UpdateProgress(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if err := s.UpdateJobProgress(ctx, j.ID, state.StepRip, 42.5, "8.4×", 120, 60); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetJob(ctx, j.ID)
	if got.Progress != 42.5 || got.Speed != "8.4×" || got.ETASeconds != 120 {
		t.Errorf("progress not updated: %+v", got)
	}
}

func TestStore_Job_UpdateState_SetsTimestamps(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if err := s.UpdateJobState(ctx, j.ID, state.JobStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetJob(ctx, j.ID)
	if got.StartedAt == nil {
		t.Errorf("StartedAt not set on transition to running")
	}

	time.Sleep(2 * time.Millisecond)

	if err := s.UpdateJobState(ctx, j.ID, state.JobStateDone, ""); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetJob(ctx, j.ID)
	if got.FinishedAt == nil {
		t.Errorf("FinishedAt not set on transition to done")
	}
}

func TestStore_Job_MarkInterrupted(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	j1 := newJob(t, s, drv, prof, disc)
	j2 := newJob(t, s, drv, prof, disc)
	if err := s.UpdateJobState(ctx, j2.ID, state.JobStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	j3 := newJob(t, s, drv, prof, disc)
	if err := s.UpdateJobState(ctx, j3.ID, state.JobStateDone, ""); err != nil {
		t.Fatal(err)
	}

	n, err := s.MarkInterruptedJobs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("want 2 interrupted, got %d", n)
	}

	g1, _ := s.GetJob(ctx, j1.ID)
	g3, _ := s.GetJob(ctx, j3.ID)
	if g1.State != state.JobStateInterrupted {
		t.Errorf("j1: want interrupted, got %s", g1.State)
	}
	if g3.State != state.JobStateDone {
		t.Errorf("j3: want done unchanged, got %s", g3.State)
	}
}

func TestStore_Job_MarkInterrupted_FlipsRunningSteps(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	j := newJob(t, s, drv, prof, disc)
	if err := s.UpdateJobState(ctx, j.ID, state.JobStateRunning, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateJobStepState(ctx, j.ID, state.StepRip, state.JobStepStateRunning); err != nil {
		t.Fatalf("step running: %v", err)
	}

	if _, err := s.MarkInterruptedJobs(ctx); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != state.JobStateInterrupted {
		t.Errorf("job: want interrupted, got %s", got.State)
	}
	var rip *state.JobStep
	for i := range got.Steps {
		if got.Steps[i].Step == state.StepRip {
			rip = &got.Steps[i]
		}
	}
	if rip == nil {
		t.Fatal("rip step missing")
	}
	if rip.State != state.JobStepStateFailed {
		t.Errorf("rip step: want failed, got %s", rip.State)
	}
	if rip.FinishedAt == nil {
		t.Errorf("rip step: finished_at not stamped")
	}

	for _, st := range got.Steps {
		if st.Step == state.StepRip {
			continue
		}
		if st.State == state.JobStepStateRunning {
			t.Errorf("step %s still running after MarkInterruptedJobs", st.Step)
		}
	}
}

func TestStore_Job_FK_DiscDeleteCascades(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if _, err := s.DB().Conn().ExecContext(ctx, "DELETE FROM discs WHERE id = ?", disc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetJob(ctx, j.ID); err == nil {
		t.Errorf("job should have been cascade-deleted")
	}
}

func TestStore_UpdateJobSubStep_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if err := s.UpdateJobSubStep(ctx, j.ID, "REFINE"); err != nil {
		t.Fatalf("UpdateJobSubStep: %v", err)
	}
	got, err := s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveSubStep != "REFINE" {
		t.Errorf("ActiveSubStep = %q, want REFINE", got.ActiveSubStep)
	}

	// Clearing
	mustStore(t, s.UpdateJobSubStep(ctx, j.ID, ""))
	got, err = s.GetJob(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveSubStep != "" {
		t.Errorf("ActiveSubStep = %q after clear, want empty", got.ActiveSubStep)
	}
}

func TestStore_UpdateJobSubStep_NotFound(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	err := s.UpdateJobSubStep(ctx, "no-such-job", "REFINE")
	if err != state.ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}
