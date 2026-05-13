package pipelines_test

import (
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestSelectHandBrakeEncoder(t *testing.T) {
	cases := []struct {
		name         string
		videoCodec   string
		gpuAvailable bool
		wantEncoder  string
		wantFellBack bool
	}{
		{"nvenc_h265 with GPU", "nvenc_h265", true, "nvenc_h265", false},
		{"nvenc_h265 no GPU falls back to x265", "nvenc_h265", false, "x265", true},
		{"nvenc_h264 with GPU", "nvenc_h264", true, "nvenc_h264", false},
		{"nvenc_h264 no GPU falls back to x264", "nvenc_h264", false, "x264", true},
		{"x264 explicit", "x264", true, "x264", false},
		{"x264 no GPU still x264", "x264", false, "x264", false},
		{"x265 explicit", "x265", true, "x265", false},
		{"empty defaults to x264", "", true, "x264", false},
		{"unknown codec passes through", "svt_av1", true, "svt_av1", false},
		{"uppercase NVENC_H265 normalized", "NVENC_H265", true, "nvenc_h265", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prof := &state.Profile{VideoCodec: tc.videoCodec}
			gotEnc, gotFell := pipelines.SelectHandBrakeEncoder(prof, tc.gpuAvailable)
			if gotEnc != tc.wantEncoder {
				t.Errorf("encoder: got %q, want %q", gotEnc, tc.wantEncoder)
			}
			if gotFell != tc.wantFellBack {
				t.Errorf("fellBack: got %v, want %v", gotFell, tc.wantFellBack)
			}
		})
	}
}
