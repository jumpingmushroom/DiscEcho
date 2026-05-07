// Package data implements pipelines.Handler for raw data discs.
//
// Pipeline shape (6 active steps; transcode AND compress skipped):
//
//	detect → identify → rip (dd) → move → notify → eject
//
// Identify reads the ISO9660 volume label via `isoinfo -d` and uses it as
// disc.Title. If the label is empty or whitespace, the title falls back to
// "data-disc-YYYYMMDD-HHMMSS". No external metadata lookup is performed;
// Identify always returns ErrNoCandidates.
//
// Run copies the raw disc to an ISO via dd, sha256-hashes the output, and
// writes "sha256=<hex>; size=<bytes>" to disc.Notes before atomic-moving
// the file to the library.
package data

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// DDCopier is the subset of *tools.DD used at rip-time.
type DDCopier interface {
	Copy(ctx context.Context, devPath, outPath string, totalBytes int64, sink tools.Sink) error
}

// LabelProber reads the ISO9660 volume label from a disc device.
type LabelProber interface {
	Probe(ctx context.Context, devPath string) (string, error)
}

// IsoinfoLabelProber implements LabelProber by shelling out to
// `isoinfo -d -i <devPath>` and extracting the "Volume id:" line.
type IsoinfoLabelProber struct {
	// Bin is the isoinfo binary name. Defaults to "isoinfo".
	Bin string
}

func (p *IsoinfoLabelProber) bin() string {
	if p.Bin == "" {
		return "isoinfo"
	}
	return p.Bin
}

// Probe returns the volume label, which may be empty for unlabelled discs.
func (p *IsoinfoLabelProber) Probe(ctx context.Context, devPath string) (string, error) {
	out, err := exec.CommandContext(ctx, p.bin(), "-d", "-i", devPath).Output()
	if err != nil {
		return "", fmt.Errorf("isoinfo -d: %w", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Volume id:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Volume id:")), nil
		}
	}
	return "", nil
}

// Deps bundles the handler's dependencies.
type Deps struct {
	DD             DDCopier
	LabelProber    LabelProber
	Tools          *tools.Registry // looked up: apprise, eject
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	// Now is called to produce the fallback timestamp title. Defaults to time.Now.
	Now func() time.Time
}

// Handler implements pipelines.Handler for raw data discs.
type Handler struct{ deps Deps }

// New returns a Handler with the given dependencies.
func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	return &Handler{deps: d}
}

func (h *Handler) DiscType() state.DiscType { return state.DiscTypeData }

// Identify reads the volume label and uses it as disc.Title.
// If the label is empty, Title falls back to "data-disc-YYYYMMDD-HHMMSS".
// Always returns ErrNoCandidates because data discs have no metadata source.
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	disc := &state.Disc{Type: state.DiscTypeData, DriveID: drv.ID}

	label := ""
	if h.deps.LabelProber != nil {
		var err error
		label, err = h.deps.LabelProber.Probe(ctx, drv.DevPath)
		if err != nil {
			slog.Warn("data: volume label probe failed; using timestamp fallback", "dev", drv.DevPath, "err", err)
		}
	}

	if strings.TrimSpace(label) == "" {
		disc.Title = "data-disc-" + h.deps.Now().UTC().Format("20060102-150405")
	} else {
		disc.Title = label
	}

	return disc, nil, pipelines.ErrNoCandidates
}

// Plan returns the 8-entry canonical plan; transcode and compress are skipped.
func (h *Handler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	skipped := map[state.StepID]bool{
		state.StepTranscode: true,
		state.StepCompress:  true,
	}
	out := make([]pipelines.StepPlan, 0, len(state.CanonicalSteps()))
	for _, sid := range state.CanonicalSteps() {
		out = append(out, pipelines.StepPlan{ID: sid, Skip: skipped[sid]})
	}
	return out
}

// Run copies the disc via dd, sha256-hashes the ISO, writes hash+size to
// disc.Notes, and atomic-moves the file to the library.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — dd produces disc.iso in a temporary work directory.
	sink.OnStepStart(state.StepRip)
	tmpdir, err := h.createWorkDir(disc.ID)
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	defer func() { _ = os.RemoveAll(tmpdir) }()

	if err := h.deps.LibraryProbe(h.deps.LibraryRoot); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("library probe: %w", err)
	}

	// Determine total disc size for dd progress reporting.
	// If blockdev fails (e.g. in tests without a real device), we pass 0
	// and dd progress percentage stays at 0 — the rip still completes.
	totalBytes := blockSize(ctx, drv.DevPath)

	isoPath := filepath.Join(tmpdir, "disc.iso")
	if h.deps.DD == nil {
		err := fmt.Errorf("data: DD not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	if err := h.deps.DD.Copy(ctx, drv.DevPath, isoPath, totalBytes, newStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("dd: %w", err)
	}

	// sha256 hash + size stored in the existing Disc fields:
	// TOCHash holds the hex digest (it's a content-identity hash here rather
	// than a TOC hash), and SizeBytesRaw holds the byte count.
	hashHex, size, err := sha256File(isoPath)
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("sha256: %w", err)
	}
	disc.TOCHash = hashHex
	disc.SizeBytesRaw = size
	sink.OnStepDone(state.StepRip, map[string]any{"file": isoPath})

	// move — atomic rename to library; no region segment for data discs.
	sink.OnStepStart(state.StepMove)
	rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, pipelines.OutputFields{
		Title: disc.Title,
	})
	if err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	dst := filepath.Join(h.deps.LibraryRoot, rel)
	if err := pipelines.AtomicMove(isoPath, dst); err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	sink.OnStepDone(state.StepMove, map[string]any{"path": dst})

	// notify
	sink.OnStepStart(state.StepNotify)
	if h.deps.Tools != nil {
		if apprise, ok := h.deps.Tools.Get("apprise"); ok {
			var urls []string
			if h.deps.URLsForTrigger != nil {
				urls = h.deps.URLsForTrigger(ctx, "done")
			}
			title := fmt.Sprintf("DiscEcho: %s", disc.Title)
			body := fmt.Sprintf("Ripped to %s", h.deps.LibraryRoot)
			argv := tools.BuildAppriseArgs(title, body, "", urls)
			_ = apprise.Run(ctx, argv, nil, "", newStepSink(sink, state.StepNotify))
		}
	}
	sink.OnStepDone(state.StepNotify, nil)

	// eject
	sink.OnStepStart(state.StepEject)
	if h.deps.Tools != nil {
		if eject, ok := h.deps.Tools.Get("eject"); ok && drv != nil && drv.DevPath != "" {
			if err := eject.Run(ctx, []string{drv.DevPath}, nil, "", newStepSink(sink, state.StepEject)); err != nil {
				sink.OnStepFailed(state.StepEject, err)
			}
		}
	}
	sink.OnStepDone(state.StepEject, nil)
	return nil
}

func (h *Handler) createWorkDir(discID string) (string, error) {
	root := h.deps.WorkRoot
	if root == "" {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "discecho-data-"+discID+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	return dir, nil
}

// blockSize calls `blockdev --getsize64 devPath` to get the disc size in bytes.
// Returns 0 on error so callers can treat it as "unknown size" and still proceed.
func blockSize(ctx context.Context, devPath string) int64 {
	out, err := exec.CommandContext(ctx, "blockdev", "--getsize64", devPath).Output()
	if err != nil {
		return 0
	}
	n, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// sha256File returns the lowercase hex SHA-256 and byte count of the file.
func sha256File(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}

// stepSink wraps an EventSink so a Tool's per-step Sink calls land
// against a fixed step ID.
type stepSink struct {
	sink pipelines.EventSink
	step state.StepID
}

func newStepSink(s pipelines.EventSink, step state.StepID) *stepSink {
	return &stepSink{sink: s, step: step}
}

func (s *stepSink) Progress(pct float64, speed string, eta int) {
	s.sink.OnProgress(s.step, pct, speed, eta)
}

func (s *stepSink) Log(level state.LogLevel, format string, args ...any) {
	s.sink.OnLog(level, format, args...)
}
