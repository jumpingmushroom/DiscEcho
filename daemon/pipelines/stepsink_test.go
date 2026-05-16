package pipelines

import (
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

type recordingSink struct {
	progress []struct {
		step    state.StepID
		pct     float64
		speed   string
		etaSecs int
	}
	logs []struct {
		level  state.LogLevel
		format string
		args   []any
	}
}

func (r *recordingSink) OnStepStart(state.StepID) {}
func (r *recordingSink) OnProgress(step state.StepID, pct float64, speed string, etaSeconds int) {
	r.progress = append(r.progress, struct {
		step    state.StepID
		pct     float64
		speed   string
		etaSecs int
	}{step, pct, speed, etaSeconds})
}
func (r *recordingSink) OnLog(level state.LogLevel, format string, args ...any) {
	r.logs = append(r.logs, struct {
		level  state.LogLevel
		format string
		args   []any
	}{level, format, args})
}
func (r *recordingSink) OnStepDone(state.StepID, map[string]any) {}
func (r *recordingSink) OnStepFailed(state.StepID, error)        {}
func (r *recordingSink) JobID() string                           { return "" }

func TestStepSink_AttributesProgressToBoundStep(t *testing.T) {
	rs := &recordingSink{}
	ss := NewStepSink(rs, state.StepCompress)

	ss.Progress(42.5, "10x", 30)

	if len(rs.progress) != 1 {
		t.Fatalf("want 1 progress event, got %d", len(rs.progress))
	}
	got := rs.progress[0]
	if got.step != state.StepCompress {
		t.Errorf("step: want %q, got %q", state.StepCompress, got.step)
	}
	if got.pct != 42.5 || got.speed != "10x" || got.etaSecs != 30 {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestStepSink_LogPassesThroughUnchanged(t *testing.T) {
	rs := &recordingSink{}
	ss := NewStepSink(rs, state.StepRip)

	ss.Log(state.LogLevelWarn, "hello %s", "world")

	if len(rs.logs) != 1 {
		t.Fatalf("want 1 log event, got %d", len(rs.logs))
	}
	got := rs.logs[0]
	if got.level != state.LogLevelWarn || got.format != "hello %s" {
		t.Errorf("log header mismatch: %+v", got)
	}
	if len(got.args) != 1 || got.args[0] != "world" {
		t.Errorf("log args mismatch: %v", got.args)
	}
}
