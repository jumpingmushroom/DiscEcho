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

type captureSink struct {
	progress []float64
	logs     []string
}

func (c *captureSink) Progress(pct float64, _ string, _ int) {
	c.progress = append(c.progress, pct)
}
func (c *captureSink) Log(_ state.LogLevel, format string, args ...any) {
	c.logs = append(c.logs, fmt.Sprintf(format, args...))
}

func TestMakeMKVParseInfo_BDMV(t *testing.T) {
	b, err := os.ReadFile("testdata/makemkv-info-bdmv.txt")
	if err != nil {
		t.Fatal(err)
	}
	titles, err := tools.ParseMakeMKVInfo(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 {
		t.Fatalf("want 2 titles, got %d", len(titles))
	}
	feat := titles[1]
	if feat.ID != 1 {
		t.Errorf("feature title id = %d, want 1", feat.ID)
	}
	if feat.DurationSec != 7002 { // 1:56:42
		t.Errorf("feature duration = %d, want 7002", feat.DurationSec)
	}
	if feat.SourceFile != "00800.mpls" {
		t.Errorf("source file = %q, want 00800.mpls", feat.SourceFile)
	}
	if len(feat.Tracks) < 3 {
		t.Errorf("expected >=3 tracks, got %d", len(feat.Tracks))
	}
	var sawAudio bool
	for _, tr := range feat.Tracks {
		if tr.Type == "Audio" && tr.Lang == "eng" && strings.HasPrefix(tr.Codec, "A_") {
			sawAudio = true
		}
	}
	if !sawAudio {
		t.Errorf("missing eng audio track in %v", feat.Tracks)
	}
}

func TestMakeMKVParseInfo_UHD(t *testing.T) {
	b, err := os.ReadFile("testdata/makemkv-info-uhd.txt")
	if err != nil {
		t.Fatal(err)
	}
	titles, err := tools.ParseMakeMKVInfo(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 1 {
		t.Fatalf("want 1 title, got %d", len(titles))
	}
	feat := titles[0]
	if feat.DurationSec != 9968 { // 2:46:08
		t.Errorf("UHD duration = %d, want 9968", feat.DurationSec)
	}
	var sawHEVC, sawAtmos bool
	for _, tr := range feat.Tracks {
		if tr.Codec == "V_MPEGH/ISO/HEVC" {
			sawHEVC = true
		}
		if strings.Contains(tr.Codec, "ATMOS") {
			sawAtmos = true
		}
	}
	if !sawHEVC {
		t.Errorf("missing HEVC stream in %v", feat.Tracks)
	}
	if !sawAtmos {
		t.Errorf("missing Atmos stream in %v", feat.Tracks)
	}
}

func TestMakeMKVParseInfo_Empty(t *testing.T) {
	if _, err := tools.ParseMakeMKVInfo(""); err == nil {
		t.Errorf("want error on empty input")
	}
}

func TestMakeMKVProgressStream_PRGV(t *testing.T) {
	sink := &captureSink{}
	in := bytes.NewBufferString(strings.Join([]string{
		`PRGV:0,1024,65536`,
		`PRGV:32768,1024,65536`,
		`PRGV:65536,1024,65536`,
	}, "\n"))
	tools.ParseMakeMKVProgressStream(in, sink)
	if len(sink.progress) != 3 {
		t.Fatalf("want 3 progress updates, got %d", len(sink.progress))
	}
	if sink.progress[0] != 0 {
		t.Errorf("first progress = %f, want 0", sink.progress[0])
	}
	if sink.progress[2] != 100 {
		t.Errorf("last progress = %f, want 100", sink.progress[2])
	}
}

func TestMakeMKVProgressStream_LogsPRGCAsLog(t *testing.T) {
	sink := &captureSink{}
	in := bytes.NewBufferString(`PRGC:5018,0,"Saving to MKV file"` + "\n")
	tools.ParseMakeMKVProgressStream(in, sink)
	if len(sink.logs) != 1 {
		t.Fatalf("want 1 log, got %d", len(sink.logs))
	}
	if !strings.Contains(sink.logs[0], "Saving to MKV file") {
		t.Errorf("log missing operation label, got %q", sink.logs[0])
	}
}

func TestNewMakeMKV_Defaults(t *testing.T) {
	m := tools.NewMakeMKV("", "")
	if m == nil {
		t.Fatal("nil MakeMKV")
	}
	// Calling Scan against a missing device should error cleanly.
	_, err := m.Scan(context.Background(), "/dev/null")
	if err == nil {
		t.Errorf("want error from /dev/null")
	}
}
