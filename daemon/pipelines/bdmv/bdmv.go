// Package bdmv implements pipelines.Handler for Blu-ray (BDMV) discs.
//
// Pipeline shape (7 active steps; compress skipped):
//
//	detect → identify → rip (MakeMKV) → transcode (HandBrake)
//	    → move → notify → eject
//
// MakeMKV decrypts AACS and demuxes the chosen title to .mkv;
// HandBrake transcodes that .mkv to the profile's preset; the result
// atomic-moves into the library.
package bdmv

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

// MakeMKVScanner is the slice of tools.MakeMKV used at scan-time.
type MakeMKVScanner interface {
	Scan(ctx context.Context, devPath string) ([]tools.MakeMKVTitle, error)
}

// MakeMKVRipper is the slice of tools.MakeMKV used at rip-time.
type MakeMKVRipper interface {
	Rip(ctx context.Context, devPath string, titleID int, outDir string, sink tools.Sink) error
}

// Deps bundles the handler's dependencies for mock injection.
type Deps struct {
	Prober         identify.DVDProber // re-used for volume-label reading
	TMDB           identify.TMDBClient
	MakeMKVScanner MakeMKVScanner
	MakeMKVRipper  MakeMKVRipper
	Tools          *tools.Registry // looked up: handbrake, apprise, eject
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	SubsLang       string

	// NVENCAvailable signals that NVIDIA NVENC is usable on the host.
	// When true and the profile requests an nvenc_* video_codec, the
	// transcode step passes the hardware encoder to HandBrake.
	// When false, NVENC profile values fall back to the closest
	// software encoder. BDMV's pipeline has always emitted 10-bit
	// HEVC, so software h264/h265 results are promoted to x265_10bit
	// to preserve bit-depth.
	NVENCAvailable bool
}

// Handler implements pipelines.Handler for BDMV.
type Handler struct{ deps Deps }

// New constructs the handler.
func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

// DiscType returns BDMV.
func (h *Handler) DiscType() state.DiscType { return state.DiscTypeBDMV }

// Identify reads the volume label and queries TMDB.
//
//   - Junk label → ErrNoCandidates
//   - TMDB returns 0 → ErrNoCandidates
//   - Otherwise → Disc with title+year+TMDB id from highest-confidence match
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	if h.deps.Prober == nil {
		return nil, nil, errors.New("bdmv: prober not configured")
	}
	if h.deps.TMDB == nil {
		return nil, nil, errors.New("bdmv: TMDB client not configured")
	}

	info, err := h.deps.Prober.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("bd probe: %w", err)
	}
	disc := &state.Disc{Type: state.DiscTypeBDMV, DriveID: drv.ID}

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

// Plan returns the 7-active-step plan; only compress is skipped.
func (h *Handler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	skipped := map[state.StepID]bool{state.StepCompress: true}
	out := make([]pipelines.StepPlan, 0, 8)
	for _, sid := range state.CanonicalSteps() {
		out = append(out, pipelines.StepPlan{ID: sid, Skip: skipped[sid]})
	}
	return out
}

// Run executes the BDMV pipeline.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — MakeMKV scan + decrypt+demux of the chosen title.
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
		err := errors.New("bdmv: MakeMKV not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	sink.OnLog(state.LogLevelInfo, "MakeMKV: scanning %s", drv.DevPath)
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
	if err := h.deps.MakeMKVRipper.Rip(ctx, drv.DevPath, picked.ID, ripDir, newStepSink(sink, state.StepRip)); err != nil {
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

	// transcode — HandBrake reads the rip and writes a transcoded mkv.
	sink.OnStepStart(state.StepTranscode)
	transcodedFile := filepath.Join(tmpdir, "out.mkv")
	if h.deps.Tools == nil {
		err := errors.New("bdmv: tools registry not configured")
		sink.OnStepFailed(state.StepTranscode, err)
		return err
	}
	hb, ok := h.deps.Tools.Get("handbrake")
	if !ok {
		err := errors.New("bdmv: handbrake tool not registered")
		sink.OnStepFailed(state.StepTranscode, err)
		return err
	}
	// Select the HandBrake encoder: NVENC if the profile asks for it
	// and the GPU was detected at boot, software otherwise. BDMV's
	// pipeline has always emitted 10-bit HEVC, so promote any software
	// h264/h265 result to x265_10bit (covers empty VideoCodec, explicit
	// x265, and fallback from nvenc_h265). NVENC values pass through;
	// hardware 10-bit NVENC needs an explicit --encoder-profile flag
	// and is tracked as a follow-up.
	encoder, fellBack := pipelines.SelectHandBrakeEncoder(prof, h.deps.NVENCAvailable)
	switch encoder {
	case "x264", "x265":
		encoder = "x265_10bit"
	}
	if fellBack {
		sink.OnLog(state.LogLevelWarn,
			"NVENC requested but unavailable on host; falling back to %s software encoder", encoder)
	}
	hbArgs := []string{
		"--input", rippedFile,
		"--output", transcodedFile,
		"--format", "av_mkv",
		"--encoder", encoder,
		"--quality", "19",
		"--all-audio",
		"--markers",
	}
	if h.deps.SubsLang != "" {
		hbArgs = append(hbArgs, "--subtitle-lang-list", h.deps.SubsLang, "--subtitle-forced=auto")
	}
	sink.OnLog(state.LogLevelInfo, "HandBrake: encoding %s", filepath.Base(rippedFile))
	encStart := time.Now()
	if err := hb.Run(ctx, hbArgs, nil, tmpdir, newStepSink(sink, state.StepTranscode)); err != nil {
		sink.OnStepFailed(state.StepTranscode, err)
		return fmt.Errorf("handbrake encode: %w", err)
	}
	var encSize int64
	if fi, statErr := os.Stat(transcodedFile); statErr == nil {
		encSize = fi.Size()
	}
	sink.OnLog(state.LogLevelInfo, "HandBrake: encode complete, %s in %s",
		pipelines.HumanBytes(encSize), pipelines.HumanDuration(time.Since(encStart)))
	sink.OnStepDone(state.StepTranscode, nil)

	// move — atomic rename to library.
	sink.OnStepStart(state.StepMove)
	rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, pipelines.OutputFields{
		Title: disc.Title, Year: disc.Year,
	})
	if err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	dst := filepath.Join(h.deps.LibraryRoot, rel)
	if err := pipelines.AtomicMove(transcodedFile, dst); err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return err
	}
	sink.OnLog(state.LogLevelInfo, "move: → %s", dst)
	sink.OnStepDone(state.StepMove, map[string]any{"path": dst})

	// notify
	sink.OnStepStart(state.StepNotify)
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
	sink.OnStepDone(state.StepNotify, nil)

	// eject
	sink.OnStepStart(state.StepEject)
	if eject, ok := h.deps.Tools.Get("eject"); ok && drv != nil && drv.DevPath != "" {
		if err := eject.Run(ctx, []string{drv.DevPath}, nil, "", newStepSink(sink, state.StepEject)); err != nil {
			sink.OnStepFailed(state.StepEject, err)
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
	dir := filepath.Join(root, "discecho-bdmv-"+discID+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	return dir, nil
}

// pickLongestTitle selects the longest title that meets
// options.min_title_seconds. Returns an error if no title qualifies.
func pickLongestTitle(titles []tools.MakeMKVTitle, prof *state.Profile) (tools.MakeMKVTitle, error) {
	min := 0
	if prof != nil && prof.Options != nil {
		switch n := prof.Options["min_title_seconds"].(type) {
		case int:
			min = n
		case float64:
			min = int(n)
		}
	}
	var best tools.MakeMKVTitle
	found := false
	for _, t := range titles {
		if t.DurationSec < min {
			continue
		}
		if !found || t.DurationSec > best.DurationSec {
			best = t
			found = true
		}
	}
	if !found {
		return tools.MakeMKVTitle{}, fmt.Errorf("no title with duration >= %ds", min)
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
