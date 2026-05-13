// Package jobs orchestrates job lifecycle: queueing, per-drive
// serialization, ctx cancellation, crash recovery, and the
// PersistentSink that bridges pipeline events into SQLite + the
// in-process event broadcaster.
package jobs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// progressInterval throttles OnProgress emits to at most one per
// interval per job. SSE clients see ≤1 progress event/sec which
// matches the design doc's spec.
const progressInterval = time.Second

// PersistentSink implements pipelines.EventSink. It writes events to
// the Store and broadcasts them via Broadcaster, coalescing
// high-frequency OnProgress emits.
type PersistentSink struct {
	store *state.Store
	bc    *state.Broadcaster
	jobID string

	mu              sync.Mutex
	lastProgressAt  time.Time
	pendingProgress *pendingProgress
}

type pendingProgress struct {
	step state.StepID
	pct  float64
	spd  string
	eta  int
}

// NewPersistentSink constructs a sink for one job.
func NewPersistentSink(store *state.Store, bc *state.Broadcaster, jobID string) *PersistentSink {
	return &PersistentSink{store: store, bc: bc, jobID: jobID}
}

// JobID returns the job this sink is bound to.
func (s *PersistentSink) JobID() string { return s.jobID }

// OnStepStart marks the step running, records it as the job's active
// step, resets the volatile progress fields so a stale 100% from the
// previous step doesn't linger on the UI, and broadcasts both the
// step transition and the reset progress. Persisting active_step here
// keeps the dashboard's pipeline stepper in sync even before any
// progress event fires (HandBrake/makemkvcon may take seconds before
// they emit anything parseable).
func (s *PersistentSink) OnStepStart(step state.StepID) {
	if err := s.store.UpdateJobStepState(context.Background(), s.jobID, step, state.JobStepStateRunning); err != nil {
		slog.Warn("PersistentSink: UpdateJobStepState running", "job", s.jobID, "step", step, "err", err)
	}
	if err := s.store.SetActiveStep(context.Background(), s.jobID, step); err != nil {
		slog.Warn("PersistentSink: SetActiveStep", "job", s.jobID, "step", step, "err", err)
	}
	// Reset progress/speed/eta so the previous step's terminal values
	// don't bleed into the new step's UI window. UpdateJobProgress also
	// re-asserts active_step; that's fine — it matches SetActiveStep.
	if err := s.store.UpdateJobProgress(context.Background(), s.jobID, step, 0, "", 0, 0); err != nil {
		slog.Warn("PersistentSink: reset progress on step start", "job", s.jobID, "step", step, "err", err)
	}
	s.mu.Lock()
	s.pendingProgress = nil
	s.lastProgressAt = time.Time{}
	s.mu.Unlock()
	s.bc.Publish(state.Event{
		Name: "job.step",
		Payload: map[string]any{
			"job_id": s.jobID,
			"step":   string(step),
			"state":  string(state.JobStepStateRunning),
		},
	})
	s.bc.Publish(state.Event{
		Name: "job.progress",
		Payload: map[string]any{
			"job_id":      s.jobID,
			"step":        string(step),
			"pct":         float64(0),
			"speed":       "",
			"eta_seconds": 0,
		},
	})
}

// OnProgress records progress with ≤1Hz coalescing.
func (s *PersistentSink) OnProgress(step state.StepID, pct float64, speed string, eta int) {
	s.mu.Lock()
	now := time.Now()
	since := now.Sub(s.lastProgressAt)
	s.pendingProgress = &pendingProgress{step: step, pct: pct, spd: speed, eta: eta}
	if since < progressInterval {
		s.mu.Unlock()
		return
	}
	emit := *s.pendingProgress
	s.pendingProgress = nil
	s.lastProgressAt = now
	s.mu.Unlock()

	s.flushProgress(emit)
}

// Flush forces any pending progress to be emitted. Called by the
// orchestrator after a step transitions out of running so the UI sees
// the final percentage even if the throttle window hadn't elapsed.
func (s *PersistentSink) Flush() {
	s.mu.Lock()
	pending := s.pendingProgress
	s.pendingProgress = nil
	s.lastProgressAt = time.Now()
	s.mu.Unlock()
	if pending != nil {
		s.flushProgress(*pending)
	}
}

func (s *PersistentSink) flushProgress(p pendingProgress) {
	if err := s.store.UpdateJobProgress(context.Background(), s.jobID, p.step, p.pct, p.spd, p.eta, 0); err != nil {
		slog.Warn("PersistentSink: UpdateJobProgress", "job", s.jobID, "err", err)
	}
	s.bc.Publish(state.Event{
		Name: "job.progress",
		Payload: map[string]any{
			"job_id":      s.jobID,
			"step":        string(p.step),
			"pct":         p.pct,
			"speed":       p.spd,
			"eta_seconds": p.eta,
		},
	})
}

// OnLog appends to the log_lines table and broadcasts.
func (s *PersistentSink) OnLog(level state.LogLevel, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	line := state.LogLine{
		JobID: s.jobID, T: time.Now(),
		Level: level, Message: msg,
	}
	if err := s.store.AppendLogLine(context.Background(), line); err != nil {
		slog.Warn("PersistentSink: AppendLogLine", "job", s.jobID, "err", err)
	}
	s.bc.Publish(state.Event{
		Name: "job.log",
		Payload: map[string]any{
			"job_id":  s.jobID,
			"t":       line.T.UTC().Format(time.RFC3339Nano),
			"level":   string(level),
			"message": msg,
		},
	})
}

// OnStepDone marks done, merges notes, broadcasts. Calls Flush first
// so any pending progress lands as 100% before "done".
func (s *PersistentSink) OnStepDone(step state.StepID, notes map[string]any) {
	s.Flush()
	if err := s.store.UpdateJobStepState(context.Background(), s.jobID, step, state.JobStepStateDone); err != nil {
		slog.Warn("PersistentSink: UpdateJobStepState done", "job", s.jobID, "step", step, "err", err)
	}
	if len(notes) > 0 {
		if err := s.store.AppendJobStepNotes(context.Background(), s.jobID, step, notes); err != nil {
			slog.Warn("PersistentSink: AppendJobStepNotes", "job", s.jobID, "step", step, "err", err)
		}
	}
	// On move-step completion, attribute the encoded output size to
	// the job row so the LIBRARY SIZE widget can sum across done jobs.
	// Pipelines record either `path: string` or `paths: []string` in the
	// move step's notes; either shape is supported.
	if step == state.StepMove {
		if b := sumMovePathBytes(notes); b > 0 {
			if err := s.store.RecordOutputBytes(context.Background(), s.jobID, b); err != nil {
				slog.Warn("PersistentSink: RecordOutputBytes", "job", s.jobID, "err", err)
			}
		}
	}
	payload := map[string]any{
		"job_id": s.jobID,
		"step":   string(step),
		"state":  string(state.JobStepStateDone),
	}
	if len(notes) > 0 {
		payload["notes"] = notes
	}
	s.bc.Publish(state.Event{Name: "job.step", Payload: payload})
}

// OnStepFailed marks the step failed and broadcasts.
func (s *PersistentSink) OnStepFailed(step state.StepID, err error) {
	if uerr := s.store.UpdateJobStepState(context.Background(), s.jobID, step, state.JobStepStateFailed); uerr != nil {
		slog.Warn("PersistentSink: UpdateJobStepState failed", "job", s.jobID, "step", step, "err", uerr)
	}
	s.bc.Publish(state.Event{
		Name: "job.step",
		Payload: map[string]any{
			"job_id": s.jobID,
			"step":   string(step),
			"state":  string(state.JobStepStateFailed),
			"error":  err.Error(),
		},
	})
}

// sumMovePathBytes walks the StepMove notes' path / paths field and
// returns the total bytes of the referenced files. Missing files are
// skipped silently. Returns 0 when notes don't carry path data.
func sumMovePathBytes(notes map[string]any) int64 {
	if notes == nil {
		return 0
	}
	var paths []string
	if p, ok := notes["path"].(string); ok && p != "" {
		paths = append(paths, p)
	}
	if ps, ok := notes["paths"].([]string); ok {
		paths = append(paths, ps...)
	}
	// Some pipelines stash []interface{} via map[string]any literal — handle that too.
	if pa, ok := notes["paths"].([]any); ok {
		for _, v := range pa {
			if s, ok := v.(string); ok {
				paths = append(paths, s)
			}
		}
	}
	var total int64
	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			continue
		}
		total += fi.Size()
	}
	return total
}
