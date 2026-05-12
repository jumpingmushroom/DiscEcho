// Package dvdvideo implements pipelines.Handler for DVD-Video discs.
package dvdvideo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// MakeMKVScanner is the slice of tools.MakeMKV used by the rip step to
// list titles. Substitutable so tests can supply a fake without a real
// makemkvcon binary.
type MakeMKVScanner interface {
	Scan(ctx context.Context, devPath string) ([]tools.MakeMKVTitle, error)
}

// MakeMKVRipper is the slice of tools.MakeMKV that performs the actual
// CSS-decrypted read into a workdir.
type MakeMKVRipper interface {
	Rip(ctx context.Context, devPath string, titleID int, outDir string, sink tools.Sink) error
}

// Deps bundles the handler's dependencies for mock injection.
type Deps struct {
	Prober         identify.DVDProber
	TMDB           identify.TMDBClient
	MakeMKVScanner MakeMKVScanner
	MakeMKVRipper  MakeMKVRipper
	Tools          *tools.Registry
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	SubsLang       string // e.g. "eng"; empty → no --subtitle-lang-list flag

	// MinEncodedBytesPerSecond is the lower-bound bytes-per-second the
	// encoded output must hit for the transcode step to be considered
	// successful. 0 → use the package default (≈ 750 kbps). A negative
	// value disables the check (used by tests with stub encoders that
	// don't write real-sized output).
	MinEncodedBytesPerSecond int
}

// Handler implements pipelines.Handler for DVD-Video.
type Handler struct {
	deps Deps
}

// New constructs the handler.
func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

// DiscType returns DVD.
func (h *Handler) DiscType() state.DiscType { return state.DiscTypeDVD }

// Identify reads the DVD volume label and queries TMDB.
//
//   - Garbage label → ErrNoCandidates
//   - TMDB returns 0 → ErrNoCandidates (UI prompts manual)
//   - Otherwise → Disc with title+year+TMDB id from highest-confidence candidate
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	if h.deps.Prober == nil {
		return nil, nil, errors.New("dvdvideo: prober not configured")
	}
	if h.deps.TMDB == nil {
		return nil, nil, errors.New("dvdvideo: TMDB client not configured")
	}

	info, err := h.deps.Prober.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("dvd probe: %w", err)
	}
	disc := &state.Disc{
		Type:    state.DiscTypeDVD,
		DriveID: drv.ID,
	}
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

// Plan returns the 8-step plan; only `compress` is skipped for DVD.
func (h *Handler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	skipped := map[state.StepID]bool{state.StepCompress: true}
	out := make([]pipelines.StepPlan, 0, 8)
	for _, sid := range state.CanonicalSteps() {
		out = append(out, pipelines.StepPlan{ID: sid, Skip: skipped[sid]})
	}
	return out
}

// Run executes the DVD-Video pipeline. Architecture mirrors BDMV:
// MakeMKV reads the encrypted disc and writes one MKV per selected
// title into the workdir; HandBrake then transcodes each MKV from
// the local filesystem. HandBrake never touches /dev/sr0 — the old
// "HandBrake --input /dev/sr0" path was brittle in the face of
// mid-rip kernel media-change uevents, which produced truncated mp4s
// that HandBrake exit-0'd into the library.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — MakeMKV scan + decrypt+demux of the selected titles.
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
		err := errors.New("dvdvideo: MakeMKV not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	titles, err := h.deps.MakeMKVScanner.Scan(ctx, drv.DevPath)
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("makemkv scan: %w", err)
	}

	encodeTitles := selectEncodeTitles(titles, prof)
	if len(encodeTitles) == 0 {
		err := errors.New("no titles to encode")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	rips := make([]rippedTitle, 0, len(encodeTitles))
	for _, t := range encodeTitles {
		ripDir := filepath.Join(tmpdir, "rip", fmt.Sprintf("title%02d", t.ID))
		if err := os.MkdirAll(ripDir, 0o755); err != nil {
			sink.OnStepFailed(state.StepRip, err)
			return fmt.Errorf("create rip dir: %w", err)
		}
		if err := h.deps.MakeMKVRipper.Rip(ctx, drv.DevPath, t.ID, ripDir, newStepSink(sink, state.StepRip)); err != nil {
			sink.OnStepFailed(state.StepRip, err)
			return fmt.Errorf("makemkv rip title %d: %w", t.ID, err)
		}
		mkv, err := singleMKVIn(ripDir)
		if err != nil {
			sink.OnStepFailed(state.StepRip, err)
			return fmt.Errorf("makemkv rip title %d: %w", t.ID, err)
		}
		rips = append(rips, rippedTitle{title: t, mkv: mkv})
	}
	sink.OnStepDone(state.StepRip, map[string]any{"title_count": len(encodeTitles)})

	// transcode — one HandBrake encode per ripped MKV. Reads from the
	// local filesystem; /dev/sr0 is no longer involved.
	sink.OnStepStart(state.StepTranscode)
	whb, ok := h.deps.Tools.Get("handbrake")
	if !ok {
		err := errors.New("dvdvideo: handbrake tool not registered")
		sink.OnStepFailed(state.StepTranscode, err)
		return err
	}
	ext := strings.ToLower(prof.Format)
	if ext != "mp4" && ext != "mkv" {
		ext = "mkv"
	}
	transcoded := make([]string, 0, len(rips))
	for i, r := range rips {
		titleIdx := i + 1
		out := filepath.Join(tmpdir, fmt.Sprintf("title%02d.%s", r.title.ID, ext))
		args := []string{
			"--input", r.mkv,
			"--output", out,
			"--quality", "20",
			"--encoder", "x264",
			"--all-audio",
			"--markers",
		}
		if h.deps.SubsLang != "" {
			args = append(args, "--subtitle-lang-list", h.deps.SubsLang, "--subtitle-forced=auto")
		}
		if ext == "mp4" {
			args = append(args, "--optimize")
		}
		env := map[string]string{
			"HB_TITLE_IDX":    strconv.Itoa(titleIdx),
			"HB_TOTAL_TITLES": strconv.Itoa(len(rips)),
		}
		stepSink := newStepSink(sink, state.StepTranscode)
		if err := whb.Run(ctx, args, env, tmpdir, stepSink); err != nil {
			sink.OnStepFailed(state.StepTranscode, err)
			return fmt.Errorf("handbrake encode title %d: %w", r.title.ID, err)
		}
		if err := validateEncodedTitle(out, r.title.DurationSec, h.deps.MinEncodedBytesPerSecond); err != nil {
			sink.OnStepFailed(state.StepTranscode, err)
			return fmt.Errorf("handbrake encode title %d: %w", r.title.ID, err)
		}
		transcoded = append(transcoded, out)
	}
	sink.OnStepDone(state.StepTranscode, nil)

	// move
	sink.OnStepStart(state.StepMove)
	moved, err := h.moveOutputs(transcoded, rips, disc, prof)
	if err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return fmt.Errorf("move: %w", err)
	}
	sink.OnStepDone(state.StepMove, map[string]any{"paths": moved})

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

// rippedTitle pairs a MakeMKV title with the path of its produced .mkv
// in the workdir, so the transcode step can find the right input file
// and the move step still knows the original title metadata for
// series-episode naming.
type rippedTitle struct {
	title tools.MakeMKVTitle
	mkv   string
}

// minEncodedBytesPerSecond is our lower-bound on the bytes-per-second
// of a HandBrake x264 quality-20 encode. Real movies hover around
// 200 KB/s (≈ 1.5 Mbps). Pre-v0.2.0 we used 37 500 (≈ 300 kbps),
// which let a 170 MB truncated 85-min mp4 (≈ 270 kbps) slip through
// during the homelab test. Bumping to 93 750 (≈ 750 kbps) keeps
// enough headroom for flat content while rejecting the truncations
// we actually saw.
const minEncodedBytesPerSecond = 93_750

func singleMKVIn(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".mkv") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .mkv produced in %s", dir)
}

// validateEncodedTitle errors out when the encoded file is missing, is
// empty, or is below the expected lower-bound for its source duration.
// HandBrakeCLI exits 0 in several end-of-stream failure modes, so we
// can't rely on the exit code alone to know whether the title encoded
// in full.
//
// minBytesPerSecond overrides the package default; 0 → default, < 0 →
// disable the size check (only the empty-file branch applies).
func validateEncodedTitle(path string, durationSeconds, minBytesPerSecond int) error {
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("validate encode: %w", err)
	}
	if fi.Size() == 0 {
		return fmt.Errorf("validate encode: empty output at %s", path)
	}
	if durationSeconds <= 0 || minBytesPerSecond < 0 {
		return nil
	}
	if minBytesPerSecond == 0 {
		minBytesPerSecond = minEncodedBytesPerSecond
	}
	minSize := int64(durationSeconds) * int64(minBytesPerSecond)
	if fi.Size() < minSize {
		return fmt.Errorf(
			"validate encode: output %s is %d bytes, expected at least %d for a %ds title (likely truncated)",
			path, fi.Size(), minSize, durationSeconds,
		)
	}
	return nil
}

func (h *Handler) createWorkDir(discID string) (string, error) {
	root := h.deps.WorkRoot
	if root == "" {
		root = os.TempDir()
	}
	dir := filepath.Join(root, "discecho-dvd-"+discID+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create workdir: %w", err)
	}
	return dir, nil
}

// selectEncodeTitles picks which titles to encode based on profile.
// Movie (MP4) profile: longest title only.
// Series (MKV) profile: every title >= options.min_title_seconds (default 300).
func selectEncodeTitles(titles []tools.MakeMKVTitle, prof *state.Profile) []tools.MakeMKVTitle {
	if strings.ToLower(prof.Format) == "mp4" {
		if len(titles) == 0 {
			return nil
		}
		best := titles[0]
		for _, t := range titles[1:] {
			if t.DurationSec > best.DurationSec {
				best = t
			}
		}
		return []tools.MakeMKVTitle{best}
	}

	minSec := 300
	if v, ok := prof.Options["min_title_seconds"]; ok {
		switch n := v.(type) {
		case int:
			minSec = n
		case float64:
			minSec = int(n)
		}
	}
	var out []tools.MakeMKVTitle
	for _, t := range titles {
		if t.DurationSec >= minSec {
			out = append(out, t)
		}
	}
	return out
}

func (h *Handler) moveOutputs(transcoded []string, rips []rippedTitle,
	disc *state.Disc, prof *state.Profile) ([]string, error) {
	season := 1
	if v, ok := prof.Options["season"]; ok {
		switch n := v.(type) {
		case int:
			season = n
		case float64:
			season = int(n)
		}
	}

	var moved []string
	for episodeIdx, src := range transcoded {
		// We want the file extension that came out of HandBrake, not the
		// profile's extension — they always agree today, but stay
		// defensive in case a profile flips formats mid-job.
		ext := strings.TrimPrefix(filepath.Ext(src), ".")
		fields := pipelines.OutputFields{
			Title:         disc.Title,
			Year:          disc.Year,
			Show:          disc.Title,
			Season:        season,
			EpisodeNumber: episodeIdx + 1,
		}
		rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, fields)
		if err != nil {
			return moved, fmt.Errorf("render template: %w", err)
		}
		if filepath.Ext(rel) == "" {
			rel += "." + ext
		}
		dst := filepath.Join(h.deps.LibraryRoot, rel)
		if err := pipelines.AtomicMove(src, dst); err != nil {
			return moved, err
		}
		moved = append(moved, dst)
	}
	_ = rips // future: per-title overrides; for now structure is symmetric with transcoded.
	return moved, nil
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
