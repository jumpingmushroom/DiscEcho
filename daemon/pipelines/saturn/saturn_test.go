package saturn_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/saturn"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// fakeSaturnProber stubs identify.SaturnProber.
type fakeSaturnProber struct {
	info *identify.SaturnInfo
	err  error
}

func (f *fakeSaturnProber) Probe(_ context.Context, _ string) (*identify.SaturnInfo, error) {
	return f.info, f.err
}

type fakeRedumper struct {
	err error
}

func (f *fakeRedumper) Rip(_ context.Context, _ string, outDir, name, mode string, _ tools.Sink) error {
	if f.err != nil {
		return f.err
	}
	if mode != "cd" {
		return errors.New("saturn: expected cd mode")
	}
	if err := os.WriteFile(filepath.Join(outDir, name+".bin"), []byte("RAW"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, name+".cue"), []byte("CUE"), 0o644); err != nil {
		return err
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

// saturnDB builds a minimal RedumpDB with one Saturn entry.
// The ROM name embeds the product number in Redump bracket notation
// so parseBootCodeFromROMName extracts it correctly.
func saturnDB(t *testing.T, bootCode, title, region, md5 string) *identify.RedumpDB {
	t.Helper()
	gameName := title + " (" + region + ")"
	romName := title + " (" + region + ") [" + bootCode + "].bin"
	xml := `<?xml version="1.0"?>` + "\n" +
		`<datafile>` + "\n" +
		`  <game name="` + gameName + `">` + "\n" +
		`    <description>` + gameName + `</description>` + "\n" +
		`    <rom name="` + romName + `" md5="` + md5 + `"/>` + "\n" +
		`  </game>` + "\n" +
		`</datafile>` + "\n"
	f, err := os.CreateTemp(t.TempDir(), "redump-sat-*.dat")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(xml); err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	_ = f.Close()
	db, err := identify.LoadRedumpDB(name)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestHandler_DiscType(t *testing.T) {
	h := saturn.New(saturn.Deps{})
	if h.DiscType() != state.DiscTypeSAT {
		t.Fatalf("disc type: %q", h.DiscType())
	}
}

func TestIdentify_RedumpHit(t *testing.T) {
	db := saturnDB(t, "MK-81088", "Nights into Dreams", "Japan", "aabbccdd")
	h := saturn.New(saturn.Deps{
		SaturnProber: &fakeSaturnProber{info: &identify.SaturnInfo{
			ProductNumber: "MK-81088",
			Region:        "JTUBKAEL",
		}},
		RedumpDB: db,
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Title != "Nights into Dreams" {
		t.Errorf("Title = %q, want %q", disc.Title, "Nights into Dreams")
	}
	if disc.MetadataProvider != "Redump" {
		t.Errorf("MetadataProvider = %q, want Redump", disc.MetadataProvider)
	}
	if disc.MetadataID != "MK-81088" {
		t.Errorf("MetadataID = %q, want MK-81088", disc.MetadataID)
	}
	if disc.Type != state.DiscTypeSAT {
		t.Errorf("disc.Type = %s, want SAT", disc.Type)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	if cands[0].Region != "Japan" {
		t.Errorf("Region = %q, want Japan", cands[0].Region)
	}
}

func TestIdentify_NoRedumpDB(t *testing.T) {
	h := saturn.New(saturn.Deps{
		SaturnProber: &fakeSaturnProber{info: &identify.SaturnInfo{ProductNumber: "MK-81088"}},
		RedumpDB:     nil,
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestIdentify_RedumpMiss(t *testing.T) {
	db := saturnDB(t, "MK-81088", "Nights into Dreams", "Japan", "aabbccdd")
	h := saturn.New(saturn.Deps{
		SaturnProber: &fakeSaturnProber{info: &identify.SaturnInfo{ProductNumber: "ZZZZ-99999"}},
		RedumpDB:     db,
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestIdentify_NoProductNumber(t *testing.T) {
	db := saturnDB(t, "MK-81088", "Nights into Dreams", "Japan", "aabbccdd")
	h := saturn.New(saturn.Deps{
		SaturnProber: &fakeSaturnProber{info: &identify.SaturnInfo{ProductNumber: ""}},
		RedumpDB:     db,
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestPlan_StepShape(t *testing.T) {
	plan := saturn.New(saturn.Deps{}).Plan(&state.Disc{}, &state.Profile{})
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

func TestRun_HappyPath(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()

	h := saturn.New(saturn.Deps{
		Redumper:    &fakeRedumper{},
		CHDMan:      &fakeCHDMan{},
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-sat",
		Name:               "Saturn-CHD",
		OutputPathTemplate: "{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd",
	}
	disc := &state.Disc{
		ID:    "disc-1",
		Type:  state.DiscTypeSAT,
		Title: "Nights into Dreams",
		Year:  1996,
	}
	disc.Candidates = []state.Candidate{{Source: "Redump", Title: "Nights into Dreams", Region: "Japan", Confidence: 100}}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(libRoot, "Nights into Dreams (Japan)", "Nights into Dreams (Japan).chd")
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
			t.Errorf("transcode should not have started for Saturn")
		}
	}
}

func TestRun_RipFailure(t *testing.T) {
	h := saturn.New(saturn.Deps{
		Redumper:    &fakeRedumper{err: errors.New("disc unreadable")},
		CHDMan:      &fakeCHDMan{},
		Tools:       newRegistry(),
		LibraryRoot: t.TempDir(),
		WorkRoot:    t.TempDir(),
	})
	prof := &state.Profile{ID: "p", Name: "Saturn-CHD", OutputPathTemplate: "{{.Title}}.chd"}
	disc := &state.Disc{ID: "disc-2", Type: state.DiscTypeSAT, Title: "X"}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(), drv, disc, prof, sink)
	if err == nil || !strings.Contains(err.Error(), "disc unreadable") {
		t.Errorf("want rip error, got %v", err)
	}
}

// Compile-time guard.
var _ = pipelines.ErrNoCandidates
