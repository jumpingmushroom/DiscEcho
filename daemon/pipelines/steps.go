package pipelines

import (
	"context"
	"fmt"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// NotifyDeps groups everything RunNotifyStep needs out of a handler's
// Deps struct. Nil Tools or a missing apprise registration is a silent
// no-op — same shape every handler used to duplicate inline.
type NotifyDeps struct {
	Tools          *tools.Registry
	URLsForTrigger func(ctx context.Context, trigger string) []string
	LibraryRoot    string
}

// RunNotifyStep emits the canonical notify step: looks up the apprise
// tool, builds the "DiscEcho: <title> — Ripped to <root>" message
// against the URLs that subscribe to the "done" trigger, and fires
// best-effort. Sink lifecycle events (start, done) bracket the call so
// callers don't have to.
func RunNotifyStep(ctx context.Context, sink EventSink, deps NotifyDeps, disc *state.Disc) {
	sink.OnStepStart(state.StepNotify)
	defer sink.OnStepDone(state.StepNotify, nil)

	if deps.Tools == nil {
		return
	}
	apprise, ok := deps.Tools.Get("apprise")
	if !ok {
		return
	}
	var urls []string
	if deps.URLsForTrigger != nil {
		urls = deps.URLsForTrigger(ctx, "done")
	}
	title := fmt.Sprintf("DiscEcho: %s", disc.Title)
	body := fmt.Sprintf("Ripped to %s", deps.LibraryRoot)
	argv := tools.BuildAppriseArgs(title, body, "", urls)
	_ = apprise.Run(ctx, argv, nil, "", NewStepSink(sink, state.StepNotify))
}

// EjectDeps groups everything RunEjectStep needs out of a handler's
// Deps struct. ShouldEject == nil falls back to "always eject" via
// ResolveShouldEject; nil Tools or missing eject registration is a
// silent no-op.
type EjectDeps struct {
	Tools       *tools.Registry
	ShouldEject func(ctx context.Context) bool
}

// RunEjectStep emits the canonical eject step. Failure of the underlying
// tool is reported via sink.OnStepFailed (the dashboard surfaces this)
// but never returns an error — the bits are already in the library and
// failing the whole job on a stuck tray would be wrong. Sink lifecycle
// events (start, done) bracket the call.
func RunEjectStep(ctx context.Context, sink EventSink, deps EjectDeps, drv *state.Drive) {
	sink.OnStepStart(state.StepEject)
	defer sink.OnStepDone(state.StepEject, nil)

	if deps.Tools == nil || drv == nil || drv.DevPath == "" {
		return
	}
	if !ResolveShouldEject(ctx, deps.ShouldEject) {
		return
	}
	eject, ok := deps.Tools.Get("eject")
	if !ok {
		return
	}
	if err := eject.Run(ctx, []string{drv.DevPath}, nil, "", NewStepSink(sink, state.StepEject)); err != nil {
		sink.OnStepFailed(state.StepEject, err)
	}
}
