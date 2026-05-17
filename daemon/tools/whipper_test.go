package tools_test

import (
	"context"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type recordedEvent struct {
	kind    string
	pct     float64
	speed   string
	eta     int
	level   state.LogLevel
	message string
}

// recordingSink captures events for assertions. The lock is needed
// for ParseWhipperStreams, which fans out to stdout + stderr parser
// goroutines that both call into the sink concurrently.
type recordingSink struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (r *recordingSink) Progress(pct float64, speed string, eta int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{kind: "progress", pct: pct, speed: speed, eta: eta})
}
func (r *recordingSink) Log(level state.LogLevel, format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{
		kind: "log", level: level,
		message: tools.FormatLog(format, args...),
	})
}
func (r *recordingSink) SubStep(string) {}

func TestWhipper_ParseStdout_KindOfBlue(t *testing.T) {
	body, err := os.ReadFile("testdata/whipper-stdout-kindofblue.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	tools.ParseWhipperStream(strings.NewReader(string(body)), sink)

	progressEvents := 0
	var firstReading, last recordedEvent
	firstReadingFound := false
	for _, e := range sink.events {
		if e.kind != "progress" {
			continue
		}
		// Boundary-derived emits carry empty speed; skip them when
		// asserting on the per-percent values from "Reading:" lines.
		if !firstReadingFound && e.speed != "" {
			firstReading = e
			firstReadingFound = true
		}
		last = e
		progressEvents++
	}
	if progressEvents == 0 {
		t.Fatal("no progress events emitted")
	}
	if !firstReadingFound {
		t.Fatal("no Reading-derived progress event emitted")
	}
	if firstReading.pct < 2.0 || firstReading.pct > 3.0 {
		t.Errorf("first Reading progress: want ~2.5, got %.2f", firstReading.pct)
	}
	if last.pct < 99.0 || last.pct > 100.0 {
		t.Errorf("last progress: want ~99.8, got %.2f", last.pct)
	}
	if firstReading.speed != "8.0×" {
		t.Errorf("first Reading speed: got %q", firstReading.speed)
	}
}

func TestParseWhipperStreams_AnnouncePreparingDriveOnce(t *testing.T) {
	// Both streams produce output; the shared announce flag should
	// limit the "preparing drive" log line to a single occurrence.
	stdout := strings.NewReader("INFO:whipper.command.cd:checking device /dev/sr0\n")
	stderr := strings.NewReader("INFO:whipper.image.cue:reading TOC\n")
	sink := &recordingSink{}
	tools.ParseWhipperStreams(stdout, stderr, sink)

	count := 0
	for _, e := range sink.events {
		if e.kind == "log" && strings.Contains(e.message, "preparing drive") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected one 'preparing drive' log line, got %d", count)
	}
}

func TestWhipper_ParseStdout_FailureLineLogsError(t *testing.T) {
	body, err := os.ReadFile("testdata/whipper-stdout-failure.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	tools.ParseWhipperStream(strings.NewReader(string(body)), sink)

	sawError := false
	for _, e := range sink.events {
		if e.kind == "log" && e.level == state.LogLevelError {
			sawError = true
			if !strings.Contains(e.message, "Read failure") {
				t.Errorf("error message should mention 'Read failure': %q", e.message)
			}
		}
	}
	if !sawError {
		t.Error("expected an error log line")
	}
}

func TestWhipper_Name(t *testing.T) {
	w := tools.NewWhipper("/usr/bin/whipper")
	if w.Name() != "whipper" {
		t.Errorf("name: got %q", w.Name())
	}
}

func TestWhipper_RunFailsCleanly(t *testing.T) {
	w := tools.NewWhipper("/usr/bin/false")
	err := w.Run(context.Background(), []string{"x"}, nil, "", tools.NopSink{})
	if err == nil {
		t.Errorf("want exec error from /usr/bin/false")
	}
}

func progressOnly(events []recordedEvent) []recordedEvent {
	out := make([]recordedEvent, 0, len(events))
	for _, e := range events {
		if e.kind == "progress" {
			out = append(out, e)
		}
	}
	return out
}

func TestParseWhipperStream_BoundaryProgress(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	input := strings.Join([]string{
		"Ripping track 1 of 11: 01. Lead Poisoning.flac",
		"Track 1 OK (AccurateRip: 5/5 conf)",
		"Ripping track 2 of 11: 02. Seven Blackbirds.flac",
		"Track 2 OK (AccurateRip: 5/5 conf)",
	}, "\n")
	tools.ParseWhipperStream(strings.NewReader(input), sink)

	got := progressOnly(sink.events)
	wantSeq := []float64{
		0.0,                // start of track 1: (1-1)/11 * 100
		1.0 / 11.0 * 100.0, // track 1 OK: 1/11 * 100
		1.0 / 11.0 * 100.0, // start of track 2: (2-1)/11 * 100
		2.0 / 11.0 * 100.0, // track 2 OK: 2/11 * 100
	}
	if len(got) != len(wantSeq) {
		t.Fatalf("progress events: want %d, got %d (%v)", len(wantSeq), len(got), got)
	}
	for i, want := range wantSeq {
		if diff := got[i].pct - want; diff > 0.001 || diff < -0.001 {
			t.Errorf("progress[%d]: want %.3f, got %.3f", i, want, got[i].pct)
		}
		if got[i].speed != "" {
			t.Errorf("progress[%d].speed: want empty, got %q", i, got[i].speed)
		}
		if got[i].eta != 0 {
			t.Errorf("progress[%d].eta: want 0, got %d", i, got[i].eta)
		}
	}
}

func TestParseWhipperStream_ModernPythonLogging_EmitsProgressAndETA(t *testing.T) {
	// Simulate a slow drive: each parser step advances the clock by
	// 180s. Track 1 starts at t=0, completes at t=180s; track 2 starts
	// at t=360s, completes at t=540s; … After track 1's CRCs-match we
	// have 1 completed track in 180s elapsed → ETA for the remaining 3
	// tracks is 540s.
	var nowCalls int
	base := time.Date(2026, 5, 16, 12, 0, 0, 0, time.UTC)
	restore := tools.SetWhipperClockForTest(func() time.Time {
		t := base.Add(time.Duration(nowCalls) * 180 * time.Second)
		nowCalls++
		return t
	})
	defer restore()

	body, err := os.ReadFile("testdata/whipper-stdout-modern.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	tools.ParseWhipperStream(strings.NewReader(string(body)), sink)

	got := progressOnly(sink.events)
	// 4 track-start emits + 4 CRCs-match emits = 8 progress events.
	wantSeq := []float64{0, 25, 25, 50, 50, 75, 75, 100}
	if len(got) != len(wantSeq) {
		t.Fatalf("progress events: want %d, got %d (%v)", len(wantSeq), len(got), got)
	}
	for i, want := range wantSeq {
		if diff := got[i].pct - want; diff > 0.001 || diff < -0.001 {
			t.Errorf("progress[%d].pct: want %.1f, got %.3f", i, want, got[i].pct)
		}
	}
	// Track 1 start: completed=0 → ETA=0.
	if got[0].eta != 0 {
		t.Errorf("progress[0].eta (track 1 start): want 0, got %d", got[0].eta)
	}
	// Track 1 CRCs match: completed=1, elapsed=180s, remaining=3 → 540s.
	if got[1].eta == 0 {
		t.Error("progress[1].eta (track 1 done): want non-zero, got 0")
	}
	// After track 2 completes (completed=2, more elapsed) the ETA
	// should still be a positive, decreasing-over-time estimate.
	if got[3].eta == 0 {
		t.Error("progress[3].eta (track 2 done): want non-zero, got 0")
	}
	// Final emit (track 4 done, completed==totalTracks) → ETA=0.
	if got[7].eta != 0 {
		t.Errorf("progress[7].eta (track 4 done, all complete): want 0, got %d", got[7].eta)
	}
}

func TestParseWhipperStream_ReadingOverridesBoundary(t *testing.T) {
	t.Parallel()
	sink := &recordingSink{}
	input := strings.Join([]string{
		"Ripping track 1 of 11: 01. Lead Poisoning.flac",
		"  Reading: 42.5%, 8.0×, ETA: 1:30",
	}, "\n")
	tools.ParseWhipperStream(strings.NewReader(input), sink)

	got := progressOnly(sink.events)
	if len(got) != 2 {
		t.Fatalf("progress events: want 2, got %d (%v)", len(got), got)
	}
	// First event: boundary (track-start) at 0%.
	if got[0].pct != 0 {
		t.Errorf("progress[0].pct: want 0, got %f", got[0].pct)
	}
	// Second event: Reading-derived. Overall = ((1-1) + 0.425) / 11 * 100.
	want := 0.425 / 11.0 * 100.0
	if diff := got[1].pct - want; diff > 0.001 || diff < -0.001 {
		t.Errorf("progress[1].pct: want %.3f, got %.3f", want, got[1].pct)
	}
	if got[1].speed != "8.0×" {
		t.Errorf("progress[1].speed: want 8.0×, got %q", got[1].speed)
	}
	if got[1].eta != 90 {
		t.Errorf("progress[1].eta: want 90, got %d", got[1].eta)
	}
}
