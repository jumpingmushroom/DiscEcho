package dreamcast_test

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/dreamcast"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// fakeRedumper stubs RedumperRipper. On success it writes a .gdi manifest
// and one .bin track so the handler can compute an MD5.
type fakeRedumper struct {
	err     error
	binData []byte // content written to the first .bin track
}

func (f *fakeRedumper) Rip(_ context.Context, _ string, outDir, name, mode string, _ tools.Sink) error {
	if f.err != nil {
		return f.err
	}
	if mode != "cd" {
		return errors.New("dreamcast: expected cd mode")
	}
	// Write a minimal GDI manifest and one .bin track.
	gdi := "1\n1 0 4 2048 " + name + "01.bin 0\n"
	if err := os.WriteFile(filepath.Join(outDir, name+".gdi"), []byte(gdi), 0o644); err != nil {
		return err
	}
	data := f.binData
	if data == nil {
		data = []byte("TRACK01")
	}
	return os.WriteFile(filepath.Join(outDir, name+"01.bin"), data, 0o644)
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

// dcDB builds a minimal RedumpDB with one Dreamcast entry keyed by the MD5
// of the provided binData. The ROM name uses the BootCode bracket notation
// so parseBootCodeFromROMName can extract it.
func dcDB(t *testing.T, bootCode, title, region string, binData []byte) (*identify.RedumpDB, string) {
	t.Helper()
	sum := md5.Sum(binData)
	md5hex := hex.EncodeToString(sum[:])

	gameName := title + " (" + region + ")"
	romName := title + " (" + region + ") [" + bootCode + "].bin"
	xml := `<?xml version="1.0"?>` + "\n" +
		`<datafile>` + "\n" +
		`  <game name="` + gameName + `">` + "\n" +
		`    <description>` + gameName + `</description>` + "\n" +
		`    <rom name="` + romName + `" md5="` + md5hex + `"/>` + "\n" +
		`  </game>` + "\n" +
		`</datafile>` + "\n"
	f, err := os.CreateTemp(t.TempDir(), "redump-dc-*.dat")
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
	return db, md5hex
}

func TestHandler_DiscType(t *testing.T) {
	h := dreamcast.New(dreamcast.Deps{})
	if h.DiscType() != state.DiscTypeDC {
		t.Fatalf("disc type: %q, want DC", h.DiscType())
	}
}

func TestIdentify_NoIPBinReader(t *testing.T) {
	// No IPBin reader configured — should return ErrNoCandidates gracefully.
	h := dreamcast.New(dreamcast.Deps{})
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	disc, cands, err := h.Identify(context.Background(), drv)

	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Fatalf("want ErrNoCandidates, got %v", err)
	}
	if disc == nil {
		t.Fatal("want non-nil disc")
	}
	if disc.Type != state.DiscTypeDC {
		t.Errorf("Type = %s, want DC", disc.Type)
	}
	if disc.MetadataProvider != "" {
		t.Errorf("MetadataProvider should be empty, got %q", disc.MetadataProvider)
	}
	if len(cands) != 0 {
		t.Errorf("want 0 candidates, got %d", len(cands))
	}
}

func TestPlan_StepShape(t *testing.T) {
	plan := dreamcast.New(dreamcast.Deps{}).Plan(&state.Disc{}, &state.Profile{})
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

func TestRun_PostRipMD5Hit(t *testing.T) {
	binData := []byte("SONIC_TRACK01_DATA")
	db, _ := dcDB(t, "HDR-0009", "Sonic Adventure", "USA", binData)

	libRoot := t.TempDir()
	workRoot := t.TempDir()

	h := dreamcast.New(dreamcast.Deps{
		Redumper:    &fakeRedumper{binData: binData},
		CHDMan:      &fakeCHDMan{},
		RedumpDB:    db,
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-dc",
		Name:               "DC-CHD",
		OutputPathTemplate: "{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd",
	}
	disc := &state.Disc{
		ID:    "disc-dc-1",
		Type:  state.DiscTypeDC,
		Title: "Dreamcast disc",
	}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	// disc.Title and disc.Region must be updated by the post-rip MD5 lookup.
	if disc.Title != "Sonic Adventure" {
		t.Errorf("disc.Title = %q, want %q", disc.Title, "Sonic Adventure")
	}
	if len(disc.Candidates) == 0 || disc.Candidates[0].Region != "USA" {
		region := ""
		if len(disc.Candidates) > 0 {
			region = disc.Candidates[0].Region
		}
		t.Errorf("disc.Candidates[0].Region = %q, want USA", region)
	}
	if disc.MetadataProvider != "Redump" {
		t.Errorf("MetadataProvider = %q, want Redump", disc.MetadataProvider)
	}
	if disc.MetadataID != "HDR-0009" {
		t.Errorf("MetadataID = %q, want HDR-0009", disc.MetadataID)
	}

	// File must land at the path rendered using the updated title.
	want := filepath.Join(libRoot, "Sonic Adventure (USA)", "Sonic Adventure (USA).chd")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}

	// Step ordering: detect → identify → rip → compress → move → notify → eject.
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
}

func TestRun_PostRipMD5Miss(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()

	// DB has no entries matching the fake bin data.
	db, _ := dcDB(t, "HDR-0009", "Sonic Adventure", "USA", []byte("DIFFERENT_DATA"))

	h := dreamcast.New(dreamcast.Deps{
		// fakeRedumper writes []byte("TRACK01") by default — won't match DB.
		Redumper:    &fakeRedumper{},
		CHDMan:      &fakeCHDMan{},
		RedumpDB:    db,
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-dc",
		Name:               "DC-CHD",
		OutputPathTemplate: "{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd",
	}
	disc := &state.Disc{
		ID:    "disc-dc-2",
		Type:  state.DiscTypeDC,
		Title: "Dreamcast disc",
	}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	// Title stays as placeholder.
	if disc.Title != "Dreamcast disc" {
		t.Errorf("disc.Title = %q, want %q", disc.Title, "Dreamcast disc")
	}

	// File lands using the placeholder title.
	want := filepath.Join(libRoot, "Dreamcast disc ()", "Dreamcast disc ().chd")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}
}

func TestRun_RipFailure(t *testing.T) {
	h := dreamcast.New(dreamcast.Deps{
		Redumper:    &fakeRedumper{err: errors.New("disc unreadable")},
		CHDMan:      &fakeCHDMan{},
		Tools:       newRegistry(),
		LibraryRoot: t.TempDir(),
		WorkRoot:    t.TempDir(),
	})
	prof := &state.Profile{ID: "p", Name: "DC-CHD", OutputPathTemplate: "{{.Title}}.chd"}
	disc := &state.Disc{ID: "disc-dc-3", Type: state.DiscTypeDC, Title: "Dreamcast disc"}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(), drv, disc, prof, sink)
	if err == nil || !strings.Contains(err.Error(), "disc unreadable") {
		t.Errorf("want rip error, got %v", err)
	}
}

func TestDCIdentify_BootCodeIndexFallback(t *testing.T) {
	idx := identify.NewBootCodeIndex()
	if err := idx.MergeFile(state.DiscTypeDC, []byte(`{
		"system":"DC","source":"Libretro","entries":{
			"MK-51000":{"title":"Sonic Adventure","region":"USA","year":1999}
		}
	}`)); err != nil {
		t.Fatal(err)
	}

	h := dreamcast.New(dreamcast.Deps{
		IPBin:         &fakeDCIPBin{info: &identify.DCIPBin{ProductNumber: "MK-51000", SoftwareName: "SONIC ADVENTURE"}},
		RedumpDB:      identify.NewEmptyRedumpDB(),
		BootCodeIndex: idx,
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "drv1"})
	if err != nil {
		t.Fatalf("Identify: %v", err)
	}
	if disc.Title != "Sonic Adventure" {
		t.Errorf("Title = %q", disc.Title)
	}
	if disc.MetadataID != "MK-51000" {
		t.Errorf("MetadataID = %q", disc.MetadataID)
	}
	if len(cands) != 1 || cands[0].Confidence != 90 {
		t.Errorf("cands = %+v", cands)
	}
}

type fakeDCIPBin struct {
	info *identify.DCIPBin
	err  error
}

func (f *fakeDCIPBin) Read(_ context.Context, _ string) (*identify.DCIPBin, error) {
	return f.info, f.err
}

// Compile-time guard.
var _ = pipelines.ErrNoCandidates
