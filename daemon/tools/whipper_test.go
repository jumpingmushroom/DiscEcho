package tools_test

import (
	"context"
	"os"
	"strings"
	"testing"

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

type recordingSink struct {
	events []recordedEvent
}

func (r *recordingSink) Progress(pct float64, speed string, eta int) {
	r.events = append(r.events, recordedEvent{kind: "progress", pct: pct, speed: speed, eta: eta})
}
func (r *recordingSink) Log(level state.LogLevel, format string, args ...any) {
	r.events = append(r.events, recordedEvent{
		kind: "log", level: level,
		message: tools.FormatLog(format, args...),
	})
}

func TestWhipper_ParseStdout_KindOfBlue(t *testing.T) {
	body, err := os.ReadFile("testdata/whipper-stdout-kindofblue.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &recordingSink{}
	tools.ParseWhipperStream(strings.NewReader(string(body)), sink)

	progressEvents := 0
	var first, last recordedEvent
	for _, e := range sink.events {
		if e.kind == "progress" {
			if progressEvents == 0 {
				first = e
			}
			last = e
			progressEvents++
		}
	}
	if progressEvents == 0 {
		t.Fatal("no progress events emitted")
	}
	if first.pct < 2.0 || first.pct > 3.0 {
		t.Errorf("first progress: want ~2.5, got %.2f", first.pct)
	}
	if last.pct < 99.0 || last.pct > 100.0 {
		t.Errorf("last progress: want ~99.8, got %.2f", last.pct)
	}
	if first.speed != "8.0×" {
		t.Errorf("first speed: got %q", first.speed)
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
