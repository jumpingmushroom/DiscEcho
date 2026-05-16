package tools_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type captureSinkCHDMan struct {
	progress []float64
	logs     []string
}

func (c *captureSinkCHDMan) Progress(pct float64, _ string, _ int) {
	c.progress = append(c.progress, pct)
}
func (c *captureSinkCHDMan) Log(_ state.LogLevel, format string, args ...any) {
	c.logs = append(c.logs, fmt.Sprintf(format, args...))
}

func TestParseCHDManProgress(t *testing.T) {
	b, err := os.ReadFile("testdata/chdman-progress.txt")
	if err != nil {
		t.Fatal(err)
	}
	sink := &captureSinkCHDMan{}
	tools.ParseCHDManProgress(bytes.NewReader(b), sink)
	if len(sink.progress) < 2 {
		t.Fatalf("expected ≥2 events, got %d", len(sink.progress))
	}
	last := sink.progress[len(sink.progress)-1]
	if last < 99.9 || last > 100.1 {
		t.Errorf("last progress = %f, want ~100", last)
	}
}

func TestParseCHDManProgress_ForwardsUnknownLinesToLog(t *testing.T) {
	in := bytes.NewBufferString("hello world\nCompressing, 25.0% complete... (ratio=80.0%)\nbye\n")
	sink := &captureSinkCHDMan{}
	tools.ParseCHDManProgress(in, sink)
	if len(sink.progress) != 1 {
		t.Errorf("want 1 progress event, got %d", len(sink.progress))
	}
	if sink.progress[0] != 25.0 {
		t.Errorf("progress = %f, want 25", sink.progress[0])
	}
	if len(sink.logs) != 2 {
		t.Fatalf("want 2 log lines, got %d: %v", len(sink.logs), sink.logs)
	}
	if sink.logs[0] != "chdman: hello world" {
		t.Errorf("log[0] = %q, want %q", sink.logs[0], "chdman: hello world")
	}
	if sink.logs[1] != "chdman: bye" {
		t.Errorf("log[1] = %q, want %q", sink.logs[1], "chdman: bye")
	}
}

func TestParseCHDManProgress_CRTerminatedLines(t *testing.T) {
	// chdman overwrites its progress line with '\r' rather than advancing
	// with '\n'. Verify each CR-separated chunk is parsed as a separate line.
	in := bytes.NewBufferString(
		"Compressing, 0.0% complete... (ratio=100.0%)\r" +
			"Compressing, 50.0% complete... (ratio=80.0%)\r" +
			"Compressing, 100.0% complete... (ratio=75.0%)\r",
	)
	sink := &captureSinkCHDMan{}
	tools.ParseCHDManProgress(in, sink)
	if len(sink.progress) != 3 {
		t.Fatalf("want 3 progress events, got %d", len(sink.progress))
	}
	if sink.progress[0] != 0.0 {
		t.Errorf("event[0] = %f, want 0", sink.progress[0])
	}
	if sink.progress[1] != 50.0 {
		t.Errorf("event[1] = %f, want 50", sink.progress[1])
	}
	if sink.progress[2] != 100.0 {
		t.Errorf("event[2] = %f, want 100", sink.progress[2])
	}
}

func TestParseCHDManProgress_LongLineIsTruncated(t *testing.T) {
	// Lines longer than 400 chars should be truncated before forwarding
	// to the log rather than passed through verbatim.
	long := strings.Repeat("y", 500)
	in := bytes.NewBufferString(long + "\n")
	sink := &captureSinkCHDMan{}
	tools.ParseCHDManProgress(in, sink)
	if len(sink.logs) != 1 {
		t.Fatalf("want 1 log line, got %d", len(sink.logs))
	}
	want := "chdman: " + strings.Repeat("y", 400)
	if sink.logs[0] != want {
		t.Errorf("log[0] len=%d, want len=%d", len(sink.logs[0]), len(want))
	}
}

func TestNewCHDMan_Defaults(t *testing.T) {
	c := tools.NewCHDMan("")
	if c == nil {
		t.Fatal("nil")
	}
	err := c.CreateCHD(context.Background(), "/no/such/file.cue", t.TempDir()+"/x.chd", &captureSinkCHDMan{})
	if err == nil {
		t.Errorf("want error from missing input")
	}
}

func TestCHDManName(t *testing.T) {
	c := tools.NewCHDMan("")
	if c.Name() != "chdman" {
		t.Errorf("Name = %q, want chdman", c.Name())
	}
}

func TestCHDMan_RejectsUnknownExtension(t *testing.T) {
	c := tools.NewCHDMan("")
	err := c.CreateCHD(context.Background(), "/tmp/foo.txt", "/tmp/x.chd", &captureSinkCHDMan{})
	if err == nil {
		t.Errorf("want error for non-cue/iso extension")
	}
}
