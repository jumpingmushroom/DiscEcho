// Package dreamcast implements pipelines.Handler for Sega Dreamcast GD-ROM discs.
//
// Pipeline shape (7 active steps; transcode skipped):
//
//	detect → identify → rip (redumper cd) → compress (chdman) → move → notify → eject
//
// Identify is type-only: the GD-ROM high-density area is inaccessible without
// a partial redumper run, so we return a placeholder disc and ErrNoCandidates.
// The new-disc sheet shows a generic placeholder; auto-confirm proceeds.
//
// Post-rip: after redumper produces the GDI + tracks, we hash the first .bin
// track (the same artifact Redump keys its MD5 against) and call
// RedumpDB.LookupByMD5. On hit, disc.Title / disc.Region / MetadataProvider /
// MetadataID are updated before the output path template is rendered, so the
// final file lands at the correct library path.
package dreamcast

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
	RedumpDB       *identify.RedumpDB
	Tools          *tools.Registry // looked up: apprise, eject
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
}

// Handler implements pipelines.Handler for Sega Dreamcast.
type Handler struct{ deps Deps }

// New returns a Handler with the given dependencies.
func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

func (h *Handler) DiscType() state.DiscType { return state.DiscTypeDC }

// Identify returns a placeholder disc and ErrNoCandidates. The GD-ROM
// high-density area cannot be read without a partial redumper run, so
// pre-rip identification is not possible.
func (h *Handler) Identify(_ context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	disc := &state.Disc{
		Type:    state.DiscTypeDC,
		DriveID: drv.ID,
		Title:   "Dreamcast disc",
	}
	return disc, nil, pipelines.ErrNoCandidates
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

// Run rips with redumper (cd mode, which handles GD-ROM layout and produces a
// GDI + tracks), performs post-rip MD5 identification, compresses with chdman,
// then atomic-moves to the library.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — redumper cd mode handles GD-ROM and produces <name>.gdi + tracks.
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
		err := errors.New("dreamcast: redumper or chdman not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	name := "dc-" + disc.ID
	if err := h.deps.Redumper.Rip(ctx, drv.DevPath, tmpdir, name, "cd", newStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("redumper: %w", err)
	}
	gdiPath := filepath.Join(tmpdir, name+".gdi")
	// The first .bin track is what Redump keys its MD5 against. Naming
	// convention: <name>01.bin (redumper zero-pads track numbers to two digits
	// starting at 01).
	track01Path := filepath.Join(tmpdir, name+"01.bin")
	sink.OnStepDone(state.StepRip, map[string]any{"file": gdiPath})

	// compress — post-rip MD5 identification before chdman.
	sink.OnStepStart(state.StepCompress)
	h.tryMD5Identify(track01Path, disc)

	chdPath := filepath.Join(tmpdir, name+".chd")
	// chdman accepts a .gdi as input, same as a .cue for Saturn.
	if err := h.deps.CHDMan.CreateCHD(ctx, gdiPath, chdPath, newStepSink(sink, state.StepCompress)); err != nil {
		sink.OnStepFailed(state.StepCompress, err)
		return fmt.Errorf("chdman: %w", err)
	}
	sink.OnStepDone(state.StepCompress, map[string]any{"file": chdPath})

	// move — atomic rename to library using title/region resolved above.
	sink.OnStepStart(state.StepMove)
	region := ""
	if len(disc.Candidates) > 0 {
		region = disc.Candidates[0].Region
	}
	rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, pipelines.OutputFields{
		Title:  disc.Title,
		Year:   disc.Year,
		Region: region,
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

// tryMD5Identify hashes track01Path and looks it up in RedumpDB. On hit it
// mutates disc in place so the output template is rendered with the correct
// title and region. On miss it logs a warning and leaves disc unchanged.
func (h *Handler) tryMD5Identify(track01Path string, disc *state.Disc) {
	if h.deps.RedumpDB == nil {
		slog.Warn("dreamcast: no RedumpDB; skipping post-rip identification")
		return
	}
	got, err := md5File(track01Path)
	if err != nil {
		slog.Warn("dreamcast: md5 hash failed", "err", err)
		return
	}
	entry := h.deps.RedumpDB.LookupByMD5(got)
	if entry == nil {
		slog.Warn("dreamcast: no Redump match for disc", "md5", got)
		return
	}
	slog.Info("dreamcast: matched", "title", entry.Title, "md5", got)
	disc.Title = entry.Title
	disc.Year = entry.Year
	disc.MetadataProvider = "Redump"
	disc.MetadataID = entry.BootCode
	// Append a Candidate so region is available to the output template
	// renderer, matching the same pattern Saturn uses at identify-time.
	disc.Candidates = []state.Candidate{{
		Source:     "Redump",
		Title:      entry.Title,
		Year:       entry.Year,
		Region:     entry.Region,
		Confidence: 100,
	}}
}

func (h *Handler) createWorkDir(discID string) (string, error) {
	root := h.deps.WorkRoot
	if root == "" {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "discecho-dc-"+discID+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
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
