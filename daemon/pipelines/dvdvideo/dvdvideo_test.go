package dvdvideo_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/dvdvideo"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

func TestIsMovieProfile_SelectionMode(t *testing.T) {
	cases := []struct {
		name     string
		profile  *state.Profile
		wantMain bool
	}{
		{
			name: "explicit main_feature wins regardless of format",
			profile: &state.Profile{
				Format:  "MKV",
				Options: map[string]any{"dvd_selection_mode": "main_feature"},
			},
			wantMain: true,
		},
		{
			name: "explicit per_title wins regardless of format",
			profile: &state.Profile{
				Format:  "MP4",
				Options: map[string]any{"dvd_selection_mode": "per_title"},
			},
			wantMain: false,
		},
		{
			name:     "legacy fallback: MP4 format with no option → main_feature",
			profile:  &state.Profile{Format: "MP4", Options: map[string]any{}},
			wantMain: true,
		},
		{
			name:     "legacy fallback: MKV format with no option → per_title",
			profile:  &state.Profile{Format: "MKV", Options: map[string]any{}},
			wantMain: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dvdvideo.IsMovieProfile(c.profile); got != c.wantMain {
				t.Errorf("IsMovieProfile = %v, want %v", got, c.wantMain)
			}
		})
	}
}

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
func (f *fakeTMDB) MovieRuntime(_ context.Context, _ int) (int, error) { return 0, nil }
func (f *fakeTMDB) MovieDetails(_ context.Context, _ int) (identify.DiscMetadata, error) {
	return identify.DiscMetadata{}, nil
}
func (f *fakeTMDB) TVDetails(_ context.Context, _ int) (identify.DiscMetadata, error) {
	return identify.DiscMetadata{}, nil
}

// fakeHandBrake satisfies tools.Tool for the transcode step AND
// dvdvideo.HandBrakeScanner for the post-rip title enumeration.
// On Run: writes a fake output file at args' --output path so the
// move step has something to relocate.
type fakeHandBrake struct {
	scanTitles []tools.HandBrakeTitle
	scanErr    error
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
	return f.scanTitles, f.scanErr
}

// fakeDVDBackup satisfies dvdvideo.DVDMirror. On Mirror it creates an
// `<outDir>/<label>/VIDEO_TS/` stub so the transcode step's input path
// exists; the path it returns is the parent that HandBrake reads.
type fakeDVDBackup struct {
	label string
	err   error
	calls []string
}

func (f *fakeDVDBackup) Mirror(_ context.Context, _ string, outDir string, _ tools.Sink) (string, error) {
	f.calls = append(f.calls, outDir)
	if f.err != nil {
		return "", f.err
	}
	label := f.label
	if label == "" {
		label = "DISC_LABEL"
	}
	dst := filepath.Join(outDir, label, "VIDEO_TS")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	// Touch one VOB so the dir feels real to anything that pokes it.
	_ = os.WriteFile(filepath.Join(dst, "VTS_01_1.VOB"), []byte("stub"), 0o644)
	return filepath.Join(outDir, label), nil
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
	bk := &fakeDVDBackup{label: "ARRIVAL"}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{})
	eject := tools.NewMockTool("eject", []tools.MockEvent{})
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(apprise)
	reg.Register(eject)

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1, // fake encoder writes a stub byte
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

	if len(bk.calls) != 1 {
		t.Errorf("dvdbackup calls: want 1, got %d", len(bk.calls))
	}
	if len(hb.calls) != 1 {
		t.Errorf("encode calls: want 1, got %d", len(hb.calls))
	}
	// HandBrake's --input must point at the local mirror, not /dev/sr0.
	// MP4 (movie) profile must pass --main-feature (lets HandBrake's
	// IFO-aware detection pick the title) and must NOT pass --title.
	if len(hb.calls) > 0 {
		args := hb.calls[0].args
		var hasMainFeature, hasTitle bool
		for i, a := range args {
			if a == "--input" && i+1 < len(args) && args[i+1] == drv.DevPath {
				t.Errorf("HandBrake --input is %s; want local mirror path", args[i+1])
			}
			if a == "--main-feature" {
				hasMainFeature = true
			}
			if a == "--title" {
				hasTitle = true
			}
		}
		if !hasMainFeature {
			t.Errorf("movie profile: HandBrake args missing --main-feature: %v", args)
		}
		if hasTitle {
			t.Errorf("movie profile: HandBrake args should not include --title (let --main-feature pick): %v", args)
		}
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
	bk := &fakeDVDBackup{label: "FRIENDS_S01D1"}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{})
	eject := tools.NewMockTool("eject", []tools.MockEvent{})
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(apprise)
	reg.Register(eject)

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
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

	if len(bk.calls) != 1 {
		t.Errorf("dvdbackup calls: want 1, got %d", len(bk.calls))
	}
	if len(hb.calls) != 3 {
		t.Errorf("encode calls: want 3, got %d", len(hb.calls))
	}
	// Series profile: every encode call must pass --title N (NOT
	// --main-feature, which would collapse the whole series into one
	// encode of the longest title).
	for i, call := range hb.calls {
		var hasMainFeature, hasTitle bool
		for _, a := range call.args {
			if a == "--main-feature" {
				hasMainFeature = true
			}
			if a == "--title" {
				hasTitle = true
			}
		}
		if hasMainFeature {
			t.Errorf("series call[%d]: --main-feature should not be set on a series profile", i)
		}
		if !hasTitle {
			t.Errorf("series call[%d]: --title required on a series profile, args=%v", i, call.args)
		}
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

// Movie disc whose longest scanned title is below the feature floor
// (no play-all PGC, or incomplete mirror) must fail at transcode
// rather than handing a junk reference to --main-feature.
func TestDVD_Run_Movie_ShortLongestTitle_Fails(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 428}, // 7m08
		{Number: 2, DurationSeconds: 312},
		{Number: 3, DurationSeconds: 95},
	}}
	bk := &fakeDVDBackup{label: "JACKASS"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
	})

	err := h.Run(context.Background(),
		&state.Drive{ID: "drv", DevPath: "/dev/sr0"},
		&state.Disc{ID: "d", Type: state.DiscTypeDVD, Title: "Jackass", Year: 2002},
		&state.Profile{
			DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MP4",
			OutputPathTemplate: `{{.Title}}.mp4`,
		},
		testutil.NewRecordingSink())
	if err == nil {
		t.Fatal("want error when longest scanned title is below the floor, got nil")
	}
	if len(hb.calls) != 0 {
		t.Errorf("encode must not run when sanity check fails; calls=%d", len(hb.calls))
	}
}

// User can override the floor to rip legitimately-short content via
// the `min_feature_seconds` profile option.
func TestDVD_Run_Movie_ShortFloorOverride(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 600}, // 10 min, below default 1200
	}}
	bk := &fakeDVDBackup{label: "PIXAR_SHORT"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
	})

	if err := h.Run(context.Background(),
		&state.Drive{ID: "drv", DevPath: "/dev/sr0"},
		&state.Disc{ID: "d", Type: state.DiscTypeDVD, Title: "Short", Year: 2020},
		&state.Profile{
			DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MP4",
			Options:            map[string]any{"min_feature_seconds": 0.0},
			OutputPathTemplate: `{{.Title}}.mp4`,
		},
		testutil.NewRecordingSink()); err != nil {
		t.Fatalf("want override to allow short, got: %v", err)
	}
	if len(hb.calls) != 1 {
		t.Errorf("encode should have run once with override; got %d", len(hb.calls))
	}
}

// Series profile must bypass the movie feature floor — TV episodes
// are routinely 20 min or shorter.
func TestDVD_Run_Series_BelowFloor_OK(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 620},
		{Number: 2, DurationSeconds: 600},
	}}
	bk := &fakeDVDBackup{label: "SITCOM_S01D1"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
	})

	if err := h.Run(context.Background(),
		&state.Drive{ID: "drv", DevPath: "/dev/sr0"},
		&state.Disc{ID: "d", Type: state.DiscTypeDVD, Title: "Sitcom", Year: 1995},
		&state.Profile{
			DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MKV",
			Options:            map[string]any{"min_title_seconds": 300.0, "season": 1.0},
			OutputPathTemplate: `{{.Show}}/Season {{printf "%02d" .Season}}/{{.Show}} - S{{printf "%02d" .Season}}E{{printf "%02d" .EpisodeNumber}}.mkv`,
		},
		testutil.NewRecordingSink()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(hb.calls) != 2 {
		t.Errorf("want 2 episode encodes, got %d", len(hb.calls))
	}
}

func TestDVD_Run_LibraryNotWritable(t *testing.T) {
	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{{Number: 1, DurationSeconds: 7000}}}
	bk := &fakeDVDBackup{}
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
		DVDBackup:        bk,
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

type recordingMetadataStore struct {
	calls []struct {
		id   string
		blob string
	}
}

func (r *recordingMetadataStore) UpdateDiscMetadataBlob(_ context.Context, id, blob string) error {
	r.calls = append(r.calls, struct {
		id   string
		blob string
	}{id, blob})
	return nil
}

func TestDVDPipeline_PersistsScanTitlesToMetadataBlob(t *testing.T) {
	libRoot := t.TempDir()
	store := &recordingMetadataStore{}
	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 5099},
	}}
	bk := &fakeDVDBackup{label: "JACKASS"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", []tools.MockEvent{}))
	reg.Register(tools.NewMockTool("eject", []tools.MockEvent{}))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MetadataStore:            store,
		MinEncodedBytesPerSecond: -1,
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{ID: "disc-1", Type: state.DiscTypeDVD, Title: "Jackass: The Movie", Year: 2002}
	prof := &state.Profile{
		Format:             "MKV",
		OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mkv`,
		Options:            map[string]any{"dvd_selection_mode": "main_feature"},
	}

	if err := h.Run(context.Background(), drv, disc, prof, &testutil.RecordingSink{}); err != nil {
		t.Fatalf("run: %v", err)
	}

	if len(store.calls) == 0 {
		t.Fatalf("expected MetadataStore.UpdateDiscMetadataBlob to be called")
	}
	if !strings.Contains(store.calls[0].blob, `"dvd_titles"`) {
		t.Errorf("blob missing dvd_titles key: %s", store.calls[0].blob)
	}
}

func TestDVD_Run_NVENCSelectsHardwareEncoder(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 7000},
	}}
	bk := &fakeDVDBackup{label: "ARRIVAL"}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{})
	eject := tools.NewMockTool("eject", []tools.MockEvent{})
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(apprise)
	reg.Register(eject)

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
		NVENCAvailable:           true,
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
		VideoCodec:         "nvenc_h265",
		OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mp4`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(hb.calls) != 1 {
		t.Fatalf("encode calls: want 1, got %d", len(hb.calls))
	}
	if gotEncoder := encoderFlag(hb.calls[0].args); gotEncoder != "nvenc_h265" {
		t.Errorf("--encoder: got %q, want nvenc_h265", gotEncoder)
	}
}

func TestDVD_Run_NVENCFallsBackWhenGPUMissing(t *testing.T) {
	libRoot := t.TempDir()

	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 7000},
	}}
	bk := &fakeDVDBackup{label: "ARRIVAL"}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{})
	eject := tools.NewMockTool("eject", []tools.MockEvent{})
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(apprise)
	reg.Register(eject)

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
		NVENCAvailable:           false,
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
		VideoCodec:         "nvenc_h265",
		OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mp4`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gotEncoder := encoderFlag(hb.calls[0].args); gotEncoder != "x265" {
		t.Errorf("--encoder: got %q, want x265 (fallback)", gotEncoder)
	}
}

func encoderFlag(args []string) string {
	for i, a := range args {
		if a == "--encoder" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestDVDPipeline_EmitsMilestoneLogs(t *testing.T) {
	libRoot := t.TempDir()
	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{
		{Number: 1, DurationSeconds: 5099},
	}}
	bk := &fakeDVDBackup{label: "TEST"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", []tools.MockEvent{}))
	reg.Register(tools.NewMockTool("eject", []tools.MockEvent{}))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
	})

	disc := &state.Disc{ID: "disc-1", Type: state.DiscTypeDVD, Title: "Test Movie", Year: 2024}
	prof := &state.Profile{
		Format:             "MKV",
		OutputPathTemplate: `{{.Title}}.mkv`,
		Options:            map[string]any{"dvd_selection_mode": "main_feature"},
	}
	sink := testutil.NewRecordingSink()
	_ = h.Run(context.Background(), &state.Drive{DevPath: "/dev/sr0"}, disc, prof, sink)

	wantSubstrings := []string{
		"dvdbackup: mirroring",
		"dvdbackup: complete",
		"HandBrake: scanning titles",
		"HandBrake: scan complete",
		"HandBrake: encoding title",
		"HandBrake: title 1 complete",
		"move: →",
	}
	for _, want := range wantSubstrings {
		found := false
		for _, e := range sink.Snapshot() {
			if e.Kind == testutil.EventLog && strings.Contains(e.Message, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing milestone log containing %q", want)
		}
	}
}

// hasFlag reports whether args contains the bare flag.
func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

// flagValue returns the argument following flag, or "" if absent.
func flagValue(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func TestDVD_Run_EncodeArgs_SubtitlesAndQualityDefaults(t *testing.T) {
	libRoot := t.TempDir()
	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{{Number: 1, DurationSeconds: 7000}}}
	bk := &fakeDVDBackup{label: "PENGUINS"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
		SubsLang:                 "eng", // must be ignored for an MKV profile
	})

	disc := &state.Disc{ID: "d", Type: state.DiscTypeDVD, Title: "March of the Penguins", Year: 2005}
	prof := &state.Profile{
		DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MKV",
		Options:            map[string]any{"dvd_selection_mode": "main_feature"},
		OutputPathTemplate: `{{.Title}}.mkv`,
	}

	if err := h.Run(context.Background(),
		&state.Drive{ID: "drv", DevPath: "/dev/sr0"}, disc, prof,
		testutil.NewRecordingSink()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(hb.calls) != 1 {
		t.Fatalf("want 1 encode call, got %d", len(hb.calls))
	}
	args := hb.calls[0].args

	// Archival MKV keeps every subtitle track — no language filter.
	if !hasFlag(args, "--all-subtitles") {
		t.Errorf("MKV encode args missing --all-subtitles: %v", args)
	}
	if hasFlag(args, "--subtitle-lang-list") {
		t.Errorf("MKV encode args should not language-filter subtitles: %v", args)
	}
	// Defaults when the profile carries no quality_rf / encoder_preset.
	if v := flagValue(args, "--quality"); v != "18" {
		t.Errorf("--quality: want 18 (default), got %q", v)
	}
	if v := flagValue(args, "--encoder-preset"); v != "slow" {
		t.Errorf("--encoder-preset: want slow (default), got %q", v)
	}
}

func TestDVD_Run_EncodeArgs_ProfileOverridesQuality(t *testing.T) {
	libRoot := t.TempDir()
	hb := &fakeHandBrake{scanTitles: []tools.HandBrakeTitle{{Number: 1, DurationSeconds: 7000}}}
	bk := &fakeDVDBackup{label: "PENGUINS"}
	reg := tools.NewRegistry()
	reg.Register(hb)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := dvdvideo.New(dvdvideo.Deps{
		Tools:                    reg,
		LibraryRoot:              libRoot,
		WorkRoot:                 t.TempDir(),
		LibraryProbe:             func(string) error { return nil },
		DVDBackup:                bk,
		HandBrakeScanner:         hb,
		MinEncodedBytesPerSecond: -1,
	})

	disc := &state.Disc{ID: "d", Type: state.DiscTypeDVD, Title: "Movie", Year: 2020}
	prof := &state.Profile{
		DiscType: state.DiscTypeDVD, Engine: "HandBrake", Format: "MKV",
		Options: map[string]any{
			"dvd_selection_mode": "main_feature",
			"quality_rf":         22,
			"encoder_preset":     "medium",
		},
		OutputPathTemplate: `{{.Title}}.mkv`,
	}

	if err := h.Run(context.Background(),
		&state.Drive{ID: "drv", DevPath: "/dev/sr0"}, disc, prof,
		testutil.NewRecordingSink()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	args := hb.calls[0].args
	if v := flagValue(args, "--quality"); v != "22" {
		t.Errorf("--quality: want 22 (profile override), got %q", v)
	}
	if v := flagValue(args, "--encoder-preset"); v != "medium" {
		t.Errorf("--encoder-preset: want medium (profile override), got %q", v)
	}
}
