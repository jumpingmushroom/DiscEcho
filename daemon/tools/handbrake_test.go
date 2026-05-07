package tools_test

import (
	"context"
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
