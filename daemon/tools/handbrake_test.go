package tools_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

func TestHandBrake_ParseScan_OneTitle(t *testing.T) {
	body, err := os.ReadFile("testdata/handbrake-scan-1title.txt")
	if err != nil {
		t.Fatal(err)
	}
	titles, err := tools.ParseHandBrakeScan(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 {
		t.Fatalf("want 1 title, got %d", len(titles))
	}
	if titles[0].Number != 1 {
		t.Errorf("number: %d", titles[0].Number)
	}
	want := 1*3600 + 56*60 + 32 // 01:56:32
	if titles[0].DurationSeconds != want {
		t.Errorf("duration: want %d, got %d", want, titles[0].DurationSeconds)
	}
}

func TestHandBrake_ParseScan_MultiTitle(t *testing.T) {
	body, err := os.ReadFile("testdata/handbrake-scan-multi.txt")
	if err != nil {
		t.Fatal(err)
	}
	titles, err := tools.ParseHandBrakeScan(string(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 5 {
		t.Fatalf("want 5 titles, got %d", len(titles))
	}
	if titles[3].DurationSeconds != 42 {
		t.Errorf("title 4 duration: %d", titles[3].DurationSeconds)
	}
}

func TestHandBrake_ParseScan_NoTitles(t *testing.T) {
	_, err := tools.ParseHandBrakeScan("HandBrake has exited.\n")
	if err == nil {
		t.Errorf("want error when no titles found")
	}
}

func TestHandBrake_ParseEncodeProgress(t *testing.T) {
	body, err := os.ReadFile("testdata/handbrake-encode-progress.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	tools.ParseHandBrakeEncodeStream(strings.NewReader(string(body)), 1, 1, sink)

	progressEvents := 0
	var lastPct float64
	for _, e := range sink.events {
		if e.kind == "progress" {
			progressEvents++
			lastPct = e.pct
		}
	}
	if progressEvents != 4 {
		t.Errorf("want 4 progress events, got %d", progressEvents)
	}
	if lastPct < 99.0 || lastPct > 100.0 {
		t.Errorf("last pct: %.2f", lastPct)
	}
}

// HandBrake separates progress updates with '\r', not '\n'. Before
// 0.3.3 the parser used the default ScanLines split which treats '\r'
// as content — a long stream of progress chunks accumulated as one
// mega-token and the scanner aborted, deadlocking the subprocess.
func TestHandBrake_ParseEncodeProgress_CarriageReturnSeparated(t *testing.T) {
	stream := strings.NewReader(
		"Encoding: task 1 of 1, 10.00 % (avg fps 30.0, ETA 00h00m30s)\r" +
			"Encoding: task 1 of 1, 50.00 % (avg fps 30.0, ETA 00h00m20s)\r" +
			"Encoding: task 1 of 1, 99.00 % (avg fps 30.0, ETA 00h00m01s)\r",
	)
	sink := &recordingSink{}
	tools.ParseHandBrakeEncodeStream(stream, 1, 1, sink)

	progressEvents := 0
	for _, e := range sink.events {
		if e.kind == "progress" {
			progressEvents++
		}
	}
	if progressEvents != 3 {
		t.Errorf("want 3 progress events from \\r-separated input, got %d", progressEvents)
	}
}

// HandBrake 1.6.1 (and likely surrounding releases) emits encode
// progress as a bare percentage when stdout is a pipe — no
// `(avg fps X, ETA YhYmYs)` tail. The parser must accept both forms.
func TestHandBrake_ParseEncodeProgress_NoParensTail(t *testing.T) {
	stream := strings.NewReader(
		"Encoding: task 1 of 1, 1.55 %\r" +
			"Encoding: task 1 of 1, 13.81 %\r" +
			"Encoding: task 1 of 1, 65.16 %\r",
	)
	sink := &recordingSink{}
	tools.ParseHandBrakeEncodeStream(stream, 1, 1, sink)

	progressEvents := 0
	var pcts []float64
	for _, e := range sink.events {
		if e.kind == "progress" {
			progressEvents++
			pcts = append(pcts, e.pct)
		}
	}
	if progressEvents != 3 {
		t.Fatalf("want 3 progress events from bare-percentage input, got %d", progressEvents)
	}
	if pcts[0] < 1.4 || pcts[0] > 1.7 {
		t.Errorf("pct[0]: want ~1.55, got %.2f", pcts[0])
	}
	if pcts[2] < 65.0 || pcts[2] > 65.3 {
		t.Errorf("pct[2]: want ~65.16, got %.2f", pcts[2])
	}
	for _, e := range sink.events {
		if e.kind == "progress" && e.eta != 0 {
			t.Errorf("eta should be 0 when no ETA tail in input, got %d", e.eta)
		}
	}
}

func TestHandBrake_ProgressForOneOfThreeTitles(t *testing.T) {
	// titleIdx=2 of totalTitles=3 → overall = ((1) + intra/100)/3 * 100
	// for intra=50 → overall = (1 + 0.5)/3 * 100 = 50.0
	stream := strings.NewReader("Encoding: task 1 of 1, 50.00 % (avg fps 30.0, ETA 00h00m10s)\n")
	sink := &recordingSink{}
	tools.ParseHandBrakeEncodeStream(stream, 2, 3, sink)
	if len(sink.events) != 1 {
		t.Fatalf("want 1 event, got %d", len(sink.events))
	}
	if sink.events[0].pct < 49.0 || sink.events[0].pct > 51.0 {
		t.Errorf("overall pct: %.2f", sink.events[0].pct)
	}
}

func TestHandBrake_Name(t *testing.T) {
	h := tools.NewHandBrake("/usr/bin/HandBrakeCLI")
	if h.Name() != "handbrake" {
		t.Errorf("name: %q", h.Name())
	}
}

func TestHandBrake_RunFailsCleanly(t *testing.T) {
	h := tools.NewHandBrake("/usr/bin/false")
	err := h.Run(context.Background(), []string{"--scan"}, nil, "", tools.NopSink{})
	if err == nil {
		t.Errorf("want exec error from /usr/bin/false")
	}
}

func TestHandBrake_ParseEncodeStream_PassesThroughNonProgressLines(t *testing.T) {
	stream := strings.NewReader(
		"Encoding: task 1 of 1, 10.00 %\r" +
			"[14:30:21] sync: reached audio 0x80bd pts 541440, exiting early\n" +
			"x264 [error]: nal write failed\n" +
			"Encoding: task 1 of 1, 50.00 %\r",
	)
	sink := &recordingSink{}
	tools.ParseHandBrakeEncodeStream(stream, 1, 1, sink)

	warns := 0
	for _, e := range sink.events {
		if e.kind == "log" {
			warns++
		}
	}
	if warns == 0 {
		t.Errorf("expected non-progress lines forwarded as log events; got 0")
	}

	// [hh:mm:ss] config-dump lines (no 'error' substring) should be skipped.
	for _, e := range sink.events {
		if e.kind == "log" && strings.Contains(e.message, "reached audio") {
			t.Errorf("config-dump line should be skipped, got %q", e.message)
		}
	}

	// x264 [error] should land as warn.
	sawX264 := false
	for _, e := range sink.events {
		if e.kind == "log" && strings.Contains(e.message, "x264 [error]") {
			sawX264 = true
		}
	}
	if !sawX264 {
		t.Errorf("expected x264 [error] line forwarded as warn")
	}
}

func TestHandBrake_ParseEncodeStream_StderrCap(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&sb, "x264 [error]: line %d\n", i)
	}
	sink := &recordingSink{}
	tools.ParseHandBrakeEncodeStream(strings.NewReader(sb.String()), 1, 1, sink)

	logs := 0
	for _, e := range sink.events {
		if e.kind == "log" {
			logs++
		}
	}
	// 200 lines + 1 cap-marker = 201 events
	if logs != 201 {
		t.Errorf("expected 201 log events (200 + cap marker), got %d", logs)
	}
}
