package identify

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ErrUnknownDiscType is returned when none of the probes recognise the
// disc. The orchestrator turns this into a job-state failure with a
// clear "unsupported disc" message.
var ErrUnknownDiscType = errors.New("identify: unknown disc type")

// Classifier probes a drive and returns its disc type.
type Classifier interface {
	Classify(ctx context.Context, devPath string) (state.DiscType, error)
}

// ClassifierConfig configures NewClassifier. FSProber and BDProber are
// optional dependencies — when nil, the classifier defaults to using
// NewFSProber / NewBDProber with default binaries.
type ClassifierConfig struct {
	CDInfoBin string   // default "cd-info"
	FSProber  FSProber // default NewFSProber({}) — distinguishes DVD/BDMV/DATA
	BDProber  BDProber // default NewBDProber({}) — distinguishes BDMV vs UHD
}

// NewClassifier returns a Classifier that runs the three-step probe.
func NewClassifier(c ClassifierConfig) Classifier {
	if c.CDInfoBin == "" {
		c.CDInfoBin = "cd-info"
	}
	if c.FSProber == nil {
		c.FSProber = NewFSProber(FSProberConfig{})
	}
	if c.BDProber == nil {
		c.BDProber = NewBDProber(BDProberConfig{})
	}
	return &multiProbeClassifier{
		cdInfoBin: c.CDInfoBin,
		fs:        c.FSProber,
		bd:        c.BDProber,
	}
}

type multiProbeClassifier struct {
	cdInfoBin string
	fs        FSProber
	bd        BDProber
}

func (c *multiProbeClassifier) Classify(ctx context.Context, devPath string) (state.DiscType, error) {
	cmd := exec.CommandContext(ctx, c.cdInfoBin, devPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("cd-info: %w", err)
	}
	base, err := ClassifyFromCDInfo(string(out))
	if err != nil {
		return "", err
	}
	return RefineDiscType(ctx, base, c.fs, c.bd, devPath), nil
}

// ClassifyFromCDInfo parses cd-info stdout/stderr and returns the
// disc-mode-level type. Recognises CD-DA → AUDIO_CD; everything else
// falls through as DATA. The higher-level classifier (RefineDiscType)
// inspects the filesystem to upgrade DATA → DVD / BDMV / UHD.
func ClassifyFromCDInfo(s string) (state.DiscType, error) {
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("ClassifyFromCDInfo: empty input")
	}
	for _, line := range strings.Split(s, "\n") {
		l := strings.ToLower(line)
		if !strings.Contains(l, "disc mode") {
			continue
		}
		if strings.Contains(l, "cd-da") {
			return state.DiscTypeAudioCD, nil
		}
	}
	return state.DiscTypeData, nil
}

// RefineDiscType upgrades a cd-info-level disc type using the
// filesystem listing and (for Blu-ray) bd_info. AUDIO_CD short-circuits
// — no fs probe is run since audio CDs have no ISO9660 filesystem.
//
// Decision tree:
//   - AUDIO_CD → AUDIO_CD (passthrough)
//   - DATA + /VIDEO_TS present → DVD
//   - DATA + /BDMV/index.bdmv present:
//     bd_info reports AACS2 → UHD
//     bd_info fails or no AACS2 → BDMV
//   - else → DATA
//
// Probes that error are logged and treated as a negative result; we
// never propagate probe errors as classification failures (the disc
// might still be a usable DATA disc).
func RefineDiscType(ctx context.Context, base state.DiscType, fs FSProber, bd BDProber, devPath string) state.DiscType {
	if base == state.DiscTypeAudioCD {
		return state.DiscTypeAudioCD
	}
	if fs == nil {
		return base
	}
	files, err := fs.List(ctx, devPath)
	if err != nil {
		slog.Warn("classify: fs probe failed", "dev", devPath, "err", err)
		return base
	}
	if hasPath(files, "/VIDEO_TS") {
		return state.DiscTypeDVD
	}
	if hasPath(files, "/BDMV/index.bdmv") {
		if bd == nil {
			return state.DiscTypeBDMV
		}
		info, err := bd.Probe(ctx, devPath)
		if err != nil {
			slog.Warn("classify: bd_info failed; defaulting to BDMV", "dev", devPath, "err", err)
			return state.DiscTypeBDMV
		}
		if info != nil && info.HasAACS2 {
			return state.DiscTypeUHD
		}
		return state.DiscTypeBDMV
	}
	return state.DiscTypeData
}

func hasPath(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
