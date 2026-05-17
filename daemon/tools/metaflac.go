package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Metaflac wraps the `metaflac` CLI from the FLAC reference distribution.
// Used by the audio-CD pipeline to embed front cover art into each
// ripped FLAC. The tool ships with Debian's `flac` package.
type Metaflac struct {
	bin string
}

// NewMetaflac returns a Metaflac that runs <bin>. Empty defaults to
// "metaflac" (resolved via PATH).
func NewMetaflac(bin string) *Metaflac {
	if bin == "" {
		bin = "metaflac"
	}
	return &Metaflac{bin: bin}
}

func (m *Metaflac) Name() string { return "metaflac" }

// Run implements the generic Tool interface. The audio-CD pipeline
// uses EmbedFrontCover directly, but the interface call lets the
// registry expose the tool to non-pipeline callers (e.g. tests).
func (m *Metaflac) Run(ctx context.Context, args []string, _ map[string]string,
	_ string, sink Sink) error {
	cmd := exec.CommandContext(ctx, m.bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		sink.Log(state.LogLevelWarn, "metaflac failed: %v: %s", err, string(out))
		return fmt.Errorf("metaflac: %w", err)
	}
	return nil
}

// EmbedFrontCover writes an `image/jpeg` PICTURE block (type 3, front
// cover) onto the FLAC at flacPath, sourcing bytes from imagePath. Any
// pre-existing PICTURE blocks are stripped first so re-runs are
// idempotent (matters for re-rips that replace earlier output).
//
// Returns ErrToolNotInstalled if the metaflac binary is not on PATH;
// callers can WARN and continue. Other errors are wrapped with
// `metaflac:` context.
func (m *Metaflac) EmbedFrontCover(ctx context.Context, flacPath, imagePath string) error {
	if _, err := exec.LookPath(m.bin); err != nil {
		return ErrToolNotInstalled
	}

	// Strip any existing PICTURE blocks. metaflac --remove with no match
	// is a no-op (exit 0), so this is safe on fresh files.
	rmCmd := exec.CommandContext(ctx, m.bin,
		"--remove", "--block-type=PICTURE", "--dont-use-padding", flacPath)
	if out, err := rmCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("metaflac --remove: %w: %s", err, string(out))
	}

	importCmd := exec.CommandContext(ctx, m.bin,
		"--import-picture-from", "3||||"+imagePath, flacPath)
	if out, err := importCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("metaflac --import-picture-from: %w: %s", err, string(out))
	}
	return nil
}

// missingPathError lets tests recognise the binary-missing case without
// reaching into exec.LookPath internals. Currently unused beyond
// errors.Is(ErrToolNotInstalled); kept to make the intent explicit if
// the API ever grows finer-grained errors.
var _ = errors.Is
