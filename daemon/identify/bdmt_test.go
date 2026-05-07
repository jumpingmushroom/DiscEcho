package identify_test

import (
	"os"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestParseBDMT_Sample(t *testing.T) {
	b, err := os.ReadFile("testdata/bdmt-eng-sample.xml")
	if err != nil {
		t.Fatal(err)
	}
	got := identify.ParseBDMT(b)
	if got != "Arrival" {
		t.Errorf("want Arrival, got %q", got)
	}
}

func TestParseBDMT_Empty(t *testing.T) {
	if got := identify.ParseBDMT(nil); got != "" {
		t.Errorf("want empty, got %q", got)
	}
	if got := identify.ParseBDMT([]byte{}); got != "" {
		t.Errorf("want empty, got %q", got)
	}
}

func TestParseBDMT_Malformed(t *testing.T) {
	if got := identify.ParseBDMT([]byte("<not-xml")); got != "" {
		t.Errorf("want empty on malformed XML, got %q", got)
	}
}

func TestParseBDMT_NoTitle(t *testing.T) {
	xml := []byte(`<?xml version="1.0"?><disclib><di:discinfo xmlns:di="x"/></disclib>`)
	if got := identify.ParseBDMT(xml); got != "" {
		t.Errorf("want empty when no title, got %q", got)
	}
}
