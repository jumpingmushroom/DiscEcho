package audiocd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// fakeWhipperInternal is a minimal whipper fake usable from package
// audiocd tests. It writes one .flac per track into workdir/album/...
// and reports the requested AccurateRip confidences via RunWithResult.
type fakeWhipperInternal struct {
	tracks []int
	ar     map[int]int
}

func (f *fakeWhipperInternal) Name() string { return "whipper" }
func (f *fakeWhipperInternal) Run(_ context.Context, _ []string, _ map[string]string,
	workdir string, _ tools.Sink) error {
	dir := filepath.Join(workdir, "album", "Artist - Title")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, n := range f.tracks {
		fname := filepathSprintf("%02d. Artist - Track %d.flac", n, n)
		if err := os.WriteFile(filepath.Join(dir, fname), []byte("fake"), 0o644); err != nil {
			return err
		}
	}
	return nil
}
func (f *fakeWhipperInternal) RunWithResult(ctx context.Context, args []string, env map[string]string,
	workdir string, sink tools.Sink) (tools.WhipperResult, error) {
	if err := f.Run(ctx, args, env, workdir, sink); err != nil {
		return tools.WhipperResult{AccurateRip: map[int]int{}}, err
	}
	ar := map[int]int{}
	for k, v := range f.ar {
		ar[k] = v
	}
	return tools.WhipperResult{AccurateRip: ar}, nil
}

// filepathSprintf is a tiny printf shim so this test file doesn't need
// an `fmt` import.
func filepathSprintf(format string, args ...any) string {
	// We only ever feed it integer-formatted strings, so the simple
	// path through a local helper is enough.
	return sprintfMini(format, args...)
}

// sprintfMini is the dumbest possible fmt.Sprintf used only by the
// test's filename builder; keeps the dependency footprint nil. Handles
// the two formats we need: %02d and %d.
func sprintfMini(format string, args ...any) string {
	var out strings.Builder
	ai := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' || i == len(format)-1 {
			out.WriteByte(format[i])
			continue
		}
		i++
		// Parse optional width (single digit; enough for "02").
		width := 0
		if format[i] >= '0' && format[i] <= '9' {
			width = int(format[i] - '0')
			i++
			if i < len(format) && format[i] >= '0' && format[i] <= '9' {
				width = width*10 + int(format[i]-'0')
				i++
			}
		}
		if i >= len(format) || format[i] != 'd' {
			continue
		}
		v, ok := args[ai].(int)
		if !ok {
			continue
		}
		ai++
		s := intToString(v)
		for len(s) < width {
			s = "0" + s
		}
		out.WriteString(s)
	}
	return out.String()
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	digits := ""
	for n > 0 {
		digits = string(rune('0'+n%10)) + digits
		n /= 10
	}
	if neg {
		return "-" + digits
	}
	return digits
}

type fakeMetaflac struct {
	mu        sync.Mutex
	calls     []string // per-flac path passed to EmbedFrontCover
	failPaths map[string]bool
	missing   bool
}

func (f *fakeMetaflac) Name() string { return "metaflac" }
func (f *fakeMetaflac) Run(_ context.Context, _ []string, _ map[string]string, _ string, _ tools.Sink) error {
	return nil
}
func (f *fakeMetaflac) EmbedFrontCover(_ context.Context, flacPath, _ string) error {
	if f.missing {
		return tools.ErrToolNotInstalled
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, flacPath)
	if f.failPaths[flacPath] {
		return errors.New("simulated embed failure")
	}
	return nil
}

type fakeLoudgain struct {
	mu      sync.Mutex
	calls   [][]string // each Run is one slice of FLAC paths
	missing bool
	failErr error
}

func (f *fakeLoudgain) Name() string { return "loudgain" }
func (f *fakeLoudgain) Run(_ context.Context, _ []string, _ map[string]string, _ string, _ tools.Sink) error {
	return nil
}
func (f *fakeLoudgain) TagAlbum(_ context.Context, paths []string) error {
	if f.missing {
		return tools.ErrToolNotInstalled
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, append([]string(nil), paths...))
	return f.failErr
}

// TestRunPostRipExtras_EmbedAndReplayGainHappyPath wires a fake CAA
// server + fake metaflac + fake loudgain and asserts both helpers run.
func TestRunPostRipExtras_EmbedAndReplayGainHappyPath(t *testing.T) {
	libRoot := t.TempDir()

	caa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8\xff\xe0fakejpeg"))
	}))
	defer caa.Close()
	defer swapCoverArtClient(caa.Client())()
	defer swapReleaseURLBase(caa.URL)()
	defer swapReleaseGroupURLBase(caa.URL)()

	whip := &fakeWhipperInternal{tracks: []int{1, 2}, ar: map[int]int{1: 87, 2: 92}}
	meta := &fakeMetaflac{}
	lg := &fakeLoudgain{}
	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(meta)
	reg.Register(lg)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := New(Deps{
		Tools:                reg,
		LibraryRoot:          libRoot,
		WorkRoot:             t.TempDir(),
		LibraryProbe:         func(string) error { return nil },
		MusicBrainzBaseURL:   caa.URL,
		MusicBrainzUserAgent: "DiscEcho-test/1",
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0", ReadOffset: 102, ReadOffsetSource: "manual"}
	disc := &state.Disc{
		ID: "d", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
		Title: "T", MetadataID: "mb-x",
		Candidates: []state.Candidate{{Source: "MusicBrainz", Title: "T", Artist: "Artist"}},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
		OutputPathTemplate: `{{printf "%02d" .TrackNumber}}.flac`,
		Options: map[string]any{
			"embed_cover_art":       true,
			"replaygain_album_mode": true,
		},
	}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(meta.calls) != 2 {
		t.Errorf("metaflac.EmbedFrontCover call count: want 2, got %d (%v)", len(meta.calls), meta.calls)
	}
	if len(lg.calls) != 1 {
		t.Errorf("loudgain.TagAlbum call count: want 1, got %d", len(lg.calls))
	}
	if len(lg.calls) > 0 && len(lg.calls[0]) != 2 {
		t.Errorf("loudgain.TagAlbum FLACs: want 2, got %d (%v)", len(lg.calls[0]), lg.calls[0])
	}

	// Substep transitions emitted in order: embed-art, replaygain, "" (clear).
	wantSubSteps := []string{"embed-art", "replaygain", ""}
	gotSubSteps := []string{}
	for _, e := range sink.Snapshot() {
		if e.Kind == testutil.EventSubStep {
			gotSubSteps = append(gotSubSteps, e.SubStep)
		}
	}
	if len(gotSubSteps) != len(wantSubSteps) {
		t.Fatalf("sub-step count: want %d, got %d (%v)", len(wantSubSteps), len(gotSubSteps), gotSubSteps)
	}
	for i := range wantSubSteps {
		if gotSubSteps[i] != wantSubSteps[i] {
			t.Errorf("sub-step[%d]: want %q, got %q", i, wantSubSteps[i], gotSubSteps[i])
		}
	}
}

// TestRunPostRipExtras_TogglesOff confirms both options disabled means
// neither helper is invoked — no metaflac, no loudgain, no sub-steps.
func TestRunPostRipExtras_TogglesOff(t *testing.T) {
	libRoot := t.TempDir()
	whip := &fakeWhipperInternal{tracks: []int{1}}
	meta := &fakeMetaflac{}
	lg := &fakeLoudgain{}
	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(meta)
	reg.Register(lg)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := New(Deps{
		Tools: reg, LibraryRoot: libRoot, WorkRoot: t.TempDir(),
		LibraryProbe: func(string) error { return nil },
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
	disc := &state.Disc{
		ID: "d", Type: state.DiscTypeAudioCD, DriveID: "drv-1",
		Title: "T", MetadataID: "mb-x",
		Candidates: []state.Candidate{{Source: "MusicBrainz", Title: "T", Artist: "A"}},
	}
	prof := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Engine: "whipper", Format: "FLAC",
		OutputPathTemplate: `{{printf "%02d" .TrackNumber}}.flac`,
		Options: map[string]any{
			"embed_cover_art":       false,
			"replaygain_album_mode": false,
		},
	}
	if err := h.Run(context.Background(), drv, disc, prof, testutil.NewRecordingSink()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(meta.calls) != 0 {
		t.Errorf("metaflac called despite toggle off: %v", meta.calls)
	}
	if len(lg.calls) != 0 {
		t.Errorf("loudgain called despite toggle off: %v", lg.calls)
	}
}

// TestRunPostRipExtras_LoudgainMissingDoesNotFailRip asserts the
// best-effort contract: a missing loudgain binary logs a WARN and the
// rip still succeeds end-to-end (files moved, eject fired).
func TestRunPostRipExtras_LoudgainMissingDoesNotFailRip(t *testing.T) {
	libRoot := t.TempDir()

	caa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write([]byte("\xff\xd8fake"))
	}))
	defer caa.Close()
	defer swapCoverArtClient(caa.Client())()
	defer swapReleaseURLBase(caa.URL)()
	defer swapReleaseGroupURLBase(caa.URL)()

	whip := &fakeWhipperInternal{tracks: []int{1}}
	meta := &fakeMetaflac{}
	lg := &fakeLoudgain{missing: true}
	reg := tools.NewRegistry()
	reg.Register(whip)
	reg.Register(meta)
	reg.Register(lg)
	reg.Register(tools.NewMockTool("apprise", nil))
	reg.Register(tools.NewMockTool("eject", nil))

	h := New(Deps{
		Tools: reg, LibraryRoot: libRoot, WorkRoot: t.TempDir(),
		LibraryProbe:         func(string) error { return nil },
		MusicBrainzBaseURL:   caa.URL,
		MusicBrainzUserAgent: "DiscEcho-test/1",
	})

	drv := &state.Drive{ID: "drv-1", DevPath: "/dev/sr0"}
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
		t.Fatalf("Run unexpectedly failed despite missing loudgain: %v", err)
	}
	// Embed should still have run (loudgain miss doesn't poison the cover step).
	if len(meta.calls) != 1 {
		t.Errorf("metaflac.EmbedFrontCover call count: want 1, got %d", len(meta.calls))
	}
	// Loudgain TagAlbum should NOT have logged a call (returned
	// ErrToolNotInstalled before recording).
	if len(lg.calls) != 0 {
		t.Errorf("loudgain.TagAlbum was called despite missing binary: %v", lg.calls)
	}

	wantWARN := "loudgain binary not installed"
	sawWARN := false
	for _, e := range sink.Snapshot() {
		if e.Kind == testutil.EventLog && e.Level == state.LogLevelWarn && strings.Contains(e.Message, wantWARN) {
			sawWARN = true
		}
	}
	if !sawWARN {
		t.Errorf("expected a WARN log containing %q; got log events: %+v", wantWARN, sink.Snapshot())
	}
}

// TestOptBoolDefaultTrue covers the pure helper. Missing key, nil value,
// non-bool stored value, and an explicit false should all behave per the
// documented contract.
func TestOptBoolDefaultTrue(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts map[string]any
		key  string
		want bool
	}{
		{name: "missing-defaults-true", opts: map[string]any{}, key: "x", want: true},
		{name: "nil-defaults-true", opts: map[string]any{"x": nil}, key: "x", want: true},
		{name: "non-bool-defaults-true", opts: map[string]any{"x": "yes"}, key: "x", want: true},
		{name: "explicit-true", opts: map[string]any{"x": true}, key: "x", want: true},
		{name: "explicit-false", opts: map[string]any{"x": false}, key: "x", want: false},
		{name: "nil-opts", opts: nil, key: "x", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := optBoolDefaultTrue(tc.opts, tc.key); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

// Compile-time assurance: the in-tree EventSink Recording must expose
// SubStep so the sink.Snapshot loop can read the field. The pipelines
// package owns the EventSink interface; we just rely on testutil.
var _ pipelines.EventSink = (*testutil.RecordingSink)(nil)
