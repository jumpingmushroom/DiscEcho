package pipelines

import (
	"context"
	"errors"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type stepEvent struct {
	kind  string // "start" | "done" | "failed"
	step  state.StepID
	notes map[string]any
	err   error
}

type stepRecorder struct{ events []stepEvent }

func (r *stepRecorder) OnStepStart(s state.StepID) {
	r.events = append(r.events, stepEvent{kind: "start", step: s})
}
func (r *stepRecorder) OnProgress(state.StepID, float64, string, int) {}
func (r *stepRecorder) OnLog(state.LogLevel, string, ...any)          {}
func (r *stepRecorder) OnSubStep(string)                              {}
func (r *stepRecorder) OnStepDone(s state.StepID, notes map[string]any) {
	r.events = append(r.events, stepEvent{kind: "done", step: s, notes: notes})
}
func (r *stepRecorder) OnStepFailed(s state.StepID, err error) {
	r.events = append(r.events, stepEvent{kind: "failed", step: s, err: err})
}
func (r *stepRecorder) JobID() string { return "" }

type recordingTool struct {
	name      string
	runArgs   []string
	runErr    error
	runCalled bool
}

func (t *recordingTool) Name() string { return t.name }
func (t *recordingTool) Run(_ context.Context, args []string, _ map[string]string, _ string, _ tools.Sink) error {
	t.runCalled = true
	t.runArgs = args
	return t.runErr
}

func TestRunNotifyStep_NoToolsRegistryIsNoOp(t *testing.T) {
	rec := &stepRecorder{}
	RunNotifyStep(context.Background(), rec, NotifyDeps{}, &state.Disc{Title: "X"})

	if len(rec.events) != 2 || rec.events[0].kind != "start" || rec.events[1].kind != "done" {
		t.Fatalf("want start+done only; got %+v", rec.events)
	}
}

func TestRunNotifyStep_FiresAppriseWithExpectedArgs(t *testing.T) {
	reg := tools.NewRegistry()
	apprise := &recordingTool{name: "apprise"}
	reg.Register(apprise)

	rec := &stepRecorder{}
	deps := NotifyDeps{
		Tools:       reg,
		LibraryRoot: "/lib",
		URLsForTrigger: func(_ context.Context, trigger string) []string {
			if trigger != "done" {
				t.Errorf("want trigger=done, got %q", trigger)
			}
			return []string{"discord://x", "slack://y"}
		},
	}

	RunNotifyStep(context.Background(), rec, deps, &state.Disc{Title: "Test Disc"})

	if !apprise.runCalled {
		t.Fatalf("apprise.Run not called")
	}
	wantArgs := tools.BuildAppriseArgs("DiscEcho: Test Disc", "Ripped to /lib", "", []string{"discord://x", "slack://y"})
	if len(apprise.runArgs) != len(wantArgs) {
		t.Fatalf("argv length: want %d got %d (%v)", len(wantArgs), len(apprise.runArgs), apprise.runArgs)
	}
	for i := range wantArgs {
		if apprise.runArgs[i] != wantArgs[i] {
			t.Errorf("argv[%d]: want %q got %q", i, wantArgs[i], apprise.runArgs[i])
		}
	}
}

func TestRunNotifyStep_ToolErrorSwallowedStillEmitsDone(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&recordingTool{name: "apprise", runErr: errors.New("boom")})

	rec := &stepRecorder{}
	RunNotifyStep(context.Background(), rec, NotifyDeps{Tools: reg}, &state.Disc{Title: "X"})

	if rec.events[len(rec.events)-1].kind != "done" {
		t.Fatalf("want trailing done, got %+v", rec.events)
	}
}

func TestRunEjectStep_NilToolsIsNoOp(t *testing.T) {
	rec := &stepRecorder{}
	RunEjectStep(context.Background(), rec, EjectDeps{}, &state.Drive{DevPath: "/dev/sr0"})

	if len(rec.events) != 2 {
		t.Fatalf("want start+done only; got %+v", rec.events)
	}
}

func TestRunEjectStep_ShouldEjectFalseSkipsTool(t *testing.T) {
	reg := tools.NewRegistry()
	eject := &recordingTool{name: "eject"}
	reg.Register(eject)

	rec := &stepRecorder{}
	deps := EjectDeps{
		Tools:       reg,
		ShouldEject: func(context.Context) bool { return false },
	}
	RunEjectStep(context.Background(), rec, deps, &state.Drive{DevPath: "/dev/sr0"})

	if eject.runCalled {
		t.Errorf("eject ran despite ShouldEject=false")
	}
}

func TestRunEjectStep_FiresEjectWithDevPath(t *testing.T) {
	reg := tools.NewRegistry()
	eject := &recordingTool{name: "eject"}
	reg.Register(eject)

	rec := &stepRecorder{}
	RunEjectStep(context.Background(), rec, EjectDeps{Tools: reg}, &state.Drive{DevPath: "/dev/sr0"})

	if !eject.runCalled {
		t.Fatalf("eject.Run not called")
	}
	if len(eject.runArgs) != 1 || eject.runArgs[0] != "/dev/sr0" {
		t.Errorf("argv: want [/dev/sr0], got %v", eject.runArgs)
	}
}

func TestRunEjectStep_ToolErrorReportedAsFailed(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&recordingTool{name: "eject", runErr: errors.New("stuck tray")})

	rec := &stepRecorder{}
	RunEjectStep(context.Background(), rec, EjectDeps{Tools: reg}, &state.Drive{DevPath: "/dev/sr0"})

	var sawFailed bool
	for _, e := range rec.events {
		if e.kind == "failed" {
			sawFailed = true
			if e.step != state.StepEject || e.err == nil {
				t.Errorf("bad failed event: %+v", e)
			}
		}
	}
	if !sawFailed {
		t.Errorf("want a failed event; got %+v", rec.events)
	}
	// Done still fires after failed — eject failure does not abort the job.
	if rec.events[len(rec.events)-1].kind != "done" {
		t.Errorf("want trailing done after failed; got %+v", rec.events)
	}
}

func TestRunEjectStep_NilDriveIsNoOp(t *testing.T) {
	reg := tools.NewRegistry()
	eject := &recordingTool{name: "eject"}
	reg.Register(eject)

	rec := &stepRecorder{}
	RunEjectStep(context.Background(), rec, EjectDeps{Tools: reg}, nil)

	if eject.runCalled {
		t.Errorf("eject ran with nil drive")
	}
}
