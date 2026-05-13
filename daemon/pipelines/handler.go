// Package pipelines defines the per-disc-type Handler contract that
// the orchestrator dispatches to. Handlers are pure Go code per disc
// type (audio CD, DVD, BDMV, ...) and compose the tools/* wrappers to
// implement the canonical 8-step pipeline.
package pipelines

import (
	"context"
	"errors"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ErrNoCandidates is returned by Handler.Identify when MB returns no
// matches (or the per-type lookup otherwise has nothing to offer).
// The orchestrator turns this into a job state of "failed" with an
// explanatory error_message.
var ErrNoCandidates = errors.New("pipelines: no candidates")

// Handler implements one disc type's pipeline.
type Handler interface {
	DiscType() state.DiscType

	// Identify probes the disc and returns base info + candidates.
	// ErrNoCandidates surfaces 0-match cases.
	Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error)

	// Plan returns the ordered step list for UI rendering / DB
	// persistence. Includes skipped steps so the stepper renders all
	// 8 canonical positions. No execution happens here.
	Plan(disc *state.Disc, profile *state.Profile) []StepPlan

	// Run executes the pipeline. drv supplies the dev_path for eject
	// and any other drive-scoped operations. The handler owns its
	// temp dir, moves outputs, fires Apprise, ejects.
	Run(ctx context.Context, drv *state.Drive, disc *state.Disc, profile *state.Profile, sink EventSink) error
}

// StepPlan is one canonical-step descriptor used at job-creation time
// to materialize the job_steps rows.
type StepPlan struct {
	ID   state.StepID
	Skip bool
}

// EventSink receives every event a Handler emits during Run.
// JobID identifies the job the sink is bound to — pipelines use it to
// attribute final output sizes back onto the right job row.
type EventSink interface {
	OnStepStart(stepID state.StepID)
	OnProgress(stepID state.StepID, pct float64, speed string, etaSeconds int)
	OnLog(level state.LogLevel, format string, args ...any)
	OnStepDone(stepID state.StepID, notes map[string]any)
	OnStepFailed(stepID state.StepID, err error)
	JobID() string
}
