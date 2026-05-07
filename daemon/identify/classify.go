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

// SaturnProber probes a CD device for Saturn IP.BIN at sector 0.
type SaturnProber interface {
	Probe(ctx context.Context, devPath string) (*SaturnInfo, error)
}

// XboxProber probes a mounted Xbox disc for the XBE certificate.
type XboxProber interface {
	Probe(ctx context.Context, mountPoint string) (*XboxInfo, error)
}

// DCProber probes a CD device's TOC for the multi-session layout that
// indicates a Dreamcast GD-ROM.
type DCProber interface {
	Probe(ctx context.Context, devPath string) (bool, error)
}

// ClassifierConfig configures NewClassifier. FSProber, BDProber, and
// SystemCNFProber are optional dependencies — when nil, the classifier
// defaults to using NewFSProber / NewBDProber / NewSystemCNFProber with
// default binaries. SaturnProber, XboxProber, and DCProber are optional;
// when nil those branches are skipped.
type ClassifierConfig struct {
	CDInfoBin       string          // default "cd-info"
	FSProber        FSProber        // default NewFSProber({}) — distinguishes DVD/BDMV/DATA
	BDProber        BDProber        // default NewBDProber({}) — distinguishes BDMV vs UHD
	SystemCNFProber SystemCNFProber // default NewSystemCNFProber("") — distinguishes PSX vs PS2
	SaturnProber    SaturnProber    // optional — detects Sega Saturn via IP.BIN
	XboxProber      XboxProber      // optional — detects Xbox via default.xbe
	DCProber        DCProber        // optional — detects Dreamcast via GD-ROM TOC heuristic
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
		saturn:    c.SaturnProber,
		xbox:      c.XboxProber,
		dc:        c.DCProber,
	}
}

type multiProbeClassifier struct {
	cdInfoBin string
	fs        FSProber
	bd        BDProber
	sysCNF    SystemCNFProber
	saturn    SaturnProber
	xbox      XboxProber
	dc        DCProber
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
	return RefineDiscType(ctx, base, c.fs, c.bd, c.sysCNF, c.saturn, c.xbox, c.dc, devPath), nil
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
// listing, bd_info (Blu-ray AACS2 detection), SYSTEM.CNF (PSX/PS2
// discrimination), and dedicated probers for Xbox, Saturn, and Dreamcast.
// AUDIO_CD short-circuits.
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
//   - DATA + /default.xbe + xbox probe ok → XBOX
//   - saturn probe ok (raw sector 0) → SAT
//   - dc probe ok (TOC heuristic) → DC
//   - else → DATA
//
// Probes that error are logged and treated as a negative result.
func RefineDiscType(ctx context.Context, base state.DiscType, fs FSProber, bd BDProber, sysCNF SystemCNFProber, saturn SaturnProber, xbox XboxProber, dc DCProber, devPath string) state.DiscType {
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
	// Xbox: /default.xbe at root signals a candidate; XBE probe confirms.
	if hasPath(files, "/default.xbe") && xbox != nil {
		info, err := xbox.Probe(ctx, devPath)
		if err != nil {
			if !errors.Is(err, ErrNotXbox) {
				slog.Warn("classify: xbox probe failed", "dev", devPath, "err", err)
			}
		} else if info != nil {
			return state.DiscTypeXBOX
		}
	}
	// Saturn: probe raw sector 0 regardless of fs listing; Saturn discs
	// often have no ISO9660 filesystem so files may be empty.
	if saturn != nil {
		info, err := saturn.Probe(ctx, devPath)
		if err != nil {
			if !errors.Is(err, ErrNotSaturn) {
				slog.Warn("classify: saturn probe failed", "dev", devPath, "err", err)
			}
		} else if info != nil {
			return state.DiscTypeSAT
		}
	}
	// Dreamcast: multi-session TOC heuristic (GD-ROM HD area at LBA ≥ 45000).
	if dc != nil {
		ok, err := dc.Probe(ctx, devPath)
		if err != nil {
			slog.Warn("classify: dc probe failed", "dev", devPath, "err", err)
		} else if ok {
			return state.DiscTypeDC
		}
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
