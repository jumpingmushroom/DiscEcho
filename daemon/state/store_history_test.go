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
