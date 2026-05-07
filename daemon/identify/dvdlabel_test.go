package identify_test

import (
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestNormaliseDVDLabel(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"underscore-separated movie", "BLADE_RUNNER_2049", "Blade Runner 2049"},
		{"dot-separated", "FRIENDS.S03.D2", "Friends S03 D2"},
		{"multiple spaces", "  multiple   spaces  ", "Multiple Spaces"},
		{"mixed case kept lowercase", "Some Movie", "Some Movie"},
		{"DVD_VIDEO is garbage", "DVD_VIDEO", ""},
		{"DVD VIDEO is garbage", "DVD VIDEO", ""},
		{"DVDVIDEO is garbage", "DVDVIDEO", ""},
		{"too short", "X", ""},
		{"3 chars too short", "FOO", ""},
		{"4 chars borderline ok", "ARGO", "Argo"},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := identify.NormaliseDVDLabel(tc.in)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
