package tools_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type captureSinkRedumper struct {
	progress []float64
	logs     []string
}

func (c *captureSinkRedumper) Progress(pct float64, _ string, _ int) {
	c.progress = append(c.progress, pct)
}
func (c *captureSinkRedumper) Log(_ state.LogLevel, format string, _ ...any) {
	c.logs = append(c.logs, format)
}

func TestParseRedumperProgress(t *testing.T) {
	b, err := os.ReadFile("testdata/redumper-progress.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSinkRedumper{}
	tools.ParseRedumperProgress(bytes.NewReader(b), sink)
	if len(sink.progress) == 0 {
		t.Fatal("expected progress events")
	}
	first := sink.progress[0]
	last := sink.progress[len(sink.progress)-1]
	if first != 0 {
		t.Errorf("first progress = %f, want 0", first)
	}
	if last < 99.9 || last > 100.1 {
		t.Errorf("last progress = %f, want ~100", last)
	}
}

func TestParseRedumperProgress_IgnoresUnknownLines(t *testing.T) {
	in := bytes.NewBufferString("garbage line\nLBA: 50/100\nmore garbage\n")
	sink := &captureSinkRedumper{}
	tools.ParseRedumperProgress(in, sink)
	if len(sink.progress) != 1 {
		t.Errorf("want 1 progress event, got %d", len(sink.progress))
	}
	if sink.progress[0] != 50.0 {
		t.Errorf("progress = %f, want 50", sink.progress[0])
	}
}

func TestNewRedumper_Defaults(t *testing.T) {
	r := tools.NewRedumper("")
	if r == nil {
		t.Fatal("nil")
	}
	// Rip against a missing device should error cleanly.
	err := r.Rip(context.Background(), "/dev/null", t.TempDir(), "x", "cd", &captureSinkRedumper{})
	if err == nil {
		t.Errorf("want error from /dev/null")
	}
}

func TestRedumperName(t *testing.T) {
	r := tools.NewRedumper("")
	if r.Name() != "redumper" {
		t.Errorf("Name = %q, want redumper", r.Name())
	}
}

func TestParseRedumperProgress_BothModes(t *testing.T) {
	// Sanity: the parser doesn't care about cd vs dvd mode (mode is a
	// caller hint, not parser state). LBA progress lines are the same shape.
	in := strings.NewReader("LBA: 0/100\nLBA: 100/100\n")
	sink := &captureSinkRedumper{}
	tools.ParseRedumperProgress(in, sink)
	if len(sink.progress) != 2 {
		t.Errorf("want 2 events, got %d", len(sink.progress))
	}
}

func TestRedumperRip_RejectsUnknownMode(t *testing.T) {
	r := tools.NewRedumper("")
	err := r.Rip(context.Background(), "/dev/null", t.TempDir(), "x", "blu-ray", &captureSinkRedumper{})
	if err == nil || !strings.Contains(err.Error(), "mode") {
		t.Errorf("want mode error, got %v", err)
	}
}

func TestRedumperOutputExt_Xbox(t *testing.T) {
	if got := tools.RedumperOutputExt("xbox"); got != ".iso" {
		t.Fatalf("xbox: got %q, want .iso", got)
	}
}

func TestRedumperOutputExt_DVDStillIso(t *testing.T) {
	if got := tools.RedumperOutputExt("dvd"); got != ".iso" {
		t.Fatalf("dvd: got %q, want .iso", got)
	}
}

func TestRedumperOutputExt_CDStillCue(t *testing.T) {
	if got := tools.RedumperOutputExt("cd"); got != ".cue" {
		t.Fatalf("cd: got %q, want .cue", got)
	}
}

func TestRedumperRip_AcceptsXboxMode(t *testing.T) {
	r := tools.NewRedumper("")
	// xbox is a valid mode; redumper binary won't exist in CI so we
	// expect a start error, not a mode-rejection error.
	err := r.Rip(context.Background(), "/dev/null", t.TempDir(), "x", "xbox", &captureSinkRedumper{})
	if err != nil && strings.Contains(err.Error(), "unknown mode") {
		t.Fatalf("xbox mode rejected: %v", err)
	}
}
