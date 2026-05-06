package tools

import (
	"context"
	"os/exec"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Apprise wraps the apprise CLI for sending notifications.
//
// Per spec §8, Apprise failures NEVER fail the job. Run logs warn-level
// messages on each failure and returns nil to the caller.
type Apprise struct {
	bin string
}

// NewApprise returns an Apprise that runs <bin>. Empty defaults to
// "apprise" (resolved via PATH).
func NewApprise(bin string) *Apprise {
	if bin == "" {
		bin = "apprise"
	}
	return &Apprise{bin: bin}
}

func (a *Apprise) Name() string { return "apprise" }

// Run forwards args verbatim to the apprise CLI. Caller is responsible
// for constructing args (see BuildAppriseArgs). On exec error, logs
// warn and returns nil so the orchestrator's notify step still
// succeeds.
func (a *Apprise) Run(ctx context.Context, args []string, env map[string]string,
	workdir string, sink Sink) error {

	cmd := exec.CommandContext(ctx, a.bin, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	if len(env) > 0 {
		envSlice := cmd.Environ()
		for k, v := range env {
			envSlice = append(envSlice, k+"="+v)
		}
		cmd.Env = envSlice
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		sink.Log(state.LogLevelWarn, "apprise failed: %v: %s", err, string(out))
		return nil
	}
	sink.Log(state.LogLevelInfo, "apprise: notification sent")
	return nil
}

// BuildAppriseArgs constructs the argv for one apprise invocation.
// title and body land in -t / -b. tag (if non-empty) goes in --tag.
// urls follow as positional arguments.
func BuildAppriseArgs(title, body, tag string, urls []string) []string {
	args := []string{"-t", title, "-b", body}
	if tag != "" {
		args = append(args, "--tag", tag)
	}
	args = append(args, urls...)
	return args
}
