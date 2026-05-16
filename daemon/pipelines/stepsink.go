package pipelines

import "github.com/jumpingmushroom/DiscEcho/daemon/state"

// StepSink adapts an EventSink so a tool's per-step Sink calls land
// against a fixed step ID. Each handler's rip/compress/transcode/move/
// notify/eject call wraps the parent sink in one of these before
// passing it to a tool. The shape (Progress + Log) matches tools.Sink
// structurally so *StepSink satisfies that interface without a Go
// import cycle (pipelines -> tools).
type StepSink struct {
	sink EventSink
	step state.StepID
}

// NewStepSink binds sink to step so subsequent Progress/Log calls
// reattribute back to that step on the parent EventSink.
func NewStepSink(sink EventSink, step state.StepID) *StepSink {
	return &StepSink{sink: sink, step: step}
}

// Progress forwards a tool's progress update to the parent sink,
// tagged with the bound step ID.
func (s *StepSink) Progress(pct float64, speed string, etaSeconds int) {
	s.sink.OnProgress(s.step, pct, speed, etaSeconds)
}

// Log forwards a tool's log line to the parent sink unchanged — log
// lines are step-agnostic at the sink layer.
func (s *StepSink) Log(level state.LogLevel, format string, args ...any) {
	s.sink.OnLog(level, format, args...)
}
