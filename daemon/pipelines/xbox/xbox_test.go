package xbox_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/xbox"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// fakeXboxProber stubs xbox.XboxProber.
type fakeXboxProber struct {
	info *identify.XboxInfo
	err  error
}

func (f *fakeXboxProber) Probe(_ context.Context, _ string) (*identify.XboxInfo, error) {
	return f.info, f.err
}

type fakeRedumper struct {
	err error
}

func (f *fakeRedumper) Rip(_ context.Context, _ string, outDir, name, mode string, _ tools.Sink) error {
	if f.err != nil {
		return f.err
	}
	if mode != "xbox" {
		return errors.New("xbox: expected xbox mode, got " + mode)
	}
	return os.WriteFile(filepath.Join(outDir, name+".iso"), []byte("ISO"), 0o644)
}

func newRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(tools.NewMockTool("apprise", nil))
	r.Register(tools.NewMockTool("eject", nil))
	return r
}

// xboxDB builds a minimal RedumpDB with one Xbox entry.
// The ROM name embeds the 8-digit hex title ID in bracket notation.
func xboxDB(t *testing.T, titleID uint32, title, region, md5sum string) *identify.RedumpDB {
	t.Helper()
	gameName := title + " (" + region + ")"
	romName := title + " (" + region + ") [" + toHex8(titleID) + "].iso"
	xml := `<?xml version="1.0"?>` + "\n" +
		`<datafile>` + "\n" +
		`  <game name="` + gameName + `">` + "\n" +
		`    <description>` + gameName + `</description>` + "\n" +
		`    <rom name="` + romName + `" md5="` + md5sum + `"/>` + "\n" +
		`  </game>` + "\n" +
		`</datafile>` + "\n"
	f, err := os.CreateTemp(t.TempDir(), "redump-xbox-*.dat")
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

func toHex8(v uint32) string {
	const hex = "0123456789ABCDEF"
	buf := make([]byte, 8)
	for i := 7; i >= 0; i-- {
		buf[i] = hex[v&0xF]
		v >>= 4
	}
	return string(buf)
}

func TestHandler_DiscType(t *testing.T) {
	h := xbox.New(xbox.Deps{})
	if h.DiscType() != state.DiscTypeXBOX {
		t.Fatalf("disc type: %q", h.DiscType())
	}
}

func TestIdentify_RedumpHit(t *testing.T) {
	db := xboxDB(t, 0x4D530002, "Halo - Combat Evolved", "USA", "abc")
	h := xbox.New(xbox.Deps{
		XboxProber: &fakeXboxProber{info: &identify.XboxInfo{TitleID: 0x4D530002, Region: "USA"}},
		RedumpDB:   db,
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Title != "Halo - Combat Evolved" {
		t.Errorf("Title = %q, want %q", disc.Title, "Halo - Combat Evolved")
	}
	if disc.MetadataProvider != "Redump" {
		t.Errorf("MetadataProvider = %q, want Redump", disc.MetadataProvider)
	}
	if disc.MetadataID != "4D530002" {
		t.Errorf("MetadataID = %q, want 4D530002", disc.MetadataID)
	}
	if disc.Type != state.DiscTypeXBOX {
		t.Errorf("disc.Type = %s, want XBOX", disc.Type)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	if cands[0].Region != "USA" {
		t.Errorf("Region = %q, want USA", cands[0].Region)
	}
}

func TestIdentify_NoRedumpDB(t *testing.T) {
	h := xbox.New(xbox.Deps{
		XboxProber: &fakeXboxProber{info: &identify.XboxInfo{TitleID: 0x4D530002}},
		RedumpDB:   nil,
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestIdentify_RedumpMiss(t *testing.T) {
	db := xboxDB(t, 0x4D530002, "Halo - Combat Evolved", "USA", "abc")
	h := xbox.New(xbox.Deps{
		XboxProber: &fakeXboxProber{info: &identify.XboxInfo{TitleID: 0xDEADBEEF}},
		RedumpDB:   db,
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestPlan_StepShape(t *testing.T) {
	plan := xbox.New(xbox.Deps{}).Plan(&state.Disc{}, &state.Profile{})
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
	if !skipped[state.StepCompress] {
		t.Errorf("compress should be skipped (no chdman for Xbox)")
	}
}

func TestRun_HappyPath(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()

	h := xbox.New(xbox.Deps{
		Redumper:    &fakeRedumper{},
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-xbox",
		Name:               "Xbox-ISO",
		OutputPathTemplate: "{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).iso",
	}
	disc := &state.Disc{
		ID:    "disc-1",
		Type:  state.DiscTypeXBOX,
		Title: "Halo - Combat Evolved",
		Year:  2001,
	}
	disc.Candidates = []state.Candidate{{Source: "Redump", Title: "Halo - Combat Evolved", Region: "USA", Confidence: 100}}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	// Output is an ISO, not CHD or BIN/CUE.
	want := filepath.Join(libRoot, "Halo - Combat Evolved (USA)", "Halo - Combat Evolved (USA).iso")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}

	// Step ordering: detect → identify → rip → move → notify → eject (no compress).
	starts := sink.StepSequence()
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
	for _, st := range starts {
		if st == state.StepTranscode || st == state.StepCompress {
			t.Errorf("step %s should not have started for Xbox", st)
		}
	}
}

func TestRun_RipFailure(t *testing.T) {
	h := xbox.New(xbox.Deps{
		Redumper:    &fakeRedumper{err: errors.New("disc unreadable")},
		Tools:       newRegistry(),
		LibraryRoot: t.TempDir(),
		WorkRoot:    t.TempDir(),
	})
	prof := &state.Profile{ID: "p", Name: "Xbox-ISO", OutputPathTemplate: "{{.Title}}.iso"}
	disc := &state.Disc{ID: "disc-2", Type: state.DiscTypeXBOX, Title: "X"}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(), drv, disc, prof, sink)
	if err == nil || !strings.Contains(err.Error(), "disc unreadable") {
		t.Errorf("want rip error, got %v", err)
	}
}

// Compile-time guard.
var _ = pipelines.ErrNoCandidates
