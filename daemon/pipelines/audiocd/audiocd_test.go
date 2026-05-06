package audiocd_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

type trackInfo struct {
	num   int
	title string
}

type fakeWhipper struct {
	tracks []trackInfo
}

func (f *fakeWhipper) Name() string { return "whipper" }
func (f *fakeWhipper) Run(_ context.Context, _ []string, _ map[string]string,
	workdir string, sink tools.Sink) error {
	for _, tr := range f.tracks {
		fname := fmt.Sprintf("track%02d.flac", tr.num)
		_ = os.WriteFile(filepath.Join(workdir, fname), []byte("fake"), 0o644)
		sink.Progress(float64(tr.num)/float64(len(f.tracks))*100, "10×", 5)
	}
	sink.Log(state.LogLevelInfo, "whipper: all tracks ripped")
	return nil
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
