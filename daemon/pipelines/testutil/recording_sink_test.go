package testutil_test

import (
	"errors"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestRecordingSink_RecordsAllKinds(t *testing.T) {
	s := testutil.NewRecordingSink()

	s.OnStepStart(state.StepRip)
	s.OnProgress(state.StepRip, 50, "5×", 30)
	s.OnLog(state.LogLevelInfo, "hello %s", "world")
	s.OnStepDone(state.StepRip, map[string]any{"x": 1})
	s.OnStepFailed(state.StepNotify, errors.New("boom"))

	got := s.Snapshot()
	if len(got) != 5 {
		t.Fatalf("want 5 events, got %d", len(got))
	}
	if got[0].Kind != testutil.EventStart || got[0].Step != state.StepRip {
		t.Errorf("event[0] mismatch: %+v", got[0])
	}
	if got[2].Message != "hello world" {
		t.Errorf("event[2] message: %q", got[2].Message)
	}
	if got[4].Err == nil {
		t.Errorf("event[4] err missing")
	}
}

func TestRecordingSink_StepSequence(t *testing.T) {
	s := testutil.NewRecordingSink()
	s.OnStepStart(state.StepDetect)
	s.OnProgress(state.StepDetect, 100, "", 0)
	s.OnStepStart(state.StepIdentify)
	s.OnStepStart(state.StepRip)

	seq := s.StepSequence()
	if len(seq) != 3 {
		t.Fatalf("want 3, got %d", len(seq))
	}
	if seq[0] != state.StepDetect || seq[2] != state.StepRip {
		t.Errorf("sequence: %v", seq)
	}
}
