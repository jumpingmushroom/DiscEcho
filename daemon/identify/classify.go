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

// ClassifierConfig configures NewClassifier. FSProber, BDProber, and
// SystemCNFProber are optional dependencies — when nil, the classifier
// defaults to using NewFSProber / NewBDProber / NewSystemCNFProber with
// default binaries.
type ClassifierConfig struct {
	CDInfoBin       string          // default "cd-info"
	FSProber        FSProber        // default NewFSProber({}) — distinguishes DVD/BDMV/DATA
	BDProber        BDProber        // default NewBDProber({}) — distinguishes BDMV vs UHD
	SystemCNFProber SystemCNFProber // default NewSystemCNFProber("") — distinguishes PSX vs PS2
}

// NewClassifier returns a Classifier that runs the multi-probe pipeline.
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
	if c.SystemCNFProber == nil {
		c.SystemCNFProber = NewSystemCNFProber("")
	}
	return &multiProbeClassifier{
		cdInfoBin: c.CDInfoBin,
		fs:        c.FSProber,
		bd:        c.BDProber,
		sysCNF:    c.SystemCNFProber,
	}
}

type multiProbeClassifier struct {
	cdInfoBin string
	fs        FSProber
	bd        BDProber
	sysCNF    SystemCNFProber
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
	return RefineDiscType(ctx, base, c.fs, c.bd, c.sysCNF, devPath), nil
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

// RefineDiscType upgrades a cd-info-level disc type using filesystem
// listing, bd_info (Blu-ray AACS2 detection), and SYSTEM.CNF (PSX/PS2
// discrimination). AUDIO_CD short-circuits.
//
// Decision tree:
//   - AUDIO_CD → AUDIO_CD (passthrough)
//   - DATA + /VIDEO_TS → DVD
//   - DATA + /BDMV/index.bdmv:
//     bd_info AACS2 → UHD
//     else          → BDMV
//   - DATA + /SYSTEM.CNF:
//     SYSTEM.CNF readable + IsPS2 → PS2
//     SYSTEM.CNF readable + !IsPS2 → PSX
//     SYSTEM.CNF unreadable → DATA
//   - else → DATA
//
// Probes that error are logged and treated as a negative result.
func RefineDiscType(ctx context.Context, base state.DiscType, fs FSProber, bd BDProber, sysCNF SystemCNFProber, devPath string) state.DiscType {
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
	if hasPath(files, "/SYSTEM.CNF") {
		if sysCNF == nil {
			slog.Warn("classify: SYSTEM.CNF present but no prober; treating as DATA", "dev", devPath)
			return state.DiscTypeData
		}
		info, err := sysCNF.Probe(ctx, devPath)
		if err != nil {
			slog.Warn("classify: system.cnf probe failed; treating as DATA", "dev", devPath, "err", err)
			return state.DiscTypeData
		}
		if info == nil {
			return state.DiscTypeData
		}
		if info.IsPS2 {
			return state.DiscTypePS2
		}
		return state.DiscTypePSX
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
