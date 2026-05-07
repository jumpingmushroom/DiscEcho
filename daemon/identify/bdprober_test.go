package identify_test

import (
	"context"
	"os"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestParseBDInfo_BDMV(t *testing.T) {
	b, err := os.ReadFile("testdata/bdinfo-bdmv.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := identify.ParseBDInfoOutput(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.HasAACS2 {
		t.Errorf("BDMV: HasAACS2=true, want false")
	}
	if got.AACSEncrypted != true {
		t.Errorf("BDMV: AACSEncrypted=false, want true")
	}
}

func TestParseBDInfo_UHD(t *testing.T) {
	b, err := os.ReadFile("testdata/bdinfo-uhd.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := identify.ParseBDInfoOutput(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasAACS2 {
		t.Errorf("UHD: HasAACS2=false, want true")
	}
}

func TestParseBDInfo_Empty(t *testing.T) {
	if _, err := identify.ParseBDInfoOutput(""); err == nil {
		t.Errorf("want error on empty input")
	}
}

func TestNewBDProber_DefaultBin(t *testing.T) {
	p := identify.NewBDProber(identify.BDProberConfig{})
	if p == nil {
		t.Fatal("nil prober")
	}
	// Calling against /dev/null should fail cleanly (not panic).
	_, err := p.Probe(context.Background(), "/dev/null")
	if err == nil {
		t.Errorf("want error from /dev/null")
	}
}
