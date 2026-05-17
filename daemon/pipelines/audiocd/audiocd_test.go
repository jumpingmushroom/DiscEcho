package audiocd_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/audiocd"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type fakeTOCReader struct {
	toc *identify.TOC
	err error
}

func (f *fakeTOCReader) Read(_ context.Context, _ string) (*identify.TOC, error) {
	return f.toc, f.err
}

type fakeMBClient struct {
	cands []state.Candidate
	err   error
}

func (f *fakeMBClient) Lookup(_ context.Context, _ string) ([]state.Candidate, error) {
	return f.cands, f.err
}
func (f *fakeMBClient) ReleaseDetails(_ context.Context, _ string) (identify.AudioCDMetadata, error) {
	return identify.AudioCDMetadata{}, nil
}
func (f *fakeMBClient) SearchByName(_ context.Context, _ string) ([]state.Candidate, error) {
	return nil, nil
}

type trackInfo struct {
	num   int
	title string
}

type fakeWhipper struct {
	tracks []trackInfo
	// subdir, if non-empty, is the directory path (relative to workdir)
	// that whipper writes outputs into. Real whipper writes to
	// `album/<Artist> - <Album>/` inside the working dir; the empty
	// default keeps older tests that expected flat output working.
	subdir string
	// nameFn, if non-nil, builds the per-track filename. Defaults to
	// `trackNN.flac`. Real whipper writes `NN. Artist - Title.flac`; the
	// duplicate-track-number test uses that shape to pin the strip.
	nameFn func(trackInfo) string
	// arConfidence, if non-nil, returns the per-track AccurateRip
	// confidence the test wants RunWithResult to report. Track numbers
	// not in the map are absent from the result (so verified < total).
	arConfidence map[int]int
	// observedArgs records the most recent argv passed to Run, so the
	// offset-arg tests can assert on `-o <N>`.
	observedArgs []string
}

func (f *fakeWhipper) Name() string { return "whipper" }
func (f *fakeWhipper) Run(_ context.Context, args []string, _ map[string]string,
	workdir string, sink tools.Sink) error {
	f.observedArgs = append([]string(nil), args...)
	dir := workdir
	if f.subdir != "" {
		dir = filepath.Join(workdir, f.subdir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	for _, tr := range f.tracks {
		var fname string
		if f.nameFn != nil {
			fname = f.nameFn(tr)
		} else {
			fname = fmt.Sprintf("track%02d.flac", tr.num)
		}
		_ = os.WriteFile(filepath.Join(dir, fname), []byte("fake"), 0o644)
		sink.Progress(float64(tr.num)/float64(len(f.tracks))*100, "10×", 5)
	}
	sink.Log(state.LogLevelInfo, "whipper: all tracks ripped")
	return nil
}

// RunWithResult satisfies the optional WhipperResultRunner interface
// the audiocd handler probes for so it can persist per-track AR data.
func (f *fakeWhipper) RunWithResult(ctx context.Context, args []string, env map[string]string,
	workdir string, sink tools.Sink) (tools.WhipperResult, error) {
	if err := f.Run(ctx, args, env, workdir, sink); err != nil {
		return tools.WhipperResult{AccurateRip: map[int]int{}}, err
	}
	ar := map[int]int{}
	for k, v := range f.arConfidence {
		ar[k] = v
	}
	return tools.WhipperResult{AccurateRip: ar}, nil
}

func TestAudioCD_DiscType(t *testing.T) {
	h := audiocd.New(audiocd.Deps{Tools: tools.NewRegistry()})
	if h.DiscType() != state.DiscTypeAudioCD {
		t.Errorf("got %s", h.DiscType())
	}
}

func TestAudioCD_Identify(t *testing.T) {
	h := audiocd.New(audiocd.Deps{
		TOC: &fakeTOCReader{toc: &identify.TOC{
			Tracks: []identify.Track{
				{Number: 1, StartLBA: 150, LengthLBA: 14955, IsAudio: true},
				{Number: 2, StartLBA: 15105, LengthLBA: 21915, IsAudio: true},
			},
			LeadoutLBA: 37020,
		}},
		MB: &fakeMBClient{cands: []state.Candidate{
			{Source: "MusicBrainz", Title: "Test", Confidence: 90, MBID: "abc"},
		}},
		Tools: tools.NewRegistry(),
	})

	disc, cands, err := h.Identify(context.Background(), &state.Drive{DevPath: "/dev/sr0"})
	if err != nil {
		t.Fatal(err)
	}
	if disc.Type != state.DiscTypeAudioCD {
		t.Errorf("want AUDIO_CD, got %s", disc.Type)
	}
	if disc.TOCHash == "" {
		t.Errorf("disc id should be computed")
	}
	if len(cands) != 1 {
		t.Errorf("want 1 candidate, got %d", len(cands))
	}
}

func TestAudioCD_Identify_NoCandidatesReturnsError(t *testing.T) {
	h := audiocd.New(audiocd.Deps{
		TOC: &fakeTOCReader{toc: &identify.TOC{
			Tracks:     []identify.Track{{Number: 1, StartLBA: 150, LengthLBA: 1000, IsAudio: true}},
			LeadoutLBA: 1150,
		}},
		MB:    &fakeMBClient{cands: nil},
		Tools: tools.NewRegistry(),
	})

	_, _, err := h.Identify(context.Background(), &state.Drive{DevPath: "/dev/sr0"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Errorf("want ErrNoCandidates, got %v", err)
	}
}

func TestAudioCD_Plan(t *testing.T) {
	h := audiocd.New(audiocd.Deps{Tools: tools.NewRegistry()})
	plan := h.Plan(&state.Disc{Type: state.DiscTypeAudioCD},
		&state.Profile{DiscType: state.DiscTypeAudioCD})

	if len(plan) != 8 {
		t.Fatalf("want 8 steps, got %d", len(plan))
	}
	skipped := map[state.StepID]bool{}
	for _, s := range plan {
		if s.Skip {
			skipped[s.ID] = true
		}
	}
	if !skipped[state.StepTranscode] || !skipped[state.StepCompress] {
		t.Errorf("transcode and compress should be skipped: %+v", skipped)
	}
	if skipped[state.StepRip] || skipped[state.StepMove] {
		t.Errorf("rip and move should not be skipped")
	}
}

func TestAudioCD_Run_FullPipeline(t *testing.T) {
	libRoot := t.TempDir()

	whip := &fakeWhipper{tracks: []trackInfo{
		{num: 1, title: "So What"}, {num: 2, title: "Freddie Freeloader"},
	}}
	apprise := tools.NewMockTool("apprise", []tools.MockEvent{
		{Log: &tools.MockLog{Level: state.LogLevelInfo, Format: "sent"}},
	})
	eject := tools.NewMockTool("eject", []tools.MockEvent{
		{Log: &tools.MockLog{Level: state.LogLevelInfo, Format: "ejected"}},
	})

	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(apprise)
	reg.Register(eject)

	h := audiocd.New(audiocd.Deps{
		Tools:        reg,
		LibraryRoot:  libRoot,
		WorkRoot:     t.TempDir(),
		LibraryProbe: func(string) error { return nil },
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{
		ID: "disc-1", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
		Title: "Kind of Blue", Year: 1959,
		MetadataID: "abc-123",
		Candidates: []state.Candidate{
			{Source: "MusicBrainz", Title: "Kind of Blue", Artist: "Miles Davis", Year: 1959, MBID: "abc-123"},
		},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
		OutputPathTemplate: `{{.Artist}}/{{.Album}} ({{.Year}})/{{printf "%02d" .TrackNumber}}.flac`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}

	seq := sink.StepSequence()
	want := []state.StepID{state.StepDetect, state.StepIdentify, state.StepRip, state.StepMove, state.StepNotify, state.StepEject}
	if len(seq) != len(want) {
		t.Fatalf("step sequence: want %v, got %v", want, seq)
	}
	for i := range want {
		if seq[i] != want[i] {
			t.Errorf("step %d: want %s, got %s", i, want[i], seq[i])
		}
	}

	matches, _ := filepath.Glob(filepath.Join(libRoot, "Miles Davis", "Kind of Blue (1959)", "*.flac"))
	if len(matches) != 2 {
		t.Errorf("want 2 FLACs in library, got %d (%v)", len(matches), matches)
	}

	if len(apprise.Calls()) != 1 {
		t.Errorf("apprise calls: %d", len(apprise.Calls()))
	}
	if len(eject.Calls()) != 1 {
		t.Errorf("eject calls: %d", len(eject.Calls()))
	}
	// eject must have received drv.DevPath as its first arg
	if got := eject.Calls()[0].Args; len(got) == 0 || got[0] != "/dev/sr0" {
		t.Errorf("eject args: %v", got)
	}
}

func TestAudioCD_Run_LibraryNotWritable(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.NewMockTool("whipper", nil))
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := audiocd.New(audiocd.Deps{
		Tools:       reg,
		LibraryRoot: "/library",
		WorkRoot:    t.TempDir(),
		LibraryProbe: func(_ string) error {
			return errors.New("not writable")
		},
	})

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(),
		&state.Drive{DevPath: "/dev/sr0"},
		&state.Disc{Type: state.DiscTypeAudioCD},
		&state.Profile{OutputPathTemplate: `{{.Title}}.flac`}, sink)
	if err == nil {
		t.Errorf("want error from probe failure")
	}
	failed := false
	for _, e := range sink.Snapshot() {
		if e.Kind == testutil.EventFailed && e.Step == state.StepRip {
			failed = true
		}
	}
	if !failed {
		t.Errorf("expected rip step failure")
	}
}

// TestAudioCD_Run_MovesNestedWhipperOutput pins the bug that lost the
// first prod audio rip: real whipper writes outputs into a nested
// `album/<Artist> - <Album>/` directory, but the move step used
// os.ReadDir(workdir) and skipped subdirectories. moveOutputs now
// walks the workdir recursively; this test bakes that in.
func TestAudioCD_Run_MovesNestedWhipperOutput(t *testing.T) {
	libRoot := t.TempDir()

	whip := &fakeWhipper{
		subdir: "album/Trust Obey - Fear and Bullets",
		tracks: []trackInfo{
			{num: 1, title: "Lead Poisoning"},
			{num: 2, title: "Seven Blackbirds"},
		},
	}
	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := audiocd.New(audiocd.Deps{
		Tools:        reg,
		LibraryRoot:  libRoot,
		WorkRoot:     t.TempDir(),
		LibraryProbe: func(string) error { return nil },
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{
		ID: "disc-fb", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
		Title: "Fear and Bullets", Year: 1997,
		MetadataID: "fb-mb",
		Candidates: []state.Candidate{
			{Source: "MusicBrainz", Title: "Fear and Bullets", Artist: "Trust Obey", Year: 1997, MBID: "fb-mb"},
		},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
		OutputPathTemplate: `{{.Artist}}/{{.Album}} ({{.Year}})/{{printf "%02d" .TrackNumber}}.flac`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(libRoot, "Trust Obey", "Fear and Bullets (1997)", "*.flac"))
	if len(matches) != 2 {
		t.Fatalf("nested output not picked up: want 2 FLACs, got %d (%v)", len(matches), matches)
	}

	// The move step's notes should carry both destination paths.
	var moveNotes map[string]any
	for _, e := range sink.Snapshot() {
		if e.Kind == testutil.EventDone && e.Step == state.StepMove {
			moveNotes = e.Notes
		}
	}
	if moveNotes == nil {
		t.Fatal("move step had no done event")
	}
	paths, _ := moveNotes["paths"].([]string)
	if len(paths) != 2 {
		t.Errorf("move notes paths: want 2, got %v", paths)
	}
}

// TestAudioCD_Run_FailsLoudlyWhenWhipperProducesNoFLACs proves the
// belt-and-braces guard: if a successful whipper exit leaves an empty
// (or FLAC-less) workdir, the move step fails rather than silently
// reporting paths: nil — which is how the missing-files bug looked in
// the wild.
func TestAudioCD_Run_FailsLoudlyWhenWhipperProducesNoFLACs(t *testing.T) {
	libRoot := t.TempDir()
	whip := &fakeWhipper{tracks: nil}
	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := audiocd.New(audiocd.Deps{
		Tools:        reg,
		LibraryRoot:  libRoot,
		WorkRoot:     t.TempDir(),
		LibraryProbe: func(string) error { return nil },
	})

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(),
		&state.Drive{DevPath: "/dev/sr0"},
		&state.Disc{
			Type: state.DiscTypeAudioCD,
			Candidates: []state.Candidate{
				{Source: "MusicBrainz", Artist: "X", Title: "Y"},
			},
		},
		&state.Profile{OutputPathTemplate: `{{.Title}}.flac`}, sink)
	if err == nil {
		t.Fatal("expected move failure when no FLACs are present")
	}
	moveFailed := false
	for _, e := range sink.Snapshot() {
		if e.Kind == testutil.EventFailed && e.Step == state.StepMove {
			moveFailed = true
		}
	}
	if !moveFailed {
		t.Error("expected StepMove failure event")
	}
}

// TestAudioCD_Run_PassesDriveReadOffsetToWhipper guards the v0.20.0
// switch from hardcoded `-o 0` to the persisted per-drive calibration:
// the rip command must echo whatever drv.ReadOffset is, including
// negative values (real Pioneer drives sit at -1164).
func TestAudioCD_Run_PassesDriveReadOffsetToWhipper(t *testing.T) {
	for _, tc := range []struct {
		name   string
		offset int
		want   string
	}{
		{"zero", 0, "0"},
		{"positive", 667, "667"},
		{"negative", -1164, "-1164"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			libRoot := t.TempDir()
			whip := &fakeWhipper{tracks: []trackInfo{{num: 1, title: "T1"}}}
			reg := tools.NewRegistry()
			reg.Register(whip)
			reg.Register(tools.NewMockTool("apprise", nil))
			reg.Register(tools.NewMockTool("eject", nil))

			h := audiocd.New(audiocd.Deps{
				Tools: reg, LibraryRoot: libRoot, WorkRoot: t.TempDir(),
				LibraryProbe: func(string) error { return nil },
			})

			drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0", ReadOffset: tc.offset}
			disc := &state.Disc{
				ID: "d", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
				Title: "T", MetadataID: "mb-x",
				Candidates: []state.Candidate{{Source: "MusicBrainz", Title: "T", Artist: "A"}},
			}
			prof := &state.Profile{
				DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
				OutputPathTemplate: `{{.Title}}.flac`,
			}

			if err := h.Run(context.Background(), drv, disc, prof, testutil.NewRecordingSink()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			var sawOffset bool
			for i, a := range whip.observedArgs {
				if a == "-o" && i+1 < len(whip.observedArgs) {
					if got := whip.observedArgs[i+1]; got != tc.want {
						t.Errorf("offset arg: want %q, got %q (full args: %v)", tc.want, got, whip.observedArgs)
					}
					sawOffset = true
				}
			}
			if !sawOffset {
				t.Errorf("no `-o` arg passed to whipper: %v", whip.observedArgs)
			}
		})
	}
}

// TestAudioCD_Run_PersistsAccurateRipSummary covers the three states the
// UI badge needs to render: verified (all tracks match), unverified
// (calibrated but mismatches present), and uncalibrated (no offset set —
// status pinned so we don't surface false "mismatch" warnings).
func TestAudioCD_Run_PersistsAccurateRipSummary(t *testing.T) {
	for _, tc := range []struct {
		name           string
		offset         int
		source         string
		ar             map[int]int
		wantStatus     string
		wantVerified  int
		wantTotal     int
		wantHaveNotes bool
	}{
		{
			name: "verified-all-tracks", offset: 667, source: "manual",
			ar:           map[int]int{1: 87, 2: 92, 3: 81},
			wantStatus:   "verified", wantVerified: 3, wantTotal: 3, wantHaveNotes: true,
		},
		{
			name: "unverified-partial", offset: 667, source: "manual",
			ar:           map[int]int{1: 87, 2: 0, 3: 5},
			wantStatus:   "unverified", wantVerified: 2, wantTotal: 3, wantHaveNotes: true,
		},
		{
			name: "uncalibrated-with-data-pins-status", offset: 0, source: "",
			ar:           map[int]int{1: 1, 2: 1},
			wantStatus:   "uncalibrated", wantVerified: 2, wantTotal: 2, wantHaveNotes: true,
		},
		{
			name: "uncalibrated-no-data-emits-no-notes", offset: 0, source: "",
			ar:           nil,
			wantHaveNotes: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			libRoot := t.TempDir()
			tracks := []trackInfo{}
			for i := 1; i <= 3; i++ {
				tracks = append(tracks, trackInfo{num: i, title: fmt.Sprintf("T%d", i)})
			}
			whip := &fakeWhipper{tracks: tracks, arConfidence: tc.ar}
			reg := tools.NewRegistry()
			reg.Register(whip)
			reg.Register(tools.NewMockTool("apprise", nil))
			reg.Register(tools.NewMockTool("eject", nil))

			h := audiocd.New(audiocd.Deps{
				Tools: reg, LibraryRoot: libRoot, WorkRoot: t.TempDir(),
				LibraryProbe: func(string) error { return nil },
			})

			drv := &state.Drive{
				ID: "drv-1", DevPath: "/dev/sr0",
				ReadOffset: tc.offset, ReadOffsetSource: tc.source,
			}
			disc := &state.Disc{
				ID: "d", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
				Title: "T", MetadataID: "mb-x",
				Candidates: []state.Candidate{{Source: "MusicBrainz", Title: "T", Artist: "A"}},
			}
			prof := &state.Profile{
				DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
				OutputPathTemplate: `{{printf "%02d" .TrackNumber}}.flac`,
			}

			sink := testutil.NewRecordingSink()
			if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
				t.Fatalf("Run: %v", err)
			}

			var ripNotes map[string]any
			for _, e := range sink.Snapshot() {
				if e.Kind == testutil.EventDone && e.Step == state.StepRip {
					ripNotes = e.Notes
				}
			}
			if !tc.wantHaveNotes {
				if ripNotes != nil {
					t.Errorf("want empty rip-step notes, got %v", ripNotes)
				}
				return
			}
			if ripNotes == nil {
				t.Fatal("expected accuraterip notes on rip-step done event")
			}
			ar, _ := ripNotes["accuraterip"].(map[string]any)
			if ar == nil {
				t.Fatalf("notes lacks 'accuraterip' map: %v", ripNotes)
			}
			if got := ar["status"]; got != tc.wantStatus {
				t.Errorf("status: want %q, got %v", tc.wantStatus, got)
			}
			if got, _ := ar["verified_tracks"].(int); got != tc.wantVerified {
				t.Errorf("verified_tracks: want %d, got %v", tc.wantVerified, ar["verified_tracks"])
			}
			if got, _ := ar["total_tracks"].(int); got != tc.wantTotal {
				t.Errorf("total_tracks: want %d, got %v", tc.wantTotal, ar["total_tracks"])
			}
		})
	}
}

// TestAudioCD_Run_StripsWhipperTrackPrefix pins the bug where the
// default audio-CD template (`{{printf "%02d" .TrackNumber}} - {{.Title}}.flac`)
// rendered the track number twice because whipper's per-track filename
// already starts with `NN. Artist - Title`. moveOutputs now strips that
// prefix before feeding the basename in as `.Title`.
func TestAudioCD_Run_StripsWhipperTrackPrefix(t *testing.T) {
	libRoot := t.TempDir()

	whip := &fakeWhipper{
		subdir: "album/Graeme Revell - The Crow",
		tracks: []trackInfo{
			{num: 1, title: "Birth of the Legend"},
			{num: 2, title: "Resurrection"},
		},
		nameFn: func(tr trackInfo) string {
			return fmt.Sprintf("%02d. Graeme Revell - %s.flac", tr.num, tr.title)
		},
	}
	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := audiocd.New(audiocd.Deps{
		Tools:        reg,
		LibraryRoot:  libRoot,
		WorkRoot:     t.TempDir(),
		LibraryProbe: func(string) error { return nil },
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{
		ID: "disc-crow", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
		Title: "The Crow", Year: 1994,
		MetadataID: "crow-mb",
		Candidates: []state.Candidate{
			{Source: "MusicBrainz", Title: "The Crow", Artist: "Graeme Revell", Year: 1994, MBID: "crow-mb"},
		},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
		OutputPathTemplate: `{{.Artist}}/{{.Album}} ({{.Year}})/{{printf "%02d" .TrackNumber}} - {{.Title}}.flac`,
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}

	matches, _ := filepath.Glob(filepath.Join(libRoot, "Graeme Revell", "The Crow (1994)", "*.flac"))
	if len(matches) != 2 {
		t.Fatalf("want 2 FLACs, got %d (%v)", len(matches), matches)
	}
	for _, m := range matches {
		base := filepath.Base(m)
		// Reject the dup-prefix shape: `NN - NN. ...`.
		if strings.HasPrefix(base, "01 - 01.") || strings.HasPrefix(base, "02 - 02.") {
			t.Errorf("track-number prefix not stripped: %s", base)
		}
	}
	wantNames := map[string]bool{
		"01 - Graeme Revell - Birth of the Legend.flac": true,
		"02 - Graeme Revell - Resurrection.flac":        true,
	}
	for _, m := range matches {
		base := filepath.Base(m)
		if !wantNames[base] {
			t.Errorf("unexpected filename: %s", base)
		}
	}
}
