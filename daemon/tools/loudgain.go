package tools

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Loudgain wraps the `loudgain` CLI (https://github.com/Moonbase59/loudgain),
// an EBU R128 ReplayGain 2.0 tagger. Used by the audio-CD pipeline to
// write per-track + album-mode ReplayGain tags onto the ripped FLACs
// after whipper finishes. Built from source in the Dockerfile because
// it is not in Debian bookworm.
type Loudgain struct {
	bin string
}

// NewLoudgain returns a Loudgain that runs <bin>. Empty defaults to
// "loudgain" (resolved via PATH).
func NewLoudgain(bin string) *Loudgain {
	if bin == "" {
		bin = "loudgain"
	}
	return &Loudgain{bin: bin}
}

func (l *Loudgain) Name() string { return "loudgain" }

// Run implements the generic Tool interface so the registry can expose
// loudgain to non-pipeline callers. The audio-CD pipeline uses
// TagAlbum directly.
func (l *Loudgain) Run(ctx context.Context, args []string, _ map[string]string,
	_ string, sink Sink) error {
	cmd := exec.CommandContext(ctx, l.bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		sink.Log(state.LogLevelWarn, "loudgain failed: %v: %s", err, string(out))
		return fmt.Errorf("loudgain: %w", err)
	}
	return nil
}

// TagAlbum writes per-track + album-mode ReplayGain 2.0 tags onto the
// supplied FLAC files. Album-mode requires all album tracks to be
// passed in one invocation so the album-gain value is consistent.
//
// Flags:
//
//	-a  album-mode (writes REPLAYGAIN_ALBUM_*)
//	-k  prevent clipping (caps gain so peak <= -1 dBTP)
//	-s e  write extended ReplayGain 2.0 tags + extra LU/LRA info
//
// Returns ErrToolNotInstalled if the binary is missing — callers
// (audio-CD pipeline) WARN and continue. An empty flacPaths slice is
// a no-op (nil error).
func (l *Loudgain) TagAlbum(ctx context.Context, flacPaths []string) error {
	if len(flacPaths) == 0 {
		return nil
	}
	if _, err := exec.LookPath(l.bin); err != nil {
		return ErrToolNotInstalled
	}

	args := append([]string{"-a", "-k", "-s", "e"}, flacPaths...)
	cmd := exec.CommandContext(ctx, l.bin, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("loudgain: %w: %s", err, string(out))
	}
	return nil
}
