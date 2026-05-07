package data_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/data"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines/testutil"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

// fakeLabelProber stubs data.LabelProber.
type fakeLabelProber struct {
	label string
	err   error
}

func (f *fakeLabelProber) Probe(_ context.Context, _ string) (string, error) {
	return f.label, f.err
}

// fakeDD stubs data.DDCopier.
type fakeDD struct {
	content []byte
	err     error
	called  bool
}

func (f *fakeDD) Copy(_ context.Context, _, outPath string, _ int64, _ tools.Sink) error {
	if f.err != nil {
		return f.err
	}
	f.called = true
	return os.WriteFile(outPath, f.content, 0o644)
}

func newRegistry() *tools.Registry {
	r := tools.NewRegistry()
	r.Register(tools.NewMockTool("apprise", nil))
	r.Register(tools.NewMockTool("eject", nil))
	return r
}

func fixedNow() time.Time {
	return time.Date(2024, 3, 15, 10, 30, 45, 0, time.UTC)
}

func TestHandler_DiscType(t *testing.T) {
	h := data.New(data.Deps{})
	if h.DiscType() != state.DiscTypeData {
		t.Fatalf("disc type: %q", h.DiscType())
	}
}

func TestIdentify_LabelPresent(t *testing.T) {
	h := data.New(data.Deps{
		LabelProber: &fakeLabelProber{label: "WIN98"},
	})
	disc, cands, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Fatalf("want ErrNoCandidates, got %v", err)
	}
	if disc.Title != "WIN98" {
		t.Errorf("Title = %q, want WIN98", disc.Title)
	}
	if len(cands) != 0 {
		t.Errorf("want 0 candidates, got %d", len(cands))
	}
}

func TestIdentify_LabelEmpty_FallbackToTimestamp(t *testing.T) {
	h := data.New(data.Deps{
		LabelProber: &fakeLabelProber{label: ""},
		Now:         fixedNow,
	})
	disc, _, err := h.Identify(context.Background(), &state.Drive{ID: "d1", DevPath: "/dev/sr0"})
	if !errors.Is(err, pipelines.ErrNoCandidates) {
		t.Fatalf("want ErrNoCandidates, got %v", err)
	}
	// Fallback title must match data-disc-YYYYMMDD-HHMMSS pattern.
	matched, _ := regexp.MatchString(`^data-disc-\d{8}-\d{6}$`, disc.Title)
	if !matched {
		t.Errorf("fallback title %q does not match expected pattern", disc.Title)
	}
	// Verify the fixed time produces a deterministic title.
	want := "data-disc-20240315-103045"
	if disc.Title != want {
		t.Errorf("Title = %q, want %q", disc.Title, want)
	}
}

func TestPlan_StepShape(t *testing.T) {
	plan := data.New(data.Deps{}).Plan(&state.Disc{}, &state.Profile{})
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
		t.Errorf("compress should be skipped")
	}
}

func TestRun_HappyPath(t *testing.T) {
	libRoot := t.TempDir()
	workRoot := t.TempDir()
	content := []byte("hello world")
	dd := &fakeDD{content: content}

	h := data.New(data.Deps{
		DD:          dd,
		LabelProber: &fakeLabelProber{label: "WIN98"},
		Tools:       newRegistry(),
		LibraryRoot: libRoot,
		WorkRoot:    workRoot,
	})
	prof := &state.Profile{
		ID:                 "p-data",
		Name:               "Data",
		OutputPathTemplate: "{{.Title}}/{{.Title}}.iso",
	}
	disc := &state.Disc{
		ID:    "disc-1",
		Type:  state.DiscTypeData,
		Title: "WIN98",
	}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	if err := h.Run(context.Background(), drv, disc, prof, sink); err != nil {
		t.Fatal(err)
	}

	// dd was called.
	if !dd.called {
		t.Error("expected dd.Copy to be called")
	}

	// sha256 of "hello world" stored in disc.TOCHash; size in SizeBytesRaw.
	// disc.Disc has no Notes field; TOCHash carries the content digest for data discs.
	wantHash := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	if disc.TOCHash != wantHash {
		t.Errorf("TOCHash = %q, want %q", disc.TOCHash, wantHash)
	}
	if disc.SizeBytesRaw != 11 {
		t.Errorf("SizeBytesRaw = %d, want 11", disc.SizeBytesRaw)
	}

	// Atomic move target: LibraryRoot/WIN98/WIN98.iso.
	want := filepath.Join(libRoot, "WIN98", "WIN98.iso")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("expected file at %s: %v", want, err)
	}
}

func TestRun_DDFailure(t *testing.T) {
	h := data.New(data.Deps{
		DD:          &fakeDD{err: errors.New("read error")},
		LabelProber: &fakeLabelProber{label: "WIN98"},
		Tools:       newRegistry(),
		LibraryRoot: t.TempDir(),
		WorkRoot:    t.TempDir(),
	})
	prof := &state.Profile{ID: "p", Name: "Data", OutputPathTemplate: "{{.Title}}/{{.Title}}.iso"}
	disc := &state.Disc{ID: "disc-2", Type: state.DiscTypeData, Title: "WIN98"}
	drv := &state.Drive{ID: "d1", DevPath: "/dev/sr0"}

	sink := testutil.NewRecordingSink()
	err := h.Run(context.Background(), drv, disc, prof, sink)
	if err == nil || !strings.Contains(err.Error(), "read error") {
		t.Errorf("want dd error, got %v", err)
	}
}

// Compile-time guard.
var _ = pipelines.ErrNoCandidates
