// Package xbox implements pipelines.Handler for original Xbox game discs.
//
// Pipeline shape (6 active steps; transcode AND compress skipped):
//
//	detect → identify → rip (redumper xbox) → move → notify → eject
//
// Identify reads default.xbe off the disc via isoinfo, parses the XBE
// certificate for title ID, and looks up against the user-supplied Redump dat.
// ErrNoCandidates surfaces when the dat is missing OR the title ID is unknown.
package xbox

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// XboxProber reads default.xbe from a disc device and returns parsed XBE info.
type XboxProber interface {
	Probe(ctx context.Context, devPath string) (*identify.XboxInfo, error)
}

// IsoinfoXboxProber implements XboxProber by shelling out to isoinfo to extract
// default.xbe bytes without mounting the disc.
type IsoinfoXboxProber struct {
	// Bin is the isoinfo binary name. Defaults to "isoinfo".
	Bin string
}

func (p *IsoinfoXboxProber) bin() string {
	if p.Bin == "" {
		return "isoinfo"
	}
	return p.Bin
}

// Probe extracts /default.xbe from devPath using `isoinfo -i <devPath> -x /default.xbe`
// and parses the result with identify.ProbeXBE.
func (p *IsoinfoXboxProber) Probe(ctx context.Context, devPath string) (*identify.XboxInfo, error) {
	out, err := exec.CommandContext(ctx, p.bin(), "-i", devPath, "-x", "/default.xbe").Output()
	if err != nil {
		return nil, fmt.Errorf("isoinfo -x /default.xbe: %w", err)
	}
	info, err := identify.ProbeXBE(out)
	if err != nil {
		return nil, err
	}
	return info, nil
}

// RedumperRipper is the subset of *tools.Redumper used at rip-time.
type RedumperRipper interface {
	Rip(ctx context.Context, devPath, outDir, name, mode string, sink tools.Sink) error
}

// Deps bundles the handler's dependencies.
type Deps struct {
	Redumper       RedumperRipper
	XboxProber     XboxProber
	RedumpDB       *identify.RedumpDB
	Tools          *tools.Registry // looked up: apprise, eject
	LibraryRoot    string
	WorkRoot       string
	LibraryProbe   func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	// ShouldEject gates the rip-end eject step; nil = always eject.
	ShouldEject func(ctx context.Context) bool
}

// Handler implements pipelines.Handler for original Xbox.
type Handler struct{ deps Deps }

// New returns a Handler with the given dependencies.
func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

func (h *Handler) DiscType() state.DiscType { return state.DiscTypeXBOX }

// Identify reads default.xbe via isoinfo, looks up title ID in RedumpDB.
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	disc := &state.Disc{Type: state.DiscTypeXBOX, DriveID: drv.ID}

	if h.deps.RedumpDB == nil {
		slog.Warn("xbox: redump xbox.dat missing", "dev", drv.DevPath)
		return disc, nil, pipelines.ErrNoCandidates
	}
	if h.deps.XboxProber == nil {
		return nil, nil, errors.New("xbox: XboxProber not configured")
	}
	info, err := h.deps.XboxProber.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("xbox: XBE probe: %w", err)
	}
	if info == nil {
		return disc, nil, pipelines.ErrNoCandidates
	}
	entry := h.deps.RedumpDB.LookupByXboxTitleID(info.TitleID)
	if entry == nil {
		return disc, nil, pipelines.ErrNoCandidates
	}
	disc.Title = entry.Title
	disc.Year = entry.Year
	disc.MetadataProvider = "Redump"
	// Store the 8-hex-digit title ID so Run can re-fetch the entry for MD5 verify.
	disc.MetadataID = fmt.Sprintf("%08X", info.TitleID)
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

// Plan returns the 6-active-step plan; both transcode and compress are skipped.
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

// Run rips with redumper (xbox mode, produces a single .iso), MD5-verifies
// against Redump, then atomic-moves to the library.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — redumper xbox mode produces a single <name>.iso.
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
	if h.deps.Redumper == nil {
		err := errors.New("xbox: redumper not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	name := "xbox-" + disc.ID
	if err := h.deps.Redumper.Rip(ctx, drv.DevPath, tmpdir, name, "xbox", newStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("redumper: %w", err)
	}
	isoPath := filepath.Join(tmpdir, name+".iso")

	// MD5 verify against Redump entry (warn only; mismatch is non-fatal).
	if h.deps.RedumpDB != nil && disc.MetadataID != "" {
		var titleID uint64
		if n, err := strconv.ParseUint(disc.MetadataID, 16, 32); err == nil {
			titleID = n
		}
		if entry := h.deps.RedumpDB.LookupByXboxTitleID(uint32(titleID)); entry != nil && entry.MD5 != "" {
			got, err := md5File(isoPath)
			if err != nil {
				slog.Warn("xbox: md5 verify failed (couldn't hash)", "err", err)
			} else if got != entry.MD5 {
				slog.Warn("xbox: md5 mismatch", "want", entry.MD5, "got", got)
			} else {
				slog.Info("xbox: md5 verify ok", "md5", got)
			}
		}
	}
	sink.OnStepDone(state.StepRip, map[string]any{"file": isoPath})

	// move — atomic rename directly to library (no chdman step).
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
	if h.deps.Tools != nil && pipelines.ResolveShouldEject(ctx, h.deps.ShouldEject) {
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
	dir := filepath.Join(root, "discecho-xbox-"+discID+"-"+strconv.FormatInt(time.Now().UnixNano(), 36))
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
