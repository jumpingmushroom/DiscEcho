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
