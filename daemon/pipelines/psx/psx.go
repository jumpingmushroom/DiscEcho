// Package psx implements pipelines.Handler for PlayStation 1 game discs.
//
// Pipeline shape (7 active steps; transcode skipped):
//
//	detect → identify → rip (redumper) → compress (chdman) → move → notify → eject
//
// Identify reads SYSTEM.CNF off the disc, then tries Redump dat (tier 1)
// and BootCodeIndex (tier 2). ErrNoCandidates surfaces when both miss.
package psx

import (
	"context"
	"encoding/json"
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
	BootCodeIndex *identify.BootCodeIndex // Tier-2 fallback; DuckStation provides cover URLs
	Tools         *tools.Registry         // looked up: apprise, eject
	LibraryRoot   string
	WorkRoot      string
	LibraryProbe  func(string) error
	URLsForTrigger func(ctx context.Context, trigger string) []string
	// ShouldEject gates the rip-end eject step; nil = always eject.
	ShouldEject func(ctx context.Context) bool
}

// Handler implements pipelines.Handler for PSX.
type Handler struct{ deps Deps }

func New(d Deps) *Handler {
	if d.LibraryProbe == nil {
		d.LibraryProbe = pipelines.ProbeWritable
	}
	return &Handler{deps: d}
}

func (h *Handler) DiscType() state.DiscType { return state.DiscTypePSX }

// Identify reads SYSTEM.CNF, then tries two tiers of lookup:
// tier 1 — Redump dat (also enables post-rip MD5 verify when it hits);
// tier 2 — BootCodeIndex (DuckStation gamedb, which includes cover URLs).
func (h *Handler) Identify(ctx context.Context, drv *state.Drive) (*state.Disc, []state.Candidate, error) {
	if h.deps.SystemCNF == nil {
		return nil, nil, errors.New("psx: SystemCNF prober not configured")
	}
	disc := &state.Disc{Type: state.DiscTypePSX, DriveID: drv.ID}

	info, err := h.deps.SystemCNF.Probe(ctx, drv.DevPath)
	if err != nil {
		return nil, nil, fmt.Errorf("psx: SYSTEM.CNF probe: %w", err)
	}
	if info == nil || info.BootCode == "" {
		return disc, nil, pipelines.ErrNoCandidates
	}

	// Tier 1: Redump dat.
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

	// Tier 2: BootCodeIndex (DuckStation gamedb).
	if h.deps.BootCodeIndex != nil {
		if entry := h.deps.BootCodeIndex.Lookup(state.DiscTypePSX, info.BootCode); entry != nil {
			disc.Title = entry.Title
			disc.Year = entry.Year
			disc.MetadataProvider = h.deps.BootCodeIndex.Source(state.DiscTypePSX)
			disc.MetadataID = info.BootCode
			// DuckStation provides cover URLs; persist directly into
			// disc.metadata_json so DiscArt picks it up on first paint
			// without a post-pick fetch.
			if entry.CoverURL != "" {
				blob, _ := json.Marshal(map[string]any{
					"system":    "Sony PlayStation",
					"serial":    info.BootCode,
					"cover_url": entry.CoverURL,
				})
				disc.MetadataJSON = string(blob)
			}
			cand := state.Candidate{
				Source: disc.MetadataProvider, Title: entry.Title, Year: entry.Year,
				Region: entry.Region, Confidence: 90,
			}
			disc.Candidates = []state.Candidate{cand}
			return disc, disc.Candidates, nil
		}
	}

	slog.Info("psx: no Redump or BootCodeIndex match", "dev", drv.DevPath, "boot", info.BootCode)
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

// Run rips with redumper, compresses with chdman, atomic-moves to the library.
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
		err := errors.New("psx: redumper or chdman not configured")
		sink.OnStepFailed(state.StepRip, err)
		return err
	}

	name := "psx-" + disc.ID
	if err := h.deps.Redumper.Rip(ctx, drv.DevPath, tmpdir, name, "cd", pipelines.NewStepSink(sink, state.StepRip)); err != nil {
		sink.OnStepFailed(state.StepRip, err)
		return fmt.Errorf("redumper: %w", err)
	}
	binPath := filepath.Join(tmpdir, name+".bin")
	cuePath := filepath.Join(tmpdir, name+".cue")
	sink.OnStepDone(state.StepRip, map[string]any{"file": binPath})

	// compress — MD5-verify .bin, then chdman createcd.
	sink.OnStepStart(state.StepCompress)
	if h.deps.RedumpDB != nil && disc.MetadataID != "" {
		if entry := h.deps.RedumpDB.LookupByBootCode(disc.MetadataID); entry != nil && entry.MD5 != "" {
			got, err := pipelines.MD5File(binPath)
			if err != nil {
				slog.Warn("psx: md5 verify failed (couldn't hash)", "err", err)
			} else if got != entry.MD5 {
				slog.Warn("psx: md5 mismatch", "want", entry.MD5, "got", got)
			} else {
				slog.Info("psx: md5 verify ok", "md5", got)
			}
		}
	}
	chdPath := filepath.Join(tmpdir, name+".chd")
	if err := h.deps.CHDMan.CreateCHD(ctx, cuePath, chdPath, pipelines.NewStepSink(sink, state.StepCompress)); err != nil {
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
	return pipelines.CreateWorkDir(h.deps.WorkRoot, "psx", discID)
}
