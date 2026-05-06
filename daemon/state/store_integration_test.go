package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// TestStore_FullCycle exercises the full M1.1-relevant chain:
// drive → profile → disc → job (with steps) → step transitions →
// log lines → cascade-delete via FK. If this passes, the Store layer
// is at parity with what Phase D's PersistentSink will need.
func TestStore_FullCycle(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	drv := &state.Drive{
		DevPath: "/dev/sr0", Model: "Test", Bus: "USB",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, drv); err != nil {
		t.Fatal(err)
	}

	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Name: "CD-FLAC",
		Engine: "whipper", Format: "FLAC", Preset: "AccurateRip",
		Options: map[string]any{}, OutputPathTemplate: "{{.Album}}/{{.Title}}.flac",
		Enabled: true, StepCount: 6,
	}
	if err := s.CreateProfile(ctx, prof); err != nil {
		t.Fatal(err)
	}

	disc := &state.Disc{
		DriveID: drv.ID, Type: state.DiscTypeAudioCD,
		Title: "Kind of Blue", Year: 1959, TOCHash: "sha1abc",
		Candidates: []state.Candidate{
			{Source: "MusicBrainz", Title: "Kind of Blue", Year: 1959, Confidence: 94, MBID: "kb-1"},
		},
	}
	if err := s.CreateDisc(ctx, disc); err != nil {
		t.Fatal(err)
	}

	job := &state.Job{
		DiscID: disc.ID, DriveID: drv.ID, ProfileID: prof.ID,
		Steps: []state.JobStep{
			{Step: state.StepTranscode, State: state.JobStepStateSkipped},
			{Step: state.StepCompress, State: state.JobStepStateSkipped},
		},
	}
	if err := s.CreateJob(ctx, job); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Steps) != 8 {
		t.Errorf("want 8 steps, got %d", len(got.Steps))
	}

	if err := s.UpdateJobStepState(ctx, job.ID, state.StepRip, state.JobStepStateRunning); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateJobProgress(ctx, job.ID, state.StepRip, 50, "10×", 60, 30); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendJobStepNotes(ctx, job.ID, state.StepRip,
		map[string]any{"accurate_rip": map[string]any{"track_1": 87}}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateJobStepState(ctx, job.ID, state.StepRip, state.JobStepStateDone); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 3; i++ {
		_ = s.AppendLogLine(ctx, state.LogLine{
			JobID: job.ID, T: time.Now(),
			Level: state.LogLevelInfo, Message: "phase A test",
		})
	}

	if err := s.UpdateJobState(ctx, job.ID, state.JobStateDone, ""); err != nil {
		t.Fatal(err)
	}

	active, err := s.ListActiveAndRecentJobs(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Errorf("want 1 in active+recent, got %d", len(active))
	}

	if _, err := s.DB().Conn().ExecContext(ctx, "DELETE FROM discs WHERE id = ?", disc.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetJob(ctx, job.ID); err == nil {
		t.Errorf("job should have been cascade-deleted")
	}
	steps, _ := s.ListJobSteps(ctx, job.ID)
	if len(steps) != 0 {
		t.Errorf("steps should have been cascade-deleted, got %d", len(steps))
	}
	tail, _ := s.TailLogLines(ctx, job.ID, 10)
	if len(tail) != 0 {
		t.Errorf("log lines should have been cascade-deleted, got %d", len(tail))
	}
}
