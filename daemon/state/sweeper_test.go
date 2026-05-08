package state_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

type fakeReader struct {
	forever bool
	days    int
}

func (f *fakeReader) GetBool(_ context.Context, key string) (bool, error) {
	if key == "retention.forever" {
		return f.forever, nil
	}
	return false, nil
}

func (f *fakeReader) GetInt(_ context.Context, key string) (int, error) {
	if key == "retention.days" {
		return f.days, nil
	}
	return 0, nil
}

func TestSweeper_Tick_NoOpWhenForever(t *testing.T) {
	s := openStore(t)
	sw := &state.Sweeper{
		Store:    s,
		Settings: &fakeReader{forever: true},
		Now:      time.Now,
		Logger:   slog.Default(),
	}
	sw.Tick(context.Background())
}

func TestSweeper_Tick_NoOpWhenDaysZero(t *testing.T) {
	s := openStore(t)
	sw := &state.Sweeper{
		Store:    s,
		Settings: &fakeReader{forever: false, days: 0},
		Now:      time.Now,
		Logger:   slog.Default(),
	}
	sw.Tick(context.Background())
}

func TestSweeper_Tick_DeletesOldJobs(t *testing.T) {
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

	sw := &state.Sweeper{
		Store:    s,
		Settings: &fakeReader{forever: false, days: 30},
		Now:      time.Now,
		Logger:   slog.Default(),
	}
	sw.Tick(ctx)

	if _, err := s.GetJob(ctx, old.ID); err == nil {
		t.Fatal("old job should be deleted")
	}
}

func TestSweeper_NextThreeAM_BeforeThree(t *testing.T) {
	now := time.Date(2026, 5, 8, 1, 0, 0, 0, time.UTC)
	got := state.NextThreeAM(now)
	want := time.Date(2026, 5, 8, 3, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSweeper_NextThreeAM_AfterThree(t *testing.T) {
	now := time.Date(2026, 5, 8, 5, 0, 0, 0, time.UTC)
	got := state.NextThreeAM(now)
	want := time.Date(2026, 5, 9, 3, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSweeper_NextThreeAM_ExactlyAtThree(t *testing.T) {
	// At exactly 03:00, "next" should be the following day.
	now := time.Date(2026, 5, 8, 3, 0, 0, 0, time.UTC)
	got := state.NextThreeAM(now)
	want := time.Date(2026, 5, 9, 3, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
