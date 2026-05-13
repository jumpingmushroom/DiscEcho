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
