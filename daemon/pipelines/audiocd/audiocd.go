// Package audiocd implements pipelines.Handler for audio CDs.
package audiocd

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// Deps bundles the handler's dependencies for mock injection.
type Deps struct {
	TOC            identify.TOCReader
	MB             identify.MusicBrainzClient
	Tools          *tools.Registry
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error                                 // defaults to pipelines.ProbeWritable
	URLsForTrigger func(ctx context.Context, trigger string) []string // returns Apprise URLs; nil → no notifications
	// ShouldEject gates the rip-end eject step; nil = always eject.
	ShouldEject func(ctx context.Context) bool
}

// Handler implements pipelines.Handler for audio CDs.
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

// DiscType returns AUDIO_CD.
func (h *Handler) DiscType() state.DiscType { return state.DiscTypeAudioCD }

// Identify reads the TOC, computes the MusicBrainz disc ID, and looks
// up release candidates. Returns ErrNoCandidates if MB returns 0 hits.
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	if h.deps.TOC == nil {
		return nil, nil, errors.New("audiocd: TOC reader not configured")
	}
	if h.deps.MB == nil {
		return nil, nil, errors.New("audiocd: MusicBrainz client not configured")
	}

	toc, err := h.deps.TOC.Read(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("toc read: %w", err)
	}
	lbas := make([]int, 0, len(toc.Tracks))
	for _, t := range toc.Tracks {
		lbas = append(lbas, t.StartLBA)
	}
	discID := identify.DiscID(toc.FirstTrack(), toc.LastTrack(), toc.LeadoutLBA, lbas)

	cands, err := h.deps.MB.Lookup(ctx, discID)
	if err != nil {
		return nil, nil, fmt.Errorf("musicbrainz lookup: %w", err)
	}

	disc := &state.Disc{
		Type:           state.DiscTypeAudioCD,
		DriveID:        drv.ID,
		TOCHash:        discID,
		Candidates:     cands,
		RuntimeSeconds: lbasToSeconds(toc),
	}
	if len(cands) > 0 {
		sort.Slice(cands, func(i, j int) bool { return cands[i].Confidence > cands[j].Confidence })
		top := cands[0]
		disc.Title = top.Title
		disc.Year = top.Year
		disc.MetadataProvider = top.Source
		disc.MetadataID = top.MBID
		disc.Candidates = cands
	}

	if len(cands) == 0 {
		return disc, nil, pipelines.ErrNoCandidates
	}
	return disc, cands, nil
}

// Plan returns the 8-step plan with transcode + compress skipped.
func (h *Handler) Plan(_ *state.Disc, _ *state.Profile) []pipelines.StepPlan {
	skipped := map[state.StepID]bool{
		state.StepTranscode: true,
		state.StepCompress:  true,
	}
	out := make([]pipelines.StepPlan, 0, 8)
	for _, sid := range state.CanonicalSteps() {
		out = append(out, pipelines.StepPlan{ID: sid, Skip: skipped[sid]})
	}
	return out
}

// Run executes the audio CD pipeline. drv supplies dev_path for eject.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	// detect + identify already happened; mark them done for the UI.
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip
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

	whipper, ok := h.deps.Tools.Get("whipper")
	if !ok {
		err := errors.New("audiocd: whipper tool not registered")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}
	// Whipper 0.10's `cd rip` doesn't accept a `--keep-bad-files` flag;
	// passing one trips Python's argparse and the process exits 2 before
	// any disc reads happen. The default behaviour (fail the run if a
	// track can't be ripped) is what we want.
	//
	// `-d` is on the `cd` subcommand and must come before `rip`. We pass
	// the drive's `dev_path` explicitly because the daemon's container
	// only exposes `/dev/sr0`/`/dev/sr1`, not the `/dev/cdrom` symlink
	// whipper falls back to.
	//
	// `-o 0` supplies a runtime sample-offset so whipper doesn't abort
	// with "drive offset unconfigured". The canonical workflow is
	// `whipper offset find` once per drive against a CD known to
	// AccurateRip — but that requires pycdio and a calibration disc,
	// neither of which we can assume in a homelab container. Offset=0
	// produces a rip that's audibly identical to a calibrated one
	// (~6 samples / 0.14 ms typical drift) but won't match AccurateRip
	// checksums. Audiophiles who care can run `whipper offset find`
	// inside the container manually and override this default.
	devPath := drv.DevPath
	if devPath == "" {
		devPath = "/dev/cdrom"
	}
	args := []string{"cd", "-d", devPath, "rip", "-R", disc.MetadataID,
		"-o", "0", "--working-directory", tmpdir}
	if err := whipper.Run(ctx, args, nil, tmpdir, pipelines.NewStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("whipper: %w", err)
	}
	sink.OnStepDone(state.StepRip, nil)

	// move
	sink.OnStepStart(state.StepMove)
	moved, err := h.moveOutputs(tmpdir, disc, prof)
	if err != nil {
		sink.OnStepFailed(state.StepMove, err)
		return fmt.Errorf("move: %w", err)
	}
	sink.OnStepDone(state.StepMove, map[string]any{"paths": moved})

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
	return pipelines.CreateWorkDir(h.deps.WorkRoot, "", discID)
}

func (h *Handler) moveOutputs(tmpdir string, disc *state.Disc, prof *state.Profile) ([]string, error) {
	// Walk the workdir recursively. whipper writes its output into a
	// nested `album/<Artist> - <Album>/` subdirectory, so a flat
	// os.ReadDir on tmpdir sees only that subdir (skipped as a
	// directory) and the move step "succeeds" with zero files moved —
	// then the deferred RemoveAll wipes the ripped FLACs. Walking the
	// tree picks up the outputs regardless of how deep whipper buries
	// them.
	var candidates []string
	if err := filepath.WalkDir(tmpdir, func(path string, dEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if dEntry.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(dEntry.Name()))
		if ext != ".flac" && ext != ".cue" {
			return nil
		}
		candidates = append(candidates, path)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("walk workdir: %w", err)
	}

	// A successful whipper rip always produces at least one FLAC. If
	// we find none, fail loud rather than silently "succeeding" with
	// an empty paths list — that's how we lost a real rip before.
	flacCount := 0
	for _, p := range candidates {
		if strings.EqualFold(filepath.Ext(p), ".flac") {
			flacCount++
		}
	}
	if flacCount == 0 {
		return nil, fmt.Errorf("move: no FLAC files found under %s", tmpdir)
	}

	var moved []string
	for _, src := range candidates {
		name := filepath.Base(src)
		ext := strings.ToLower(filepath.Ext(name))

		fields := pipelines.OutputFields{
			Album:       disc.Title,
			Year:        disc.Year,
			TrackNumber: trackNumberFromFilename(name),
			Title:       stripTrackPrefix(strings.TrimSuffix(name, ext)),
		}
		if len(disc.Candidates) > 0 {
			fields.Artist = disc.Candidates[0].Artist
		}

		rel, err := pipelines.RenderOutputPath(prof.OutputPathTemplate, fields)
		if err != nil {
			return moved, fmt.Errorf("render template: %w", err)
		}
		if filepath.Ext(rel) == "" {
			rel += ext
		}
		dst := filepath.Join(h.deps.LibraryRoot, rel)
		if err := pipelines.AtomicMove(src, dst); err != nil {
			return moved, err
		}
		moved = append(moved, dst)
	}
	return moved, nil
}

// trackPrefixRE matches the leading track-number marker whipper emits at
// the start of a per-track filename, so it can be removed before the
// remainder is fed to the output-path template as `.Title`. The default
// audio-CD template already prepends `{{printf "%02d" .TrackNumber}} - `,
// and without this strip the rendered name carries the track number
// twice (e.g. `01 - 01. Artist - Title.flac`).
//
// Covers the two shapes we actually see:
//   - real whipper: `NN. Artist - Title` (default whipper template
//     `%a - %d/%t. %a - %n`)
//   - test fakes: `trackNN`
var trackPrefixRE = regexp.MustCompile(`(?i)^(track)?\s*\d+\s*[.\-)\s]*`)

func stripTrackPrefix(name string) string {
	return trackPrefixRE.ReplaceAllString(name, "")
}

func trackNumberFromFilename(name string) int {
	// Strip optional leading "track" prefix
	s := strings.TrimPrefix(strings.ToLower(name), "track")
	for i, r := range s {
		if r < '0' || r > '9' {
			if i == 0 {
				return 0
			}
			n, err := strconv.Atoi(s[:i])
			if err != nil {
				return 0
			}
			return n
		}
	}
	return 0
}

func lbasToSeconds(toc *identify.TOC) int {
	if toc == nil {
		return 0
	}
	total := 0
	for _, t := range toc.Tracks {
		total += t.LengthLBA
	}
	return total / 75 // 75 sectors per second on CDDA
}
