// Package xbox implements pipelines.Handler for original Xbox game discs.
//
// Pipeline shape (6 active steps; transcode AND compress skipped):
//
//	detect → identify → rip (redumper xbox) → move → notify → eject
//
// Identify reads default.xbe off the disc via isoinfo, parses the XBE
// certificate for title ID, then tries Redump dat (tier 1) and BootCodeIndex
// (tier 2). ErrNoCandidates surfaces when both miss.
package xbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

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
	Redumper      RedumperRipper
	XboxProber    XboxProber
	RedumpDB      *identify.RedumpDB
	BootCodeIndex *identify.BootCodeIndex // Tier-2 fallback when Redump dat lacks the title ID
	Tools         *tools.Registry         // looked up: apprise, eject
	LibraryRoot   string
	WorkRoot      string
	LibraryProbe  func(string) error
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

// Identify reads default.xbe via isoinfo, then tries two tiers of lookup:
// tier 1 — Redump dat (also enables post-rip MD5 verify when it hits);
// tier 2 — BootCodeIndex (Libretro).
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	disc := &state.Disc{Type: state.DiscTypeXBOX, DriveID: drv.ID}

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

	// Store the 8-hex-digit title ID so Run can re-fetch the entry for MD5 verify.
	code := fmt.Sprintf("%08X", info.TitleID)

	// Tier 1: Redump dat.
	if h.deps.RedumpDB != nil {
		if entry := h.deps.RedumpDB.LookupByXboxTitleID(info.TitleID); entry != nil {
			disc.Title = entry.Title
			disc.Year = entry.Year
			disc.MetadataProvider = "Redump"
			disc.MetadataID = code
			disc.Candidates = []state.Candidate{{
				Source: "Redump", Title: entry.Title, Year: entry.Year,
				Region: entry.Region, Confidence: 100,
			}}
			return disc, disc.Candidates, nil
		}
	}

	// Tier 2: BootCodeIndex (Libretro).
	if h.deps.BootCodeIndex != nil {
		if entry := h.deps.BootCodeIndex.Lookup(state.DiscTypeXBOX, code); entry != nil {
			disc.Title = entry.Title
			disc.Year = entry.Year
			disc.MetadataProvider = h.deps.BootCodeIndex.Source(state.DiscTypeXBOX)
			disc.MetadataID = code
			disc.Candidates = []state.Candidate{{
				Source: disc.MetadataProvider, Title: entry.Title, Year: entry.Year,
				Region: entry.Region, Confidence: 90,
			}}
			return disc, disc.Candidates, nil
		}
	}

	slog.Info("xbox: no Redump or BootCodeIndex match", "dev", drv.DevPath, "title_id", code)
	return disc, nil, pipelines.ErrNoCandidates
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
	if err := h.deps.Redumper.Rip(ctx, drv.DevPath, tmpdir, name, "xbox", pipelines.NewStepSink(sink, state.StepRip)); err != nil {
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
			got, err := pipelines.MD5File(isoPath)
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
	return pipelines.CreateWorkDir(h.deps.WorkRoot, "xbox", discID)
}
