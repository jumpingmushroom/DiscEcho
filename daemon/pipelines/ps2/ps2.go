// Package ps2 implements pipelines.Handler for PlayStation 2 game discs.
//
// Pipeline shape (7 active steps; transcode skipped):
//
//	detect → identify → rip (redumper, dvd mode) → compress (chdman)
//	    → move → notify → eject
//
// Identify reads SYSTEM.CNF off the disc, then tries Redump dat (tier 1)
// and BootCodeIndex (tier 2). ErrNoCandidates surfaces when both miss.
package ps2

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

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
	Redumper      RedumperRipper
	CHDMan        CHDManCompressor
	SystemCNF     identify.SystemCNFProber
	RedumpDB      *identify.RedumpDB
	BootCodeIndex *identify.BootCodeIndex // Tier-2 fallback when Redump dat lacks bracketed boot codes
	Tools         *tools.Registry         // looked up: apprise, eject
	LibraryRoot   string
	WorkRoot      string
	LibraryProbe  func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	// ShouldEject gates the rip-end eject step; nil = always eject.
	ShouldEject func(ctx context.Context) bool
}

// Handler implements pipelines.Handler for PS2.
type Handler struct{ deps Deps }

func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

func (h *Handler) DiscType() state.DiscType { return state.DiscTypePS2 }

// Identify reads SYSTEM.CNF, then tries two tiers of lookup:
// tier 1 — Redump dat (also enables post-rip MD5 verify when it hits);
// tier 2 — BootCodeIndex (PCSX2 GameDB).
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	if h.deps.SystemCNF == nil {
		return nil, nil, errors.New("ps2: SystemCNF prober not configured")
	}
	disc := &state.Disc{Type: state.DiscTypePS2, DriveID: drv.ID}

	info, err := h.deps.SystemCNF.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("ps2: SYSTEM.CNF probe: %w", err)
	}
	if info == nil || info.BootCode == "" {
		return disc, nil, pipelines.ErrNoCandidates
	}

	// Tier 1: Redump dat (rare hit with modern public dats, but when it
	// hits, post-rip MD5 verify at the compress step also works).
	if h.deps.RedumpDB != nil {
		if entry := h.deps.RedumpDB.LookupByBootCode(info.BootCode); entry != nil {
			disc.Title = entry.Title
			disc.Year = entry.Year
			disc.MetadataProvider = "Redump"
			disc.MetadataID = entry.BootCode
			cand := state.Candidate{
				Source: "Redump", Title: entry.Title, Year: entry.Year,
				Region: entry.Region, Confidence: 100,
			}
			disc.Candidates = []state.Candidate{cand}
			return disc, disc.Candidates, nil
		}
	}

	// Tier 2: BootCodeIndex (PCSX2 GameDB).
	if h.deps.BootCodeIndex != nil {
		if entry := h.deps.BootCodeIndex.Lookup(state.DiscTypePS2, info.BootCode); entry != nil {
			region := entry.Region
			if region == "" {
				region = identify.InferRegion(info.BootCode)
			}
			disc.Title = entry.Title
			disc.Year = entry.Year
			disc.MetadataProvider = h.deps.BootCodeIndex.Source(state.DiscTypePS2)
			disc.MetadataID = info.BootCode
			cand := state.Candidate{
				Source: disc.MetadataProvider, Title: entry.Title, Year: entry.Year,
				Region: region, Confidence: 90,
			}
			disc.Candidates = []state.Candidate{cand}
			return disc, disc.Candidates, nil
		}
	}

	slog.Info("ps2: no Redump or BootCodeIndex match", "dev", drv.DevPath, "boot", info.BootCode)
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

// Run rips with redumper (dvd mode), compresses with chdman, atomic-moves to the library.
func (h *Handler) Run(ctx context.Context, drv *state.Drive, disc *state.Disc, prof *state.Profile, sink pipelines.EventSink) error {
	sink.OnStepStart(state.StepDetect)
	sink.OnStepDone(state.StepDetect, nil)
	sink.OnStepStart(state.StepIdentify)
	sink.OnStepDone(state.StepIdentify, nil)

	// rip — redumper produces <name>.iso (DVD media, single file).
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
		err := errors.New("ps2: redumper or chdman not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	name := "ps2-" + disc.ID
	if err := h.deps.Redumper.Rip(ctx, drv.DevPath, tmpdir, name, "dvd", pipelines.NewStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("redumper: %w", err)
	}
	isoPath := filepath.Join(tmpdir, name+".iso")
	sink.OnStepDone(state.StepRip, map[string]any{"file": isoPath})

	// compress — MD5-verify .iso, then chdman (auto-picks createdvd from .iso).
	sink.OnStepStart(state.StepCompress)
	if h.deps.RedumpDB != nil && disc.MetadataID != "" {
		if entry := h.deps.RedumpDB.LookupByBootCode(disc.MetadataID); entry != nil && entry.MD5 != "" {
			got, err := pipelines.MD5File(isoPath)
			if err != nil {
				slog.Warn("ps2: md5 verify failed (couldn't hash)", "err", err)
			} else if got != entry.MD5 {
				slog.Warn("ps2: md5 mismatch", "want", entry.MD5, "got", got)
			} else {
				slog.Info("ps2: md5 verify ok", "md5", got)
			}
		}
	}
	chdPath := filepath.Join(tmpdir, name+".chd")
	if err := h.deps.CHDMan.CreateCHD(ctx, isoPath, chdPath, pipelines.NewStepSink(sink, state.StepCompress)); err != nil {
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
	return pipelines.CreateWorkDir(h.deps.WorkRoot, "ps2", discID)
}
