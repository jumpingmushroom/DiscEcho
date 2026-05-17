package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// captureSinkDVDBackup records sink calls for the parseDVDBackupStream
// internal tests. Internal package so it can reach the unexported
// parser directly — no need to export the parser just for tests.
type captureSinkDVDBackup struct {
	events []capturedEvent
}

type capturedEvent struct {
	kind    string
	level   state.LogLevel
	message string
}

func (s *captureSinkDVDBackup) Progress(_ float64, _ string, _ int) {
	s.events = append(s.events, capturedEvent{kind: "progress"})
}

func (s *captureSinkDVDBackup) Log(level state.LogLevel, format string, args ...any) {
	s.events = append(s.events, capturedEvent{
		kind: "log", level: level, message: fmt.Sprintf(format, args...),
	})
}

func (s *captureSinkDVDBackup) SubStep(string) {}

func TestDVDBackup_ParseStream_ForwardsVOBAsInfo(t *testing.T) {
	stream := strings.NewReader(
		"Copying VTS_01_1.VOB\n" +
			"Copying VTS_02_1.VOB\n",
	)
	sink := &captureSinkDVDBackup{}
	parseDVDBackupStream(stream, sink)

	infos := 0
	for _, e := range sink.events {
		if e.kind == "log" && e.level == state.LogLevelInfo {
			infos++
		}
	}
	if infos != 2 {
		t.Errorf("want 2 info VOB lines, got %d", infos)
	}
}

func TestDVDBackup_ParseStream_ForwardsErrorsAsWarn(t *testing.T) {
	stream := strings.NewReader(
		"libdvdread: Couldn't find device name.\n" +
			"libdvdnav: error mounting\n",
	)
	sink := &captureSinkDVDBackup{}
	parseDVDBackupStream(stream, sink)

	warns := 0
	for _, e := range sink.events {
		if e.kind == "log" && e.level == state.LogLevelWarn {
			warns++
		}
	}
	if warns != 2 {
		t.Errorf("want 2 warn lines, got %d", warns)
	}
}

func TestDVDBackup_ParseStream_DropsProgress(t *testing.T) {
	// dvdbackup's -p output overstrikes the terminal with '\r'; a whole
	// run of progress chunks arrives as one '\n'-terminated blob.
	stream := strings.NewReader(
		"Copying menu: 9% done (1/11 MiB)\r" +
			"Copying menu: 100% done (11/11 MiB)\r\n" +
			"Copying Title, part 1/1: 2% done (1/47 MiB)\r" +
			"Copying Title, part 1/1: 100% done (47/47 MiB)\n",
	)
	sink := &captureSinkDVDBackup{}
	parseDVDBackupStream(stream, sink)

	for _, e := range sink.events {
		if e.kind == "log" {
			t.Errorf("progress line should not be logged, got: %q", e.message)
		}
	}
}

func TestDVDBackup_ParseStream_DropsLibdvdreadTrace(t *testing.T) {
	stream := strings.NewReader(
		"libdvdread: Get key for /VIDEO_TS/VTS_02_1.VOB at 0x00021926\n" +
			"libdvdread: Elapsed time 0\n" +
			"libdvdread: Found 3 VTS's\n",
	)
	sink := &captureSinkDVDBackup{}
	parseDVDBackupStream(stream, sink)

	for _, e := range sink.events {
		if e.kind == "log" {
			t.Errorf("libdvdread trace should not be logged, got: %q", e.message)
		}
	}
}

func TestDVDBackup_ParseStream_KeepsLibdvdreadErrors(t *testing.T) {
	stream := strings.NewReader("libdvdread: Cannot open /dev/sr0\n")
	sink := &captureSinkDVDBackup{}
	parseDVDBackupStream(stream, sink)

	warns := 0
	for _, e := range sink.events {
		if e.kind == "log" && e.level == state.LogLevelWarn {
			warns++
		}
	}
	if warns != 1 {
		t.Errorf("want 1 warn line for a libdvdread error, got %d", warns)
	}
}

func TestDVDBackup_ParseStream_Cap(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&sb, "libdvd noise line %d\n", i)
	}
	sink := &captureSinkDVDBackup{}
	parseDVDBackupStream(strings.NewReader(sb.String()), sink)

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
