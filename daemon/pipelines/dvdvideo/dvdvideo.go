// Package dvdvideo implements pipelines.Handler for DVD-Video discs.
package dvdvideo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

// DVDMirror mirrors a CSS-protected DVD-Video disc's VIDEO_TS to a
// local directory using dvdbackup + libdvdcss. Returns the path to
// the produced `<VOLUME_LABEL>/` directory (one level above
// VIDEO_TS/), which HandBrake's `--input` can read directly.
type DVDMirror interface {
	Mirror(ctx context.Context, devPath, outDir string, sink tools.Sink) (string, error)
}

// HandBrakeScanner enumerates titles in a local VIDEO_TS tree. The
// transcode step uses it (after the rip step has produced the local
// copy) to discover title durations for movie / series selection.
type HandBrakeScanner interface {
	Scan(ctx context.Context, source string) ([]tools.HandBrakeTitle, error)
}

// Deps bundles the handler's dependencies for mock injection.
// MetadataStore is the thin slice of *state.Store the pipeline needs
// to update disc.metadata_json mid-run (e.g. to persist the scan title
// list for the pane's Files tab). Nil-safe; the handler skips the
// write when this is unset.
type MetadataStore interface {
	UpdateDiscMetadataBlob(ctx context.Context, id string, blob string) error
}

type Deps struct {
	Prober           identify.DVDProber
	TMDB             identify.TMDBClient
	DVDBackup        DVDMirror
	HandBrakeScanner HandBrakeScanner
	Tools            *tools.Registry
	LibraryRoot      string
	WorkRoot         string
	LibraryProbe     func(string) error
	URLsForTrigger   func(ctx context.Context, trigger string) []string
	SubsLang         string        // e.g. "eng"; empty → no --subtitle-lang-list flag
	MetadataStore    MetadataStore // optional; pipeline persists scan title list when set

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

// Run executes the DVD-Video pipeline. The rip step uses dvdbackup
// (GPL, libdvdcss-backed) to mirror the disc's VIDEO_TS into a local
// workdir; the transcode step then asks HandBrake to scan the local
// tree, picks titles by profile, and encodes each one from the
// filesystem. HandBrake never touches /dev/sr0, so a spurious kernel
// media-change uevent during a long read can't truncate the output.
// MakeMKV is no longer needed for DVD — the rolling beta-key dance
// is restricted to BDMV / UHD where MakeMKV is the only viable
// decoder.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — dvdbackup mirror of the entire DVD-Video tree.
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
	if h.deps.DVDBackup == nil {
		err := errors.New("dvdvideo: DVDBackup not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	if h.deps.HandBrakeScanner == nil {
		err := errors.New("dvdvideo: HandBrakeScanner not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	ripDir := filepath.Join(tmpdir, "rip")
	if err := os.MkdirAll(ripDir, 0o755); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("create rip dir: %w", err)
	}
	source, err := h.deps.DVDBackup.Mirror(ctx, drv.DevPath, ripDir, newStepSink(sink, state.StepRip))
	if err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("dvdbackup mirror: %w", err)
	}
	sink.OnStepDone(state.StepRip, map[string]any{"source": source})

	// transcode — HandBrake scans the local VIDEO_TS, we pick titles
	// by profile, then HandBrake encodes each one from the local mirror.
	sink.OnStepStart(state.StepTranscode)
	titles, err := h.deps.HandBrakeScanner.Scan(ctx, source)
	if err != nil {
		sink.OnStepFailed(state.StepTranscode, err)
		return fmt.Errorf("handbrake scan: %w", err)
	}
	logScannedTitles(disc.ID, titles)
	warnOnRuntimeMismatch(disc, titles)
	if h.deps.MetadataStore != nil {
		_ = mergeMetadataField(ctx, h.deps.MetadataStore, disc.ID, disc.MetadataJSON, "dvd_titles", titles)
	}

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
	isMovie := IsMovieProfile(prof)

	// Movie profiles delegate title selection to HandBrake's own
	// `--main-feature` flag, which reads the IFO's main-feature bit
	// rather than guessing by duration. Series profiles still need
	// our scan-and-filter logic to enumerate episode titles.
	encodeTitles := selectEncodeTitles(titles, prof)
	if !isMovie && len(encodeTitles) == 0 {
		err := errors.New("no titles to encode")
		sink.OnStepFailed(state.StepTranscode, err)
		return err
	}
	if isMovie {
		// Single encode using --main-feature. encodeTitles is set to
		// the scan's longest title only so the duration-floor check
		// below has a number to compare the output bytes to.
		encodeTitles = []tools.HandBrakeTitle{longestTitle(titles)}
		if err := validateMovieTitleSelection(encodeTitles[0], prof); err != nil {
			sink.OnStepFailed(state.StepTranscode, err)
			return err
		}
	}

	transcoded := make([]string, 0, len(encodeTitles))
	for i, t := range encodeTitles {
		titleIdx := i + 1
		out := filepath.Join(tmpdir, fmt.Sprintf("title%02d.%s", t.Number, ext))
		args := []string{
			"--input", source,
			"--output", out,
			"--quality", "20",
			"--encoder", "x264",
			"--all-audio",
			"--markers",
		}
		if isMovie {
			args = append(args, "--main-feature")
		} else {
			args = append(args, "--title", strconv.Itoa(t.Number))
		}
		if h.deps.SubsLang != "" {
			args = append(args, "--subtitle-lang-list", h.deps.SubsLang, "--subtitle-forced=auto")
		}
		if ext == "mp4" {
			args = append(args, "--optimize")
		}
		env := map[string]string{
			"HB_TITLE_IDX":    strconv.Itoa(titleIdx),
			"HB_TOTAL_TITLES": strconv.Itoa(len(encodeTitles)),
		}
		stepSink := newStepSink(sink, state.StepTranscode)
		if err := whb.Run(ctx, args, env, tmpdir, stepSink); err != nil {
			sink.OnStepFailed(state.StepTranscode, err)
			return fmt.Errorf("handbrake encode title %d: %w", t.Number, err)
		}
		if err := validateEncodedTitle(out, t.DurationSeconds, h.deps.MinEncodedBytesPerSecond); err != nil {
			sink.OnStepFailed(state.StepTranscode, err)
			return fmt.Errorf("handbrake encode title %d: %w", t.Number, err)
		}
		transcoded = append(transcoded, out)
	}
	sink.OnStepDone(state.StepTranscode, nil)

	// move
	sink.OnStepStart(state.StepMove)
	moved, err := h.moveOutputs(transcoded, encodeTitles, disc, prof)
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

// minEncodedBytesPerSecond is our lower-bound on the bytes-per-second
// of a HandBrake x264 quality-20 encode. Real movies hover around
// 200 KB/s (≈ 1.5 Mbps); we use 93 750 (≈ 750 kbps) so the check
// rejects truncated encodes (HandBrake exiting cleanly mid-stream)
// without false-positives on extremely flat content.
const minEncodedBytesPerSecond = 93_750

// minMovieFeatureSeconds is the default floor (20 min) below which we
// refuse to start a movie-profile encode. --main-feature handles the
// happy path, but a disc with no main-feature bit set in the IFO (or
// an incomplete dvdbackup mirror) can still leave the scan's longest
// title at a few minutes — see the Jackass: The Movie regression that
// shipped a 7-min sketch in v0.2.3. Failing here is preferable to
// producing a junk file that passes the downstream byte-size check
// (which only compares against the *encoded* duration, not the
// expected feature duration). Override per profile via
// `min_feature_seconds`; set to 0 to disable.
const minMovieFeatureSeconds = 1200

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

// selectEncodeTitles picks which titles to encode for **series**
// profiles. Movie profiles bypass this entirely and let HandBrake's
// --main-feature pick from the IFO.
//
// Series (MKV) profile: every title >= options.min_title_seconds
// (default 300).
func selectEncodeTitles(titles []tools.HandBrakeTitle, prof *state.Profile) []tools.HandBrakeTitle {
	if IsMovieProfile(prof) {
		// Movie path is driven by --main-feature; the caller still needs
		// *something* to iterate to drive its outer loop once, so it
		// substitutes longestTitle(titles) directly. Returning nil here
		// makes the no-titles guard in Run fire only for series, which is
		// the correct semantics.
		return nil
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
	var out []tools.HandBrakeTitle
	for _, t := range titles {
		if t.DurationSeconds >= minSec {
			out = append(out, t)
		}
	}
	return out
}

// IsMovieProfile decides whether the DVD-Movie title-selection path
// (HandBrake --main-feature) applies, or whether the DVD-Series path
// (enumerate-and-floor) applies. Resolution order:
//
//  1. profile option "dvd_selection_mode" — "main_feature" / "per_title"
//  2. legacy fallback: lowercased Format == "mp4" means movie
//
// The legacy fallback exists for DBs that haven't yet picked up the
// 003_dvd_default_mkv migration (e.g. a freshly-restored backup from
// before that migration shipped).
func IsMovieProfile(prof *state.Profile) bool {
	if mode, ok := prof.Options["dvd_selection_mode"].(string); ok {
		switch mode {
		case "main_feature":
			return true
		case "per_title":
			return false
		}
	}
	return strings.ToLower(prof.Format) == "mp4"
}

// longestTitle returns the title with the largest DurationSeconds, or
// a zero HandBrakeTitle when titles is empty. Used as the validation
// reference (expected duration) for movie-profile encodes that
// HandBrake selected via --main-feature.
func longestTitle(titles []tools.HandBrakeTitle) tools.HandBrakeTitle {
	if len(titles) == 0 {
		return tools.HandBrakeTitle{}
	}
	best := titles[0]
	for _, t := range titles[1:] {
		if t.DurationSeconds > best.DurationSeconds {
			best = t
		}
	}
	return best
}

// validateMovieTitleSelection rejects movie-profile encodes when the
// longest scanned title is below the configured feature floor. The
// longest scanned title is also what `validateEncodedTitle` later
// compares the output bytes against, so a too-short pick here means
// the byte-size check would also be using a too-short reference and
// would pass on a junk encode. Returns nil when the profile sets
// `min_feature_seconds=0`.
func validateMovieTitleSelection(picked tools.HandBrakeTitle, prof *state.Profile) error {
	floor := minMovieFeatureSeconds
	if v, ok := prof.Options["min_feature_seconds"]; ok {
		switch n := v.(type) {
		case int:
			floor = n
		case float64:
			floor = int(n)
		}
	}
	if floor <= 0 {
		return nil
	}
	if picked.DurationSeconds < floor {
		return fmt.Errorf(
			"longest scanned title is %ds, below movie feature floor of %ds — disc likely has no play-all title or the mirror is incomplete; set profile option min_feature_seconds=0 to override",
			picked.DurationSeconds, floor,
		)
	}
	return nil
}

// logScannedTitles emits one INFO line per title HandBrake's scan
// returned. Cheap, but invaluable when a future "wrong title got
// picked" regression needs to be diagnosed from `docker logs`.
func logScannedTitles(discID string, titles []tools.HandBrakeTitle) {
	for _, t := range titles {
		slog.Info("scanned title",
			"disc", discID, "title", t.Number, "duration_sec", t.DurationSeconds)
	}
}

// warnOnRuntimeMismatch logs a WARN when the longest scanned title
// diverges by more than 50 % from the disc's TMDB-reported runtime.
// Doesn't fail — DVDs legitimately differ from theatrical runtimes
// (director's cuts, regional edits) — but a 5× gap is a red flag
// that the rip captured the wrong content (e.g. an outtakes reel
// instead of the feature).
func warnOnRuntimeMismatch(disc *state.Disc, titles []tools.HandBrakeTitle) {
	if disc == nil || disc.RuntimeSeconds <= 0 {
		return
	}
	longest := longestTitle(titles)
	if longest.DurationSeconds <= 0 {
		return
	}
	expected := float64(disc.RuntimeSeconds)
	got := float64(longest.DurationSeconds)
	ratio := got / expected
	if ratio < 0.5 || ratio > 1.5 {
		slog.Warn("duration mismatch",
			"disc", disc.ID,
			"expected_sec", disc.RuntimeSeconds,
			"scanned_longest_sec", longest.DurationSeconds,
			"ratio", fmt.Sprintf("%.2f", ratio),
		)
	}
}

func (h *Handler) moveOutputs(transcoded []string, _ []tools.HandBrakeTitle,
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

// mergeMetadataField reads the current blob, sets one top-level key to
// value, and persists the merged JSON. Failures are non-fatal — the
// rip continues regardless. Used to attach the HandBrake scan title
// list onto disc.metadata_json so the pane's Files tab can render the
// source-disc inventory after the rip completes too.
func mergeMetadataField(ctx context.Context, store MetadataStore, discID, existing string, key string, value any) error {
	merged := map[string]any{}
	if existing != "" && existing != "{}" {
		_ = json.Unmarshal([]byte(existing), &merged)
	}
	merged[key] = value
	body, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	return store.UpdateDiscMetadataBlob(ctx, discID, string(body))
}
