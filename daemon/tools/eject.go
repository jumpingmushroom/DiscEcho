package tools

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Eject wraps the system `eject` binary. Unlike Apprise, eject failure
// IS surfaced as an error so the eject step can be marked failed; the
// orchestrator decides not to fail the JOB on top of that.
type Eject struct {
	bin string
}

// NewEject returns an Eject that runs <bin>. Empty defaults to "eject".
func NewEject(bin string) *Eject {
	if bin == "" {
		bin = "eject"
	}
	return &Eject{bin: bin}
}

func (e *Eject) Name() string { return "eject" }

// Run executes `<bin> <devPath>`. Sink gets a log line on success or
// failure; the error itself is also returned so the orchestrator can
// mark the step's state.
func (e *Eject) Run(ctx context.Context, args []string, _ map[string]string,
	_ string, sink Sink) error {

	cmd := exec.CommandContext(ctx, e.bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		sink.Log(state.LogLevelWarn, "eject failed: %v: %s", err, string(out))
		return fmt.Errorf("eject: %w", err)
	}
	sink.Log(state.LogLevelInfo, "eject: tray released")
	return nil
}
