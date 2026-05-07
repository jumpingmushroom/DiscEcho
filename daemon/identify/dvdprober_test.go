package identify_test

import (
	"context"
	"os"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestParseIsoInfo(t *testing.T) {
	out, err := os.ReadFile("testdata/isoinfo-arrival.txt")
	if err != nil {
		t.Fatal(err)
	}
	info, err := identify.ParseIsoInfoOutput(string(out))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.VolumeLabel != "ARRIVAL" {
		t.Errorf("volume label: got %q", info.VolumeLabel)
	}
}

func TestParseIsoInfo_Blank(t *testing.T) {
	out, err := os.ReadFile("testdata/isoinfo-blank.txt")
	if err != nil {
		t.Fatal(err)
	}
	info, err := identify.ParseIsoInfoOutput(string(out))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.VolumeLabel != "" {
		t.Errorf("blank disc label: got %q", info.VolumeLabel)
	}
}

func TestParseIsoInfo_NoVolumeIdLine(t *testing.T) {
	_, err := identify.ParseIsoInfoOutput("garbage output\nwith no useful fields\n")
	if err == nil {
		t.Errorf("want error when Volume id: line missing")
	}
}

func TestDVDProber_ExecFailureSurfaces(t *testing.T) {
	p := identify.NewDVDProber(identify.DVDProberConfig{IsoInfoBin: "/usr/bin/false"})
	_, err := p.Probe(context.Background(), "/dev/null")
	if err == nil {
		t.Errorf("want error from /usr/bin/false")
	}
}
