package identify_test

import (
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

// Known vector taken verbatim from the MusicBrainz XML Web Service
// docs (https://musicbrainz.org/doc/Development/XML_Web_Service/Version_2):
//
//	/ws/2/discid/I5l9cCSFccLKFEKS.7wqSZAorPU-?toc=1+12+267257+150+22767+
//	  41887+58317+72102+91375+104652+115380+132165+143932+159870+174597
//
// → first=1, last=12, leadout=267257, then 12 track LBAs.
func TestDiscID_KnownVector(t *testing.T) {
	got := identify.DiscID(1, 12, 267257, []int{
		150, 22767, 41887, 58317, 72102, 91375,
		104652, 115380, 132165, 143932, 159870, 174597,
	})
	want := "I5l9cCSFccLKFEKS.7wqSZAorPU-"
	if got != want {
		t.Errorf("disc id mismatch:\nwant %s\ngot  %s", want, got)
	}
}

func TestDiscID_ShortTOC(t *testing.T) {
	got := identify.DiscID(1, 3, 200000, []int{150, 60000, 130000})
	if len(got) != 28 {
		t.Errorf("disc id should be 28 chars, got %q (len %d)", got, len(got))
	}
	if got == "" {
		t.Errorf("disc id empty")
	}
}

func TestDiscID_DeterministicSameInput(t *testing.T) {
	a := identify.DiscID(1, 3, 200000, []int{150, 60000, 130000})
	b := identify.DiscID(1, 3, 200000, []int{150, 60000, 130000})
	if a != b {
		t.Errorf("non-deterministic: %s vs %s", a, b)
	}
}
