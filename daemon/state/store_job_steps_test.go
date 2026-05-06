package state_test

import (
	"context"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestStore_JobStep_UpdateState_StartedAndFinished(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if err := s.UpdateJobStepState(ctx, j.ID, state.StepRip, state.JobStepStateRunning); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListJobSteps(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	var rip *state.JobStep
	for i := range got {
		if got[i].Step == state.StepRip {
			rip = &got[i]
			break
		}
	}
	if rip == nil {
		t.Fatal("rip step missing")
	}
	if rip.State != state.JobStepStateRunning || rip.StartedAt == nil {
		t.Errorf("rip running mismatch: %+v", rip)
	}
	if rip.AttemptCount != 1 {
		t.Errorf("attempt_count: want 1, got %d", rip.AttemptCount)
	}

	if err := s.UpdateJobStepState(ctx, j.ID, state.StepRip, state.JobStepStateDone); err != nil {
		t.Fatal(err)
	}
	got, _ = s.ListJobSteps(ctx, j.ID)
	for _, st := range got {
		if st.Step == state.StepRip && (st.State != state.JobStepStateDone || st.FinishedAt == nil) {
			t.Errorf("rip done mismatch: %+v", st)
		}
	}
}

func TestStore_JobStep_AppendNotes_Merges(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if err := s.AppendJobStepNotes(ctx, j.ID, state.StepRip,
		map[string]any{"accurate_rip": map[string]any{"track_1": 87}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendJobStepNotes(ctx, j.ID, state.StepRip,
		map[string]any{"speed_avg": "12.4×"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.ListJobSteps(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range got {
		if st.Step == state.StepRip {
			if st.Notes["speed_avg"] != "12.4×" {
				t.Errorf("speed_avg missing: %+v", st.Notes)
			}
			if _, ok := st.Notes["accurate_rip"]; !ok {
				t.Errorf("accurate_rip missing after merge: %+v", st.Notes)
			}
		}
	}
}

func TestStore_JobStep_UpdateNotFound(t *testing.T) {
	s := openStore(t)
	if err := s.UpdateJobStepState(context.Background(), "nope", state.StepRip, state.JobStepStateRunning); err == nil {
		t.Errorf("want ErrNotFound")
	}
}
