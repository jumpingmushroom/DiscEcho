// Package uhd implements pipelines.Handler for Ultra HD Blu-ray discs.
//
// Pipeline shape (6 active steps; transcode + compress skipped):
//
//	detect → identify → rip (MakeMKV remux) → move → notify → eject
//
// Identify performs a key-file precheck before any TMDB lookup: if
// AACS2KeyDB does not exist, returns ErrAACS2KeyMissing so the
// orchestrator can surface a clear "missing keys" error before any
// disc read happens.
package uhd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// ErrAACS2KeyMissing is returned by Identify when the configured
// KEYDB.cfg path is unset or doesn't exist. Wrapped error messages
// include the path so the user knows where to drop the file.
var ErrAACS2KeyMissing = errors.New("uhd: AACS2 key file missing")

// MakeMKVScanner / MakeMKVRipper are the slice of tools.MakeMKV used
// at scan-time and rip-time respectively. Mirrors the bdmv package.
type MakeMKVScanner interface {
	Scan(ctx context.Context, devPath string) ([]tools.MakeMKVTitle, error)
}
type MakeMKVRipper interface {
	Rip(ctx context.Context, devPath string, titleID int, outDir string, sink tools.Sink) error
}

// Deps bundles the handler's dependencies for mock injection.
type Deps struct {
	Prober         identify.DVDProber
	TMDB           identify.TMDBClient
	MakeMKVScanner MakeMKVScanner
	MakeMKVRipper  MakeMKVRipper
	Tools          *tools.Registry // looked up: apprise, eject
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	SubsLang       string
	AACS2KeyDB     string // path to KEYDB.cfg; checked at Identify-time
	// ShouldEject gates the rip-end eject step; nil = always eject.
	ShouldEject func(ctx context.Context) bool
}

// Handler implements pipelines.Handler for UHD.
type Handler struct{ deps Deps }

// New constructs the handler.
func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

// DiscType returns UHD.
func (h *Handler) DiscType() state.DiscType { return state.DiscTypeUHD }

// Identify runs the AACS2 key-file precheck, then volume-label probe,
// then TMDB lookup. Errors:
//
//   - ErrAACS2KeyMissing if AACS2KeyDB is unset or doesn't exist
//   - ErrNoCandidates if the volume label doesn't normalise or TMDB returns 0
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	if h.deps.AACS2KeyDB == "" {
		return nil, nil, fmt.Errorf("%w: AACS2KeyDB not configured", ErrAACS2KeyMissing)
	}
	if _, err := os.Stat(h.deps.AACS2KeyDB); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, fmt.Errorf("%w: %s", ErrAACS2KeyMissing, h.deps.AACS2KeyDB)
		}
		return nil, nil, fmt.Errorf("uhd: stat key file: %w", err)
	}
	if h.deps.Prober == nil {
		return nil, nil, errors.New("uhd: prober not configured")
	}
	if h.deps.TMDB == nil {
		return nil, nil, errors.New("uhd: TMDB client not configured")
	}

	info, err := h.deps.Prober.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("uhd probe: %w", err)
	}
	disc := &state.Disc{Type: state.DiscTypeUHD, DriveID: drv.ID}

	q := identify.NormaliseDVDLabel(info.VolumeLabel)
	if q == "" {
		return disc, nil, pipelines.ErrNoCandidates
	}
	cands, err := h.deps.TMDB.SearchBoth(ctx, q)
	if err != nil {
		return nil, nil, fmt.Errorf("tmdb search: %w", err)
	}
	if len(cands) == 0 {
		return disc, nil, pipelines.ErrNoCandidates
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].Confidence > cands[j].Confidence })
	top := cands[0]
	disc.Title = top.Title
	disc.Year = top.Year
	disc.MetadataProvider = top.Source
	disc.MetadataID = strconv.Itoa(top.TMDBID)
	disc.Candidates = cands
	return disc, cands, nil
}

// Plan returns the 6-active-step plan; transcode + compress skipped.
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

// Run executes the UHD pipeline: scan → rip → move → notify → eject.
// transcode + compress are SKIPPED — neither step is even started so
// the recording sink never sees them. The MakeMKV output IS the
// artifact (HDR/DV/lossless audio preserved).
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

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
	if h.deps.MakeMKVScanner == nil || h.deps.MakeMKVRipper == nil {
		err := errors.New("uhd: MakeMKV not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	sink.OnLog(state.LogLevelInfo, "MakeMKV: scanning %s (UHD)", drv.DevPath)
	titles, err := h.deps.MakeMKVScanner.Scan(ctx, drv.DevPath)
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("makemkv scan: %w", err)
	}
	picked, err := pickLongestTitle(titles, prof)
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	sink.OnLog(state.LogLevelInfo, "MakeMKV: scan complete, picked title %d (longest %s)",
		picked.ID, pipelines.HumanDuration(time.Duration(picked.DurationSec)*time.Second))
	ripDir := filepath.Join(tmpdir, "rip")
	if err := os.MkdirAll(ripDir, 0o755); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	ripStart := time.Now()
	if err := h.deps.MakeMKVRipper.Rip(ctx, drv.DevPath, picked.ID, ripDir, pipelines.NewStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("makemkv rip: %w", err)
	}
	rippedFile, err := singleMKVIn(ripDir)
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	var ripSize int64
	if fi, statErr := os.Stat(rippedFile); statErr == nil {
		ripSize = fi.Size()
	}
	sink.OnLog(state.LogLevelInfo, "MakeMKV: rip complete, %s in %s",
		pipelines.HumanBytes(ripSize), pipelines.HumanDuration(time.Since(ripStart)))
	sink.OnStepDone(state.StepRip, map[string]any{"title_id": picked.ID, "duration_sec": picked.DurationSec})

	// move — atomic rename of the MakeMKV output directly into the library.
	sink.OnStepStart(state.StepMove)
	rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, pipelines.OutputFields{
		Title: disc.Title, Year: disc.Year,
	})
	if err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	dst := filepath.Join(h.deps.LibraryRoot, rel)
	if err := pipelines.AtomicMove(rippedFile, dst); err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	sink.OnLog(state.LogLevelInfo, "move: → %s", dst)
	sink.OnStepDone(state.StepMove, map[string]any{"path": dst})

	pipelines.RunNotifyStep(ctx, sink, pipelines.NotifyDeps{
		Tools:          h.deps.Tools,
		URLsForTrigger: h.deps.URLsForTrigger,
		LibraryRoot:    h.deps.LibraryRoot,
	}, disc)
	pipelines.RunEjectStep(ctx, sink, pipelines.EjectDeps{
		Tools:       h.deps.Tools,
		ShouldEject: h.deps.ShouldEject,
	}, drv)
	return nil
}

func (h *Handler) createWorkDir(discID string) (string, error) {
	return pipelines.CreateWorkDir(h.deps.WorkRoot, "uhd", discID)
}

func pickLongestTitle(titles []tools.MakeMKVTitle, prof *state.Profile) (tools.MakeMKVTitle, error) {
	minSec := 0
	if prof != nil && prof.Options != nil {
		switch n := prof.Options["min_title_seconds"].(type) {
		case int:
			minSec = n
		case float64:
			minSec = int(n)
		}
	}
	var best tools.MakeMKVTitle
	found := false
	for _, t := range titles {
		if t.DurationSec < minSec {
			continue
		}
		if !found || t.DurationSec > best.DurationSec {
			best = t
			found = true
		}
	}
	if !found {
		return tools.MakeMKVTitle{}, fmt.Errorf("no title with duration >= %ds", minSec)
	}
	return best, nil
}

func singleMKVIn(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".mkv" {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .mkv produced in %s", dir)
}
