package dvdvideo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/dvdvideo"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type fakeProber struct {
	info *identify.DVDInfo
	err  error
}

func (f *fakeProber) Probe(_ context.Context, _ string) (*identify.DVDInfo, error) {
	return f.info, f.err
}

type fakeTMDB struct {
	cands []state.Candidate
	err   error
}

func (f *fakeTMDB) SearchMovie(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, f.err
}
func (f *fakeTMDB) SearchTV(_ context.Context, _ string) ([]state.Candidate, error) {
	return nil, nil
}
func (f *fakeTMDB) SearchBoth(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, f.err
}

// fakeHandBrake satisfies tools.Tool AND dvdvideo.HandBrakeScanner.
// On encode runs (Run): writes a fake output file at args' --output path.
type fakeHandBrake struct {
	scanTitles []tools.HandBrakeTitle
	encodeErr  error
	calls      []encodeCall
}

type encodeCall struct {
	args    []string
	workdir string
}

func (f *fakeHandBrake) Name() string { return "handbrake" }
func (f *fakeHandBrake) Run(_ context.Context, args []string, _ map[string]string,
	workdir string, _ tools.Sink) error {
	f.calls = append(f.calls, encodeCall{args: args, workdir: workdir})
	if f.encodeErr != nil {
		return f.encodeErr
	}
	for i, a := range args {
		if a == "--output" && i+1 < len(args) {
			_ = os.WriteFile(args[i+1], []byte("fake"), 0o644)
		}
	}
	return nil
}

func (f *fakeHandBrake) Scan(_ context.Context, _ string) ([]tools.HandBrakeTitle, error) {
	return f.scanTitles, nil
}

func TestDVD_DiscType(t *testing.T) {
	h := dvdvideo.New(dvdvideo.Deps{Tools: tools.NewRegistry()})
	if h.DiscType() != state.DiscTypeDVD {
		t.Errorf("got %s", h.DiscType())
	}
}

func TestDVD_Identify_GoodLabel(t *testing.T) {
	h := dvdvideo.New(dvdvideo.Deps{
		Prober: &fakeProber{info: &identify.DVDInfo{VolumeLabel: "BLADE_RUNNER_2049"}},
		TMDB: &fakeTMDB{cands: []state.Candidate{
			{Source: "TMDB", Title: "Blade Runner 2049", Year: 2017, Confidence: 80, TMDBID: 1, MediaType: "movie"},
		}},
		Tools: tools.NewRegistry(),
	})

	disc, cands, err := h.Identify(context.Background(), &state.Drive{DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Type != state.DiscTypeDVD {
		t.Errorf("type: %s", disc.Type)
	}
	if disc.Title != "Blade Runner 2049" || disc.Year != 2017 {
		t.Errorf("disc: %+v", disc)
	}
	if disc.MetadataProvider != "TMDB" || disc.MetadataID != "1" {
		t.Errorf("metadata: %s/%s", disc.MetadataProvider, disc.MetadataID)
	}
	if len(cands) != 1 || cands[0].MediaType != "movie" {
		t.Errorf("cands: %+v", cands)
	}
}

func TestDVD_Identify_GarbageLabel(t *testing.T) {
	h := dvdvideo.New(dvdvideo.Deps{
		Prober: &fakeProber{info: &identify.DVDInfo{VolumeLabel: "DVD_VIDEO"}},
		TMDB:   &fakeTMDB{},
		Tools:  tools.NewRegistry(),
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{DevPath: "/dev/sr0"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestDVD_Identify_TMDBEmpty(t *testing.T) {
	h := dvdvideo.New(dvdvideo.Deps{
		Prober: &fakeProber{info: &identify.DVDInfo{VolumeLabel: "OBSCURE_MOVIE_X"}},
		TMDB:   &fakeTMDB{cands: nil},
		Tools:  tools.NewRegistry(),
	})
	_, _, err := h.Identify(context.Background(), &state.Drive{DevPath: "/dev/sr0"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestDVD_Plan(t *testing.T) {
	h := dvdvideo.New(dvdvideo.Deps{Tools: tools.NewRegistry()})
	plan := h.Plan(&state.Disc{Type: state.DiscTypeDVD},
		&state.Profile{DiscType: state.DiscTypeDVD})

	if len(plan) != 8 {
		t.Fatalf("want 8 steps, got %d", len(plan))
	}
	skipped := 0
	for _, s := range plan {
		if s.Skip {
			skipped++
			if s.ID != state.StepCompress {
				t.Errorf("only compress should be skipped, got %s", s.ID)
			}
		}
	}
	if skipped != 1 {
		t.Errorf("want 1 skipped step, got %d", skipped)
	}
}

func TestDVD_Run_MovieEndToEnd(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 7000},
	}}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{})
	eject := tools.NewMockTool("eject", []tools.MockEvent{})
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(apprise)
	reg.Register(eject)

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:            reg,
		LibraryRoot:      libRoot,
		WorkRoot:         t.TempDir(),
		LibraryProbe:     func(string) error { return nil },
		HandBrakeScanner: hb,
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{
		ID: "disc-1", Type: state.DiscTypeDVD, DriveID: "drv-1",
		Title: "Arrival", Year: 2016,
		MetadataID: "329865", MetadataProvider: "TMDB",
		Candidates: []state.Candidate{
			{Source: "TMDB", Title: "Arrival", Year: 2016, MediaType: "movie", TMDBID: 329865, Confidence: 80},
		},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MP4",
		OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mp4`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(hb.calls) != 1 {
		t.Errorf("encode calls: want 1, got %d", len(hb.calls))
	}

	expected := filepath.Join(libRoot, "Arrival (2016)", "Arrival (2016).mp4")
	if _, err := os.Stat(expected); err != nil {
		matches, _ := filepath.Glob(filepath.Join(libRoot, "*", "*.mp4"))
		t.Errorf("expected %s, found: %v", expected, matches)
	}
}

func TestDVD_Run_SeriesMultiTitle(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 1330},
		{Number: 2, DurationSeconds: 1318},
		{Number: 3, DurationSeconds: 42}, // < 5min — filtered
		{Number: 4, DurationSeconds: 1384},
	}}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{})
	eject := tools.NewMockTool("eject", []tools.MockEvent{})
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(apprise)
	reg.Register(eject)

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:            reg,
		LibraryRoot:      libRoot,
		WorkRoot:         t.TempDir(),
		LibraryProbe:     func(string) error { return nil },
		HandBrakeScanner: hb,
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{
		ID: "disc-2", Type: state.DiscTypeDVD, DriveID: "drv-1",
		Title: "Friends", Year: 1994,
		MetadataID: "1668", MetadataProvider: "TMDB",
		Candidates: []state.Candidate{
			{Source: "TMDB", Title: "Friends", Year: 1994, MediaType: "tv", TMDBID: 1668, Confidence: 90},
		},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MKV",
		Options: map[string]any{
			"min_title_seconds": 300.0,
			"season":            1.0,
		},
		OutputPathTemplate: `{{.Show}}/Season {{printf "%02d" .Season}}/{{.Show}} - S{{printf "%02d" .Season}}E{{printf "%02d" .EpisodeNumber}}.mkv`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(hb.calls) != 3 {
		t.Errorf("encode calls: want 3, got %d", len(hb.calls))
	}

	matches, _ := filepath.Glob(filepath.Join(libRoot, "Friends", "Season 01", "*.mkv"))
	sort.Strings(matches)
	if len(matches) != 3 {
		t.Fatalf("want 3 files, got %d: %v", len(matches), matches)
	}
	for i, m := range matches {
		want := filepath.Join(libRoot, "Friends", "Season 01",
			fmt.Sprintf("Friends - S01E%02d.mkv", i+1))
		if m != want {
			t.Errorf("file[%d]: got %s, want %s", i, m, want)
		}
	}
}

func TestDVD_Run_LibraryNotWritable(t *testing.T) {
	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{{Number: 1, DurationSeconds: 7000}}}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:       reg,
		LibraryRoot: "/library",
		WorkRoot:    t.TempDir(),
		LibraryProbe: func(_ string) error {
			return errors.New("not writable")
		},
		HandBrakeScanner: hb,
	})

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(),
		&state.Drive{DevPath: "/dev/sr0"},
		&state.Disc{Type: state.DiscTypeDVD, Title: "x"},
		&state.Profile{OutputPathTemplate: `{{.Title}}.mp4`}, sink)
	if err == nil {
		t.Errorf("want error from probe failure")
	}
}
