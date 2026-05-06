package identify_test

import (
	"context"
	"os"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestParseCDParanoiaQ_KindOfBlue(t *testing.T) {
	out, err := os.ReadFile("testdata/cdparanoia-Q-kindofblue.txt")
	if err != nil {
		t.Fatal(err)
	}
	toc, err := identify.ParseCDParanoiaQ(string(out))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(toc.Tracks) != 5 {
		t.Fatalf("want 5 tracks, got %d", len(toc.Tracks))
	}
	if toc.FirstTrack() != 1 || toc.LastTrack() != 5 {
		t.Errorf("first/last: %d/%d", toc.FirstTrack(), toc.LastTrack())
	}
	t1 := toc.Tracks[0]
	if t1.Number != 1 || t1.StartLBA != 150 || t1.LengthLBA != 14955 || !t1.IsAudio {
		t.Errorf("track 1 mismatch: %+v", t1)
	}
	t5 := toc.Tracks[4]
	wantLeadout := 76815 + 44190
	if toc.LeadoutLBA != wantLeadout {
		t.Errorf("leadout: want %d, got %d", wantLeadout, toc.LeadoutLBA)
	}
	if t5.Number != 5 || t5.StartLBA != 76815 {
		t.Errorf("track 5 mismatch: %+v", t5)
	}
}

func TestParseCDParanoiaQ_OKComputer(t *testing.T) {
	out, err := os.ReadFile("testdata/cdparanoia-Q-okcomputer.txt")
	if err != nil {
		t.Fatal(err)
	}
	toc, err := identify.ParseCDParanoiaQ(string(out))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(toc.Tracks) != 12 {
		t.Errorf("want 12 tracks, got %d", len(toc.Tracks))
	}
	if toc.FirstTrack() != 1 || toc.LastTrack() != 12 {
		t.Errorf("first/last: %d/%d", toc.FirstTrack(), toc.LastTrack())
	}
}

func TestParseCDParanoiaQ_NoTracks(t *testing.T) {
	bad := "cdparanoia III\n\nNo tracks found\n"
	_, err := identify.ParseCDParanoiaQ(bad)
	if err == nil {
		t.Errorf("want error for empty TOC")
	}
}

func TestTOCReader_Interface(t *testing.T) {
	// NewCDParanoiaTOCReader already returns TOCReader, so this is just
	// a smoke check that construction with both an explicit binary path
	// and the empty default works.
	if r := identify.NewCDParanoiaTOCReader("/usr/bin/cdparanoia"); r == nil {
		t.Fatal("nil reader from explicit bin")
	}
	if r := identify.NewCDParanoiaTOCReader(""); r == nil {
		t.Fatal("nil reader from default bin")
	}
	_ = context.Background()
}
