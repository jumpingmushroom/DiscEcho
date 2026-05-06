package identify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ErrUnknownDiscType is returned when the cd-info output doesn't match
// any known disc type. The orchestrator turns this into a job-state
// failure with a clear "unsupported disc" message.
var ErrUnknownDiscType = errors.New("identify: unknown disc type")

// Classifier probes a drive and returns its disc type.
type Classifier interface {
	Classify(ctx context.Context, devPath string) (state.DiscType, error)
}

// ClassifierConfig configures NewClassifier.
type ClassifierConfig struct {
	CDInfoBin string // default "cd-info"
}

// NewClassifier returns a Classifier that shells out to cd-info.
func NewClassifier(c ClassifierConfig) Classifier {
	if c.CDInfoBin == "" {
		c.CDInfoBin = "cd-info"
	}
	return &cdInfoClassifier{bin: c.CDInfoBin}
}

type cdInfoClassifier struct {
	bin string
}

func (c *cdInfoClassifier) Classify(ctx context.Context, devPath string) (state.DiscType, error) {
	cmd := exec.CommandContext(ctx, c.bin, devPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cd-info: %w", err)
	}
	return ClassifyFromCDInfo(string(out))
}

// ClassifyFromCDInfo parses cd-info stdout/stderr and returns the disc
// type. M1.1 recognises CD-DA → AUDIO_CD; all other valid disc modes
// fall through as DATA. Empty input is an error (we expect cd-info to
// have produced *something* if it exited zero).
func ClassifyFromCDInfo(s string) (state.DiscType, error) {
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("ClassifyFromCDInfo: empty input")
	}
	for _, line := range strings.Split(s, "\n") {
		l := strings.ToLower(line)
		if !strings.Contains(l, "disc mode") {
			continue
		}
		switch {
		case strings.Contains(l, "cd-da"):
			return state.DiscTypeAudioCD, nil
		case strings.Contains(l, "cd-rom mode"):
			return state.DiscTypeData, nil
		case strings.Contains(l, "dvd"):
			return state.DiscTypeData, nil
		}
	}
	return state.DiscTypeData, nil
}
