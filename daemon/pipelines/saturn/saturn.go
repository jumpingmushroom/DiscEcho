// Package saturn implements pipelines.Handler for Sega Saturn game discs.
//
// Pipeline shape (7 active steps; transcode skipped):
//
//	detect → identify → rip (redumper) → compress (chdman) → move → notify → eject
//
// Identify reads Saturn IP.BIN off sector 0 of the disc, looks up the
// product number against the user-supplied Redump dat.
// ErrNoCandidates surfaces when the dat is missing OR the product number
// is unknown — both fall through to manualIdentify (M2.2 sheet).
package saturn

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// RedumperRipper is the slice of *tools.Redumper used at rip-time.
type RedumperRipper interface {
	Rip(ctx context.Context, devPath, outDir, name, mode string, sink tools.Sink) error
}

// CHDManCompressor is the slice of *tools.CHDMan used at compress-time.
type CHDManCompressor interface {
	CreateCHD(ctx context.Context, input, output string, sink tools.Sink) error
}

// Deps bundles the handler's dependencies.
type Deps struct {
	Redumper       RedumperRipper
	CHDMan         CHDManCompressor
	SaturnProber   identify.SaturnProber
	RedumpDB       *identify.RedumpDB
	Tools          *tools.Registry // looked up: apprise, eject
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
}

// Handler implements pipelines.Handler for Sega Saturn.
type Handler struct{ deps Deps }

func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

func (h *Handler) DiscType() state.DiscType { return state.DiscTypeSAT }

// Identify reads Saturn IP.BIN, looks up the product number in RedumpDB.
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	disc := &state.Disc{Type: state.DiscTypeSAT, DriveID: drv.ID}

	if h.deps.RedumpDB == nil {
		slog.Warn("saturn: redump saturn.dat missing", "dev", drv.DevPath)
		return disc, nil, pipelines.ErrNoCandidates
	}
	if h.deps.SaturnProber == nil {
		return nil, nil, errors.New("saturn: SaturnProber not configured")
	}
	info, err := h.deps.SaturnProber.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("saturn: IP.BIN probe: %w", err)
	}
	if info == nil || info.ProductNumber == "" {
		return disc, nil, pipelines.ErrNoCandidates
	}
	entry := h.deps.RedumpDB.LookupByBootCode(info.ProductNumber)
	if entry == nil {
		return disc, nil, pipelines.ErrNoCandidates
	}
	disc.Title = entry.Title
	disc.Year = entry.Year
	disc.MetadataProvider = "Redump"
	disc.MetadataID = info.ProductNumber
	cand := state.Candidate{
		Source:     "Redump",
		Title:      entry.Title,
		Year:       entry.Year,
		Region:     entry.Region,
		Confidence: 100,
	}
	disc.Candidates = []state.Candidate{cand}
	return disc, disc.Candidates, nil
}

// Plan returns the 7-active-step plan; transcode is skipped.
func (h *Handler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	skipped := map[state.StepID]bool{state.StepTranscode: true}
	out := make([]pipelines.StepPlan, 0, len(state.CanonicalSteps()))
	for _, sid := range state.CanonicalSteps() {
		out = append(out, pipelines.StepPlan{ID: sid, Skip: skipped[sid]})
	}
	return out
}

// Run rips with redumper (cd mode), compresses with chdman, atomic-moves to the library.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — redumper produces <name>.bin + <name>.cue (CD media).
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
	if h.deps.Redumper == nil || h.deps.CHDMan == nil {
		err := errors.New("saturn: redumper or chdman not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	name := "sat-" + disc.ID
	if err := h.deps.Redumper.Rip(ctx, drv.DevPath, tmpdir, name, "cd", newStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("redumper: %w", err)
	}
	binPath := filepath.Join(tmpdir, name+".bin")
	cuePath := filepath.Join(tmpdir, name+".cue")
	sink.OnStepDone(state.StepRip, map[string]any{"file": binPath})

	// compress — MD5-verify .bin (warn only; mismatch is non-fatal), then chdman createcd.
	sink.OnStepStart(state.StepCompress)
	if h.deps.RedumpDB != nil && disc.MetadataID != "" {
		if entry := h.deps.RedumpDB.LookupByBootCode(disc.MetadataID); entry != nil && entry.MD5 != "" {
			got, err := md5File(binPath)
			if err != nil {
				slog.Warn("saturn: md5 verify failed (couldn't hash)", "err", err)
			} else if got != entry.MD5 {
				slog.Warn("saturn: md5 mismatch", "want", entry.MD5, "got", got)
			} else {
				slog.Info("saturn: md5 verify ok", "md5", got)
			}
		}
	}
	chdPath := filepath.Join(tmpdir, name+".chd")
	if err := h.deps.CHDMan.CreateCHD(ctx, cuePath, chdPath, newStepSink(sink, state.StepCompress)); err != nil {
		sink.OnStepFailed(state.StepCompress, err)
		return fmt.Errorf("chdman: %w", err)
	}
	sink.OnStepDone(state.StepCompress, map[string]any{"file": chdPath})

	// move — atomic rename to library.
	sink.OnStepStart(state.StepMove)
	region := ""
	if len(disc.Candidates) > 0 {
		region = disc.Candidates[0].Region
	}
	rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, pipelines.OutputFields{
		Title: disc.Title, Year: disc.Year, Region: region,
	})
	if err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	dst := filepath.Join(h.deps.LibraryRoot, rel)
	if err := pipelines.AtomicMove(chdPath, dst); err != nil {
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
	dir := filepath.Join(root, "discecho-sat-"+discID+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	return dir, nil
}

// md5File returns the lowercase hex MD5 of the file's contents.
func md5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
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
