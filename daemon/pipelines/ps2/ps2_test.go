package ps2_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/ps2"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type fakeSystemCNF struct {
	info *identify.SystemCNF
}

func (f *fakeSystemCNF) Probe(_ context.Context, _ string) (*identify.SystemCNF, error) {
	return f.info, nil
}

type fakeRedumper struct{}

func (f *fakeRedumper) Rip(_ context.Context, _ string, outDir, name, mode string, _ tools.Sink) error {
	if mode != "dvd" {
		return errors.New("ps2: expected dvd mode")
	}
	return os.WriteFile(filepath.Join(outDir, name+".iso"), []byte("ISO"), 0o644)
}

type fakeCHDMan struct{}

func (f *fakeCHDMan) CreateCHD(_ context.Context, input string, output string, _ tools.Sink) error {
	if filepath.Ext(input) != ".iso" {
		return errors.New("ps2: chdman expected .iso input")
	}
	return os.WriteFile(output, []byte("CHD"), 0o644)
}

func newRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(tools.NewMockTool("apprise", nil))
	r.Register(tools.NewMockTool("eject", nil))
	return r
}

// Reuses the PSX sample dat — boot codes don't overlap and the lookup
// contract is identical.
func samplePSXDB(t *testing.T) *identify.RedumpDB {
	t.Helper()
	db, err := identify.LoadRedumpDB(filepath.Join("..", "..", "identify", "testdata", "redump-psx-sample.dat"))
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPS2Handler_DiscType(t *testing.T) {
	if h := ps2.New(ps2.Deps{}); h.DiscType() != state.DiscTypePS2 {
		t.Errorf("disc type = %s, want PS2", h.DiscType())
	}
}

func TestPS2Handler_Identify_HappyPath(t *testing.T) {
	h := ps2.New(ps2.Deps{
		SystemCNF: &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCUS_004.34", IsPS2: true}},
		RedumpDB:  samplePSXDB(t),
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Type != state.DiscTypePS2 {
		t.Errorf("disc.Type = %s, want PS2", disc.Type)
	}
	if disc.Title != "Final Fantasy VII" {
		t.Errorf("Title = %q", disc.Title)
	}
	if len(cands) != 1 {
		t.Errorf("want 1 candidate, got %d", len(cands))
	}
}

func TestPS2Handler_Plan_TranscodeSkipped(t *testing.T) {
	plan := ps2.New(ps2.Deps{}).Plan(&state.Disc{}, &state.Profile{})
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
		t.Errorf("compress should NOT be skipped")
	}
}

func TestPS2Handler_Run_HappyPath(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()

	h := ps2.New(ps2.Deps{
		Redumper:    &fakeRedumper{},
		CHDMan:      &fakeCHDMan{},
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-ps2",
		Name:               "PS2-CHD",
		OutputPathTemplate: "{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd",
	}
	disc := &state.Disc{
		ID:    "disc-1",
		Type:  state.DiscTypePS2,
		Title: "Shadow of the Colossus",
		Year:  2005,
		Candidates: []state.Candidate{
			{Source: "Redump", Title: "Shadow of the Colossus", Region: "USA", Confidence: 100},
		},
	}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(libRoot, "Shadow of the Colossus (USA)", "Shadow of the Colossus (USA).chd")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}
	for _, st := range sink.StepSequence() {
		if st == state.StepTranscode {
			t.Errorf("transcode should not have started for PS2")
		}
	}
}

// mustRedumpDat builds a minimal in-memory RedumpDB from a temp dat file
// containing a single game whose ROM name has the boot code in bracket notation.
func mustRedumpDat(t *testing.T, subdir, romName, md5 string) *identify.RedumpDB {
	t.Helper()
	dir := t.TempDir()
	sub := filepath.Join(dir, subdir)
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	xml := fmt.Sprintf(`<?xml version="1.0"?>
<datafile><game name="Sly 3 - Honour Among Thieves (Europe)">
  <rom name="%s" size="1" md5="%s"/>
</game></datafile>`, romName, md5)
	if err := os.WriteFile(filepath.Join(sub, "test.dat"), []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := identify.LoadRedumpDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestPS2Identify_BootCodeIndexFallback(t *testing.T) {
	// Redump dat is empty (matches reality with modern public dats),
	// but the BootCodeIndex has the entry. Handler returns a
	// PCSX2-GameDB-sourced candidate at confidence 90.
	idx := identify.NewBootCodeIndex()
	if err := idx.MergeFile(state.DiscTypePS2, []byte(`{
		"system":"PS2","source":"PCSX2-GameDB","entries":{
			"SCES_534.09":{"title":"Sly 3: Honor Among Thieves","region":"Europe"}
		}
	}`)); err != nil {
		t.Fatal(err)
	}

	h := ps2.New(ps2.Deps{
		SystemCNF:     &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCES_534.09", IsPS2: true}},
		RedumpDB:      identify.NewEmptyRedumpDB(),
		BootCodeIndex: idx,
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "drv1", DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if disc.Title != "Sly 3: Honor Among Thieves" {
		t.Errorf("Title = %q", disc.Title)
	}
	if disc.MetadataProvider != "PCSX2-GameDB" {
		t.Errorf("MetadataProvider = %q, want PCSX2-GameDB", disc.MetadataProvider)
	}
	if disc.MetadataID != "SCES_534.09" {
		t.Errorf("MetadataID = %q", disc.MetadataID)
	}
	if len(cands) != 1 || cands[0].Confidence != 90 {
		t.Errorf("cands = %+v, want one entry at confidence 90", cands)
	}
}

func TestPS2Identify_RedumpWinsOverBootCodeIndex(t *testing.T) {
	dat := mustRedumpDat(t, "ps2", "Sly 3 [SCES_534.09].iso", "abc123")
	idx := identify.NewBootCodeIndex()
	if err := idx.MergeFile(state.DiscTypePS2, []byte(`{
		"system":"PS2","source":"PCSX2-GameDB","entries":{
			"SCES_534.09":{"title":"Should Not Win"}
		}
	}`)); err != nil {
		t.Fatal(err)
	}

	h := ps2.New(ps2.Deps{
		SystemCNF:     &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCES_534.09", IsPS2: true}},
		RedumpDB:      dat,
		BootCodeIndex: idx,
	})
	disc, _, err := h.Identify(context.Background(), &state.Drive{ID: "drv1"})
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if disc.MetadataProvider != "Redump" {
		t.Errorf("MetadataProvider = %q, want Redump (tier 1 wins)", disc.MetadataProvider)
	}
}

func TestPS2Identify_NoMatchAnywhere(t *testing.T) {
	h := ps2.New(ps2.Deps{
		SystemCNF:     &fakeSystemCNF{info: &identify.SystemCNF{BootCode: "SCES_999.99", IsPS2: true}},
		RedumpDB:      identify.NewEmptyRedumpDB(),
		BootCodeIndex: identify.NewBootCodeIndex(),
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "drv1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("err = %v, want ErrNoCandidates", err)
	}
	if disc == nil || disc.Type != state.DiscTypePS2 {
		t.Errorf("disc = %+v, want non-nil PS2", disc)
	}
	if cands != nil {
		t.Errorf("cands = %+v, want nil", cands)
	}
}

// Compile-time guard.
var _ = pipelines.ErrNoCandidates
