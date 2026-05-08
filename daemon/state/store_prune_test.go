package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// backdateJobFinishedAt overwrites finished_at via raw SQL so we can test
// cutoff comparisons without coupling to UpdateJobState's "now" logic.
func backdateJobFinishedAt(t *testing.T, s *state.Store, jobID string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	_, err := s.DB().Conn().ExecContext(ctx,
		`UPDATE jobs SET finished_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339Nano), jobID)
	if err != nil {
		t.Fatalf("backdate finished_at: %v", err)
	}
}

func TestStore_PruneHistoryBefore_DeletesOldDoneJobs(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "p", state.DiscTypeAudioCD)

	disc := newDisc(t, s, drv)
	old := newJob(t, s, drv, prof, disc)
	if err := s.UpdateJobState(ctx, old.ID, state.JobStateDone, ""); err != nil {
		t.Fatalf("UpdateJobState: %v", err)
	}
	backdateJobFinishedAt(t, s, old.ID, time.Now().AddDate(0, 0, -100))

	disc2 := newDisc(t, s, drv)
	fresh := newJob(t, s, drv, prof, disc2)
	if err := s.UpdateJobState(ctx, fresh.ID, state.JobStateDone, ""); err != nil {
		t.Fatalf("UpdateJobState: %v", err)
	}
	backdateJobFinishedAt(t, s, fresh.ID, time.Now().AddDate(0, 0, -1))

	cutoff := time.Now().AddDate(0, 0, -30)
	n, err := s.PruneHistoryBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 deletion; got %d", n)
	}
	if _, err := s.GetJob(ctx, old.ID); err == nil {
		t.Fatal("old job should be deleted")
	}
	if _, err := s.GetJob(ctx, fresh.ID); err != nil {
		t.Fatalf("fresh job should be preserved: %v", err)
	}
	// Orphaned disc deleted, referenced disc preserved.
	if _, err := s.GetDisc(ctx, disc.ID); err == nil {
		t.Fatal("orphaned disc should be deleted")
	}
	if _, err := s.GetDisc(ctx, disc2.ID); err != nil {
		t.Fatalf("disc2 still referenced; should remain: %v", err)
	}
}

func TestStore_PruneHistoryBefore_KeepsRunningJobs(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "p", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)

	running := newJob(t, s, drv, prof, disc)
	if err := s.UpdateJobState(ctx, running.ID, state.JobStateRunning, ""); err != nil {
		t.Fatalf("UpdateJobState: %v", err)
	}
	// Backdate started_at to simulate a long-running job; finished_at remains NULL.
	_, _ = s.DB().Conn().ExecContext(ctx,
		`UPDATE jobs SET started_at = ? WHERE id = ?`,
		time.Now().AddDate(0, 0, -100).UTC().Format(time.RFC3339Nano), running.ID)

	cutoff := time.Now().AddDate(0, 0, -30)
	n, err := s.PruneHistoryBefore(ctx, cutoff)
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 0 {
		t.Fatalf("running jobs should not be pruned; got %d deletions", n)
	}
}

func TestStore_PruneHistoryBefore_NothingToPrune(t *testing.T) {
	s := openStore(t)
	cutoff := time.Now().AddDate(0, 0, -30)
	n, err := s.PruneHistoryBefore(context.Background(), cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0; got %d", n)
	}
}
