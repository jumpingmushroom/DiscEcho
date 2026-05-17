// Package testutil exposes the RecordingSink used by handler and
// orchestrator tests. It implements pipelines.EventSink by appending
// every call into a slice for later assertions.
package testutil

import (
	"fmt"
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// EventKind discriminates between the recorded event types.
type EventKind string

const (
	EventStart    EventKind = "step.start"
	EventProgress EventKind = "step.progress"
	EventLog      EventKind = "log"
	EventDone     EventKind = "step.done"
	EventFailed   EventKind = "step.failed"
	EventSubStep  EventKind = "step.substep"
)

// RecordedEvent is one captured Sink call.
type RecordedEvent struct {
	Kind       EventKind
	Step       state.StepID
	Pct        float64
	Speed      string
	ETASeconds int
	Level      state.LogLevel
	Message    string
	Notes      map[string]any
	Err        error
	SubStep    string
}

// RecordingSink implements pipelines.EventSink and records every call.
// JobIDValue is what JobID() returns; default empty string is fine for
// tests that don't care which job ID the pipeline is operating on.
type RecordingSink struct {
	mu         sync.Mutex
	Events     []RecordedEvent
	JobIDValue string
}

// NewRecordingSink returns an empty sink.
func NewRecordingSink() *RecordingSink {
	return &RecordingSink{}
}

func (r *RecordingSink) append(e RecordedEvent) {
	r.mu.Lock()
	r.Events = append(r.Events, e)
	r.mu.Unlock()
}

// OnStepStart records a step.start event.
func (r *RecordingSink) OnStepStart(s state.StepID) {
	r.append(RecordedEvent{Kind: EventStart, Step: s})
}

// OnProgress records a step.progress event.
func (r *RecordingSink) OnProgress(s state.StepID, pct float64, speed string, eta int) {
	r.append(RecordedEvent{Kind: EventProgress, Step: s, Pct: pct, Speed: speed, ETASeconds: eta})
}

// OnLog records a log event. The format string and args are formatted
// via fmt.Sprintf so callers can assert on the rendered Message.
func (r *RecordingSink) OnLog(level state.LogLevel, format string, args ...any) {
	r.append(RecordedEvent{Kind: EventLog, Level: level, Message: fmt.Sprintf(format, args...)})
}

// OnSubStep records a sub-step transition. Empty name signals the
// pipeline is clearing the current sub-step; both are recorded so
// callers can assert on the full transition sequence.
func (r *RecordingSink) OnSubStep(name string) {
	r.append(RecordedEvent{Kind: EventSubStep, SubStep: name})
}

// OnStepDone records a step.done event with optional notes.
func (r *RecordingSink) OnStepDone(s state.StepID, notes map[string]any) {
	r.append(RecordedEvent{Kind: EventDone, Step: s, Notes: notes})
}

// OnStepFailed records a step.failed event.
func (r *RecordingSink) OnStepFailed(s state.StepID, err error) {
	r.append(RecordedEvent{Kind: EventFailed, Step: s, Err: err})
}

// JobID satisfies the EventSink contract. Returns JobIDValue (default "").
func (r *RecordingSink) JobID() string {
	return r.JobIDValue
}

// Snapshot returns a copy of Events for thread-safe inspection.
func (r *RecordingSink) Snapshot() []RecordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]RecordedEvent, len(r.Events))
	copy(out, r.Events)
	return out
}

// StepSequence returns the ordered list of step IDs the sink saw start.
// Useful for asserting "detect → identify → rip → ..." order.
func (r *RecordingSink) StepSequence() []state.StepID {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []state.StepID
	for _, e := range r.Events {
		if e.Kind == EventStart {
			out = append(out, e.Step)
		}
	}
	return out
}
