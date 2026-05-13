package uhd_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/uhd"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type fakeProber struct {
	info *identify.DVDInfo
}

func (f *fakeProber) Probe(_ context.Context, _ string) (*identify.DVDInfo, error) {
	return f.info, nil
}

// fakeTMDB satisfies identify.TMDBClient (SearchMovie + SearchTV +
// SearchBoth). Mirrors dvdvideo_test.go / bdmv_test.go.
type fakeTMDB struct {
	cands []state.Candidate
}

func (f *fakeTMDB) SearchMovie(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, nil
}
func (f *fakeTMDB) SearchTV(_ context.Context, _ string) ([]state.Candidate, error) {
	return nil, nil
}
func (f *fakeTMDB) SearchBoth(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, nil
}
func (f *fakeTMDB) MovieRuntime(_ context.Context, _ int) (int, error) { return 0, nil }
func (f *fakeTMDB) MovieDetails(_ context.Context, _ int) (identify.DiscMetadata, error) {
	return identify.DiscMetadata{}, nil
}
func (f *fakeTMDB) TVDetails(_ context.Context, _ int) (identify.DiscMetadata, error) {
	return identify.DiscMetadata{}, nil
}

type fakeMakeMKV struct {
	titles   []tools.MakeMKVTitle
	stubName string
}

func (f *fakeMakeMKV) Scan(_ context.Context, _ string) ([]tools.MakeMKVTitle, error) {
	return f.titles, nil
}
func (f *fakeMakeMKV) Rip(_ context.Context, _ string, _ int, outDir string, _ tools.Sink) error {
	name := f.stubName
	if name == "" {
		name = "title_t00.mkv"
	}
	return os.WriteFile(filepath.Join(outDir, name), []byte("UHD-STUB"), 0o644)
}

func newRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(tools.NewMockTool("apprise", nil))
	r.Register(tools.NewMockTool("eject", nil))
	return r
}

func TestUHDHandler_DiscType(t *testing.T) {
	if got := uhd.New(uhd.Deps{}).DiscType(); got != state.DiscTypeUHD {
		t.Errorf("disc type = %s, want UHD", got)
	}
}

func TestUHDHandler_Identify_MissingKeyDB(t *testing.T) {
	dir := t.TempDir()
	missingKey := filepath.Join(dir, "no-such-keydb.cfg")
	h := uhd.New(uhd.Deps{
		AACS2KeyDB: missingKey,
		Prober:     &fakeProber{info: &identify.DVDInfo{VolumeLabel: "DUNE"}},
		TMDB:       &fakeTMDB{cands: nil},
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, uhd.ErrAACS2KeyMissing) {
		t.Errorf("want ErrAACS2KeyMissing, got %v", err)
	}
}

func TestUHDHandler_Identify_NotConfigured(t *testing.T) {
	// AACS2KeyDB empty string is treated the same as missing.
	h := uhd.New(uhd.Deps{
		Prober: &fakeProber{info: &identify.DVDInfo{VolumeLabel: "DUNE"}},
		TMDB:   &fakeTMDB{cands: nil},
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, uhd.ErrAACS2KeyMissing) {
		t.Errorf("want ErrAACS2KeyMissing for empty AACS2KeyDB, got %v", err)
	}
}

func TestUHDHandler_Identify_KeyPresent(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "KEYDB.cfg")
	if err := os.WriteFile(key, []byte("# fake key"), 0o600); err != nil {
		t.Fatal(err)
	}
	h := uhd.New(uhd.Deps{
		AACS2KeyDB: key,
		Prober:     &fakeProber{info: &identify.DVDInfo{VolumeLabel: "DUNE_PART_TWO"}},
		TMDB: &fakeTMDB{cands: []state.Candidate{
			{Source: "TMDB", Title: "Dune: Part Two", Year: 2024, Confidence: 95, TMDBID: 693134, MediaType: "movie"},
		}},
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Title != "Dune: Part Two" {
		t.Errorf("disc title = %q", disc.Title)
	}
	if len(cands) != 1 {
		t.Errorf("want 1 candidate")
	}
	if disc.Type != state.DiscTypeUHD {
		t.Errorf("disc.Type = %s, want UHD", disc.Type)
	}
}

func TestUHDHandler_Plan_TranscodeAndCompressSkipped(t *testing.T) {
	plan := uhd.New(uhd.Deps{}).Plan(&state.Disc{}, &state.Profile{})
	if len(plan) != 8 {
		t.Fatalf("want 8 step plans, got %d", len(plan))
	}
	skipped := map[state.StepID]bool{}
	for _, sp := range plan {
		if sp.Skip {
			skipped[sp.ID] = true
		}
	}
	if !skipped[state.StepTranscode] {
		t.Errorf("transcode should be skipped")
	}
	if !skipped[state.StepCompress] {
		t.Errorf("compress should be skipped")
	}
	if skipped[state.StepRip] {
		t.Errorf("rip should NOT be skipped")
	}
}

func TestUHDHandler_Run_HappyPath(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "KEYDB.cfg")
	_ = os.WriteFile(key, []byte("# key"), 0o600)

	libRoot := t.TempDir()
	workRoot := t.TempDir()

	h := uhd.New(uhd.Deps{
		AACS2KeyDB: key,
		MakeMKVScanner: &fakeMakeMKV{titles: []tools.MakeMKVTitle{
			{ID: 0, DurationSec: 9968, SourceFile: "00800.mpls"},
		}},
		MakeMKVRipper: &fakeMakeMKV{stubName: "title_t00.mkv"},
		Tools:         newRegistry(),
		LibraryRoot:   libRoot,
		WorkRoot:      workRoot,
	})

	prof := &state.Profile{
		ID:                 "p-uhd",
		Name:               "UHD-Remux",
		Preset:             "passthrough",
		OutputPathTemplate: "{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}) [UHD].mkv",
		Options:            map[string]any{"min_title_seconds": float64(3600)},
	}
	disc := &state.Disc{ID: "disc-u1", Type: state.DiscTypeUHD, Title: "Dune Part Two", Year: 2024}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(libRoot, "Dune Part Two (2024)", "Dune Part Two (2024) [UHD].mkv")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}

	// Step ordering: detect → identify → rip → move → notify → eject.
	// Transcode + Compress must NOT have been started.
	starts := sink.StepSequence()
	for _, st := range starts {
		if st == state.StepTranscode || st == state.StepCompress {
			t.Errorf("step %s should not have started for UHD", st)
		}
	}
	wantOrder := []state.StepID{
		state.StepDetect, state.StepIdentify, state.StepRip,
		state.StepMove, state.StepNotify, state.StepEject,
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

// Compile-time check that ErrNoCandidates is reachable for grep-greppers.
var _ = pipelines.ErrNoCandidates
