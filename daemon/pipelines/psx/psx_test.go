package psx_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/psx"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type fakeSystemCNF struct {
	info *identify.SystemCNF
	err  error
}

func (f *fakeSystemCNF) Probe(_ context.Context, _ string) (*identify.SystemCNF, error) {
	return f.info, f.err
}

type fakeRedumper struct {
	stubExt string // ".bin" for PSX
	err     error
}

func (f *fakeRedumper) Rip(_ context.Context, _ string, outDir, name, mode string, _ tools.Sink) error {
	if f.err != nil {
		return f.err
	}
	if mode != "cd" {
		return errors.New("psx: expected cd mode")
	}
	ext := f.stubExt
	if ext == "" {
		ext = ".bin"
	}
	if err := os.WriteFile(filepath.Join(outDir, name+ext), []byte("RAW"), 0o644); err != nil {
		return err
	}
	if ext == ".bin" {
		// Always emit a matching cue so the chdman fake gets a
		// realistic file to consume.
		if err := os.WriteFile(filepath.Join(outDir, name+".cue"), []byte("CUE"), 0o644); err != nil {
			return err
		}
	}
	return nil
}

type fakeCHDMan struct{ err error }

func (f *fakeCHDMan) CreateCHD(_ context.Context, _ string, output string, _ tools.Sink) error {
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(output, []byte("CHD"), 0o644)
}

func newRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(tools.NewMockTool("apprise", nil))
	r.Register(tools.NewMockTool("eject", nil))
	return r
}

func sampleDB(t *testing.T) *identify.RedumpDB {
	t.Helper()
	db, err := identify.LoadRedumpDB(filepath.Join("..", "..", "identify", "testdata", "redump-psx-sample.dat"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPSXHandler_DiscType(t *testing.T) {
	h := psx.New(psx.Deps{})
	if h.DiscType() != state.DiscTypePSX {
		t.Errorf("disc type = %s, want PSX", h.DiscType())
	}
}

func TestPSXHandler_Identify_HappyPath(t *testing.T) {
	db := sampleDB(t)
	h := psx.New(psx.Deps{
		SystemCNF: &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCUS_004.34", IsPS2: false}},
		RedumpDB:  db,
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Title != "Final Fantasy VII" {
		t.Errorf("Title = %q", disc.Title)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	if cands[0].Region != "USA" {
		t.Errorf("Region = %q, want USA", cands[0].Region)
	}
	if disc.Type != state.DiscTypePSX {
		t.Errorf("disc.Type = %s, want PSX", disc.Type)
	}
}

func TestPSXHandler_Identify_NoBootCode(t *testing.T) {
	h := psx.New(psx.Deps{
		SystemCNF: &fakeSystemCNF{info: nil}, // probe returned nil
		RedumpDB:  sampleDB(t),
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestPSXHandler_Identify_DBMissing(t *testing.T) {
	h := psx.New(psx.Deps{
		SystemCNF: &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCUS_004.34"}},
		RedumpDB:  nil, // dat file missing
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestPSXHandler_Identify_BootCodeNotInDB(t *testing.T) {
	h := psx.New(psx.Deps{
		SystemCNF: &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "ZZZZ_999.99"}},
		RedumpDB:  sampleDB(t),
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestPSXHandler_Plan_TranscodeSkipped(t *testing.T) {
	plan := psx.New(psx.Deps{}).Plan(&state.Disc{}, &state.Profile{})
	if len(plan) != 8 {
		t.Fatalf("want 8 entries, got %d", len(plan))
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
	if skipped[state.StepCompress] {
		t.Errorf("compress should NOT be skipped (chdman runs there)")
	}
}

func TestPSXHandler_Run_HappyPath(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()

	h := psx.New(psx.Deps{
		Redumper:    &fakeRedumper{},
		CHDMan:      &fakeCHDMan{},
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-psx",
		Name:               "PSX-CHD",
		OutputPathTemplate: "{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd",
	}
	disc := &state.Disc{
		ID:    "disc-1",
		Type:  state.DiscTypePSX,
		Title: "Final Fantasy VII",
		Year:  1997,
	}
	disc.Candidates = []state.Candidate{{Source: "Redump", Title: "Final Fantasy VII", Region: "USA", Confidence: 100}}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(libRoot, "Final Fantasy VII (USA)", "Final Fantasy VII (USA).chd")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}

	// Step ordering: detect → identify → rip → compress → move → notify → eject
	starts := sink.StepSequence()
	wantOrder := []state.StepID{
		state.StepDetect, state.StepIdentify, state.StepRip,
		state.StepCompress, state.StepMove, state.StepNotify, state.StepEject,
	}
	if len(starts) != len(wantOrder) {
		t.Fatalf("started %d steps, want %d: %v", len(starts), len(wantOrder), starts)
	}
	for i := range wantOrder {
		if starts[i] != wantOrder[i] {
			t.Errorf("step %d = %s, want %s", i, starts[i], wantOrder[i])
		}
	}
	for _, st := range starts {
		if st == state.StepTranscode {
			t.Errorf("transcode should not have started for PSX")
		}
	}
}

func TestPSXHandler_Run_RipFailure(t *testing.T) {
	h := psx.New(psx.Deps{
		Redumper:    &fakeRedumper{err: errors.New("disc unreadable")},
		CHDMan:      &fakeCHDMan{},
		Tools:       newRegistry(),
		LibraryRoot: t.TempDir(),
		WorkRoot:    t.TempDir(),
	})
	prof := &state.Profile{ID: "p", Name: "PSX-CHD", OutputPathTemplate: "{{.Title}}.chd"}
	disc := &state.Disc{ID: "disc-2", Type: state.DiscTypePSX, Title: "X"}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(), drv, disc, prof, sink)
	if err == nil || !strings.Contains(err.Error(), "disc unreadable") {
		t.Errorf("want rip error, got %v", err)
	}
}

func TestPSXIdentify_BootCodeIndexFallback_WithCover(t *testing.T) {
	idx := identify.NewBootCodeIndex()
	if err := idx.MergeFile(state.DiscTypePSX, []byte(`{
		"system":"PSX","source":"DuckStation","entries":{
			"SCUS_944.61":{"title":"Final Fantasy VII (Disc 1)","region":"USA","year":1997,"cover_url":"https://thumbnails.libretro.com/Sony%20-%20PlayStation/Named_Boxarts/Final%20Fantasy%20VII%20(USA)%20(Disc%201).png"}
		}
	}`)); err != nil {
		t.Fatal(err)
	}

	h := psx.New(psx.Deps{
		SystemCNF:     &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCUS_944.61", IsPS2: false}},
		RedumpDB:      identify.NewEmptyRedumpDB(),
		BootCodeIndex: idx,
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "drv1"})
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if disc.Title != "Final Fantasy VII (Disc 1)" {
		t.Errorf("Title = %q", disc.Title)
	}
	if disc.MetadataProvider != "DuckStation" {
		t.Errorf("MetadataProvider = %q", disc.MetadataProvider)
	}
	if len(cands) != 1 {
		t.Fatalf("cands len = %d, want 1", len(cands))
	}
	// PSX is the one system that carries cover URLs from its boot-code
	// source. The handler stashes them into disc.MetadataJSON at identify
	// time so DiscArt can render real cover art on first paint.
	if disc.MetadataJSON == "" {
		t.Errorf("MetadataJSON empty; PSX cover_url should be persisted at identify time")
	}
}

// Compile-time guard.
var _ = pipelines.ErrNoCandidates
