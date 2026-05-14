package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestStore_ListHistory_FilterByType(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	for i := 0; i < 2; i++ {
		d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
		_ = s.CreateDisc(ctx, d)
		j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
		_ = s.CreateJob(ctx, j)
		_ = s.UpdateJobState(ctx, j.ID, state.JobStateDone, "")
	}
	dvd := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeDVD, Title: "x"}
	_ = s.CreateDisc(ctx, dvd)
	j := &state.Job{DiscID: dvd.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, j)
	_ = s.UpdateJobState(ctx, j.ID, state.JobStateDone, "")

	rows, err := s.ListHistory(ctx, state.HistoryFilter{Type: state.DiscTypeDVD})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Errorf("want 1 dvd row, got %d", len(rows))
	}
}

func TestStore_ListHistory_FilterByDateRange(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	d := newDisc(t, s, drv)

	j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, j)
	_ = s.UpdateJobState(ctx, j.ID, state.JobStateDone, "")

	from := time.Now().Add(time.Hour)
	rows, err := s.ListHistory(ctx, state.HistoryFilter{From: from})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0, got %d", len(rows))
	}

	from = time.Now().Add(-time.Hour)
	rows, _ = s.ListHistory(ctx, state.HistoryFilter{From: from})
	if len(rows) != 1 {
		t.Errorf("want 1, got %d", len(rows))
	}
}

func TestStore_ListHistory_OrdersByFinishedAtDESC(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	for i := 0; i < 3; i++ {
		d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
		_ = s.CreateDisc(ctx, d)
		j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
		_ = s.CreateJob(ctx, j)
		_ = s.UpdateJobState(ctx, j.ID, state.JobStateDone, "")
		time.Sleep(2 * time.Millisecond)
	}

	rows, err := s.ListHistory(ctx, state.HistoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3, got %d", len(rows))
	}
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Job.FinishedAt == nil || rows[i].Job.FinishedAt == nil {
			t.Fatal("FinishedAt nil")
		}
		if rows[i-1].Job.FinishedAt.Before(*rows[i].Job.FinishedAt) {
			t.Errorf("not DESC at index %d", i)
		}
	}
}

func TestStore_ListHistory_LimitOffset(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	for i := 0; i < 5; i++ {
		d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
		_ = s.CreateDisc(ctx, d)
		j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
		_ = s.CreateJob(ctx, j)
		_ = s.UpdateJobState(ctx, j.ID, state.JobStateDone, "")
	}

	rows, _ := s.ListHistory(ctx, state.HistoryFilter{Limit: 2, Offset: 2})
	if len(rows) != 2 {
		t.Errorf("want 2, got %d", len(rows))
	}
}

func TestStore_CountHistory(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	for i := 0; i < 4; i++ {
		d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
		_ = s.CreateDisc(ctx, d)
		j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
		_ = s.CreateJob(ctx, j)
		_ = s.UpdateJobState(ctx, j.ID, state.JobStateDone, "")
	}

	n, err := s.CountHistory(ctx, state.HistoryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("want 4, got %d", n)
	}
}

func TestStore_ListHistory_ExcludesActiveJobs(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	d := newDisc(t, s, drv)
	j := &state.Job{DiscID: d.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, j)

	rows, _ := s.ListHistory(ctx, state.HistoryFilter{})
	if len(rows) != 0 {
		t.Errorf("running jobs should not appear in history, got %d rows", len(rows))
	}
}

func TestStore_ClearHistory(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	// A finished (done) rip.
	discDone := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "done"}
	_ = s.CreateDisc(ctx, discDone)
	jobDone := &state.Job{DiscID: discDone.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, jobDone)
	_ = s.UpdateJobState(ctx, jobDone.ID, state.JobStateDone, "")

	// A finished (failed) rip.
	discFailed := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "failed"}
	_ = s.CreateDisc(ctx, discFailed)
	jobFailed := &state.Job{DiscID: discFailed.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, jobFailed)
	_ = s.UpdateJobState(ctx, jobFailed.ID, state.JobStateFailed, "boom")

	// An in-progress rip (job left in the default queued state).
	discActive := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "active"}
	_ = s.CreateDisc(ctx, discActive)
	jobActive := &state.Job{DiscID: discActive.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, jobActive)

	// A disc still awaiting a decision — no job at all.
	discAwaiting := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "awaiting"}
	_ = s.CreateDisc(ctx, discAwaiting)

	// A re-rip: one finished job AND one active job on the same disc.
	discRerip := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "rerip"}
	_ = s.CreateDisc(ctx, discRerip)
	jobReripOld := &state.Job{DiscID: discRerip.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, jobReripOld)
	_ = s.UpdateJobState(ctx, jobReripOld.ID, state.JobStateDone, "")
	jobReripNew := &state.Job{DiscID: discRerip.ID, DriveID: drv.ID, ProfileID: prof.ID}
	_ = s.CreateJob(ctx, jobReripNew)

	n, err := s.ClearHistory(ctx)
	if err != nil {
		t.Fatalf("ClearHistory: %v", err)
	}
	// jobDone + jobFailed + jobReripOld = 3 finished jobs deleted.
	if n != 3 {
		t.Errorf("deleted count: want 3, got %d", n)
	}

	// Finished history is gone.
	if cnt, _ := s.CountHistory(ctx, state.HistoryFilter{}); cnt != 0 {
		t.Errorf("CountHistory after clear: want 0, got %d", cnt)
	}
	// Discs whose only jobs were finished are gone.
	if _, err := s.GetDisc(ctx, discDone.ID); err == nil {
		t.Errorf("discDone should have been deleted")
	}
	if _, err := s.GetDisc(ctx, discFailed.ID); err == nil {
		t.Errorf("discFailed should have been deleted")
	}
	// In-progress rip and its disc survive.
	if _, err := s.GetDisc(ctx, discActive.ID); err != nil {
		t.Errorf("discActive should remain: %v", err)
	}
	if _, err := s.GetJob(ctx, jobActive.ID); err != nil {
		t.Errorf("jobActive should remain: %v", err)
	}
	// Awaiting-decision disc (never had a job) is left alone.
	if _, err := s.GetDisc(ctx, discAwaiting.ID); err != nil {
		t.Errorf("discAwaiting should remain: %v", err)
	}
	// Re-rip disc kept; its old finished job cleared, its active job kept.
	if _, err := s.GetDisc(ctx, discRerip.ID); err != nil {
		t.Errorf("discRerip should remain: %v", err)
	}
	if _, err := s.GetJob(ctx, jobReripOld.ID); err == nil {
		t.Errorf("jobReripOld (finished) should have been deleted")
	}
	if _, err := s.GetJob(ctx, jobReripNew.ID); err != nil {
		t.Errorf("jobReripNew (active) should remain: %v", err)
	}

	// Second call is a no-op and returns 0.
	n2, err := s.ClearHistory(ctx)
	if err != nil {
		t.Fatalf("ClearHistory (second call): %v", err)
	}
	if n2 != 0 {
		t.Errorf("second ClearHistory: want 0, got %d", n2)
	}
}
