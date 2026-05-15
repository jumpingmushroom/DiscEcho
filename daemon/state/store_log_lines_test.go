package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestStore_LogLine_AppendAndTail(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	now := time.Now()
	for i := 0; i < 5; i++ {
		if err := s.AppendLogLine(ctx, state.LogLine{
			JobID: j.ID, T: now.Add(time.Duration(i) * time.Millisecond),
			Level: state.LogLevelInfo, Message: "line " + string(rune('a'+i)),
		}); err != nil {
			t.Fatal(err)
		}
	}

	tail, err := s.TailLogLines(ctx, j.ID, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(tail) != 3 {
		t.Fatalf("want 3, got %d", len(tail))
	}
	if tail[0].Message != "line c" || tail[2].Message != "line e" {
		t.Errorf("ordering wrong: %+v", tail)
	}
}

func TestStore_LogLine_AppendStoresStep(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	if err := s.AppendLogLine(ctx, state.LogLine{
		JobID: j.ID, T: time.Now(), Step: state.StepRip,
		Level: state.LogLevelInfo, Message: "first",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.TailLogLines(ctx, j.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Step != state.StepRip {
		t.Errorf("step not stored: %+v", got)
	}
}

func TestStore_LogLine_ListFiltersByStep(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	now := time.Now()
	lines := []state.LogLine{
		{JobID: j.ID, T: now, Step: state.StepRip, Level: state.LogLevelInfo, Message: "r1"},
		{JobID: j.ID, T: now.Add(time.Millisecond), Step: state.StepRip, Level: state.LogLevelInfo, Message: "r2"},
		{JobID: j.ID, T: now.Add(2 * time.Millisecond), Step: state.StepTranscode, Level: state.LogLevelInfo, Message: "t1"},
	}
	for _, l := range lines {
		if err := s.AppendLogLine(ctx, l); err != nil {
			t.Fatal(err)
		}
	}

	rip, total, err := s.ListLogLines(ctx, j.ID, state.LogFilter{Step: state.StepRip})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(rip) != 2 {
		t.Fatalf("rip filter: total=%d len=%d", total, len(rip))
	}
	if rip[0].Message != "r1" || rip[1].Message != "r2" {
		t.Errorf("rip order: %+v", rip)
	}

	all, allTotal, err := s.ListLogLines(ctx, j.ID, state.LogFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if allTotal != 3 || len(all) != 3 {
		t.Errorf("unfiltered: total=%d len=%d", allTotal, len(all))
	}
	if all[0].Message != "r1" || all[2].Message != "t1" {
		t.Errorf("order: %+v", all)
	}
}

func TestStore_LogLine_Tail_DefaultsTo200(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")
	prof := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	disc := newDisc(t, s, drv)
	j := newJob(t, s, drv, prof, disc)

	for i := 0; i < 250; i++ {
		_ = s.AppendLogLine(ctx, state.LogLine{
			JobID: j.ID, T: time.Now(), Level: state.LogLevelInfo, Message: "x",
		})
	}
	got, err := s.TailLogLines(ctx, j.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 200 {
		t.Errorf("want 200, got %d", len(got))
	}
}
