package bdmv_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/bdmv"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// fakeProber returns a fixed DVDInfo (BDMV reuses the existing DVD
// prober for volume-label reading).
type fakeProber struct {
	info *identify.DVDInfo
	err  error
}

func (f *fakeProber) Probe(_ context.Context, _ string) (*identify.DVDInfo, error) {
	return f.info, f.err
}

// fakeTMDB returns canned candidates.
type fakeTMDB struct {
	cands []state.Candidate
	err   error
}

func (f *fakeTMDB) SearchMovie(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, f.err
}
func (f *fakeTMDB) SearchTV(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, f.err
}
func (f *fakeTMDB) SearchBoth(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, f.err
}
func (f *fakeTMDB) MovieRuntime(_ context.Context, _ int) (int, error) { return 0, nil }

// fakeMakeMKV satisfies both bdmv.MakeMKVScanner and bdmv.MakeMKVRipper.
type fakeMakeMKV struct {
	scanTitles []tools.MakeMKVTitle
	scanErr    error
	ripErr     error
	stubName   string
}

func (f *fakeMakeMKV) Scan(_ context.Context, _ string) ([]tools.MakeMKVTitle, error) {
	return f.scanTitles, f.scanErr
}

func (f *fakeMakeMKV) Rip(_ context.Context, _ string, _ int, outDir string, _ tools.Sink) error {
	if f.ripErr != nil {
		return f.ripErr
	}
	name := f.stubName
	if name == "" {
		name = "title_t00.mkv"
	}
	return os.WriteFile(filepath.Join(outDir, name), []byte("STUB"), 0o644)
}

// fakeHandBrake satisfies tools.Tool. On Run: writes a stub file at the
// --output arg.
type fakeHandBrake struct {
	encodeErr error
}

func (f *fakeHandBrake) Name() string { return "handbrake" }
func (f *fakeHandBrake) Run(_ context.Context, args []string, _ map[string]string,
	_ string, _ tools.Sink) error {
	if f.encodeErr != nil {
		return f.encodeErr
	}
	for i, a := range args {
		if a == "--output" && i+1 < len(args) {
			_ = os.WriteFile(args[i+1], []byte("ENCODED"), 0o644)
		}
	}
	return nil
}

func newRegistry() (*tools.Registry, *tools.MockTool, *tools.MockTool, *fakeHandBrake) {
	hb := &fakeHandBrake{}
	apprise := tools.NewMockTool("apprise", nil)
	eject := tools.NewMockTool("eject", nil)
	r := tools.NewRegistry()
	r.Register(hb)
	r.Register(apprise)
	r.Register(eject)
	return r, apprise, eject, hb
}

func TestBDMVHandler_DiscType(t *testing.T) {
	h := bdmv.New(bdmv.Deps{})
	if got := h.DiscType(); got != state.DiscTypeBDMV {
		t.Errorf("disc type = %s, want BDMV", got)
	}
}

func TestBDMVHandler_Identify_HappyPath(t *testing.T) {
	h := bdmv.New(bdmv.Deps{
		Prober: &fakeProber{info: &identify.DVDInfo{VolumeLabel: "ARRIVAL"}},
		TMDB: &fakeTMDB{cands: []state.Candidate{
			{Source: "TMDB", Title: "Arrival", Year: 2016, Confidence: 95, TMDBID: 329865, MediaType: "movie"},
		}},
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Title != "Arrival" || disc.Year != 2016 {
		t.Errorf("unexpected disc: %+v", disc)
	}
	if len(cands) != 1 {
		t.Errorf("want 1 candidate, got %d", len(cands))
	}
	if disc.Type != state.DiscTypeBDMV {
		t.Errorf("disc.Type = %s, want BDMV", disc.Type)
	}
}

func TestBDMVHandler_Identify_NoCandidates(t *testing.T) {
	h := bdmv.New(bdmv.Deps{
		// "BLURAY" normalises to "" via NormaliseDVDLabel — the existing
		// junk filter rejects too-short / generic labels.
		Prober: &fakeProber{info: &identify.DVDInfo{VolumeLabel: "BLURAY"}},
		TMDB:   &fakeTMDB{cands: nil},
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestBDMVHandler_Plan_CompressSkipped(t *testing.T) {
	h := bdmv.New(bdmv.Deps{})
	plan := h.Plan(&state.Disc{}, &state.Profile{})
	if len(plan) != 8 {
		t.Fatalf("want 8 steps, got %d", len(plan))
	}
	for _, sp := range plan {
		if sp.ID == state.StepCompress && !sp.Skip {
			t.Errorf("compress should be skipped")
		}
		if sp.ID == state.StepTranscode && sp.Skip {
			t.Errorf("transcode should NOT be skipped (BDMV transcodes via HandBrake)")
		}
	}
}

func TestBDMVHandler_Run_HappyPath(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()

	reg, _, _, _ := newRegistry()
	h := bdmv.New(bdmv.Deps{
		MakeMKVScanner: &fakeMakeMKV{scanTitles: []tools.MakeMKVTitle{
			{ID: 0, DurationSec: 30, SourceFile: "00000.mpls"},
			{ID: 1, DurationSec: 7000, SourceFile: "00800.mpls"},
		}},
		MakeMKVRipper: &fakeMakeMKV{stubName: "title_t01.mkv"},
		Tools:         reg,
		LibraryRoot:   libRoot,
		WorkRoot:      workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-bd",
		DiscType:           state.DiscTypeBDMV,
		Name:               "BD-1080p",
		Preset:             "x265 RF 19 10-bit",
		OutputPathTemplate: "{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mkv",
		Options:            map[string]any{"min_title_seconds": float64(3600)},
	}
	disc := &state.Disc{ID: "disc-1", Type: state.DiscTypeBDMV, Title: "Arrival", Year: 2016}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(libRoot, "Arrival (2016)", "Arrival (2016).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}

	starts := sink.StepSequence()
	wantOrder := []state.StepID{
		state.StepDetect, state.StepIdentify, state.StepRip,
		state.StepTranscode, state.StepMove, state.StepNotify, state.StepEject,
	}
	if len(starts) != len(wantOrder) {
		t.Fatalf("started %d steps, want %d: %v", len(starts), len(wantOrder), starts)
	}
	for i := range wantOrder {
		if starts[i] != wantOrder[i] {
			t.Errorf("step %d = %s, want %s", i, starts[i], wantOrder[i])
		}
	}
}

func TestBDMVHandler_Run_NoTitleAboveMin(t *testing.T) {
	reg, _, _, _ := newRegistry()
	h := bdmv.New(bdmv.Deps{
		MakeMKVScanner: &fakeMakeMKV{scanTitles: []tools.MakeMKVTitle{
			{ID: 0, DurationSec: 30}, // all titles below 1 hour
		}},
		MakeMKVRipper: &fakeMakeMKV{},
		Tools:         reg,
		LibraryRoot:   t.TempDir(),
		WorkRoot:      t.TempDir(),
	})
	prof := &state.Profile{
		ID:      "p-bd",
		Name:    "BD-1080p",
		Preset:  "x265 RF 19 10-bit",
		Options: map[string]any{"min_title_seconds": float64(3600)},
	}
	disc := &state.Disc{ID: "disc-2", Type: state.DiscTypeBDMV, Title: "X", Year: 2020}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}
	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(), drv, disc, prof, sink)
	if err == nil || !strings.Contains(err.Error(), "no title") {
		t.Errorf("want 'no title' error, got %v", err)
	}
}
