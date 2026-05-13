package pipelines

import (
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// SelectHandBrakeEncoder maps a profile's video_codec to the
// HandBrake --encoder flag. When the profile requests NVENC but the
// GPU isn't available, falls back to the closest software encoder
// and returns fellBack=true so the pipeline can log a WARN.
//
// Unknown codecs pass through unchanged — the user picked an exotic
// option and we trust them. Empty defaults to x264, matching the
// pre-NVENC behaviour of the DVD-Video pipeline.
func SelectHandBrakeEncoder(profile *state.Profile, gpuAvailable bool) (encoder string, fellBack bool) {
	requested := strings.ToLower(strings.TrimSpace(profile.VideoCodec))
	switch requested {
	case "nvenc_h264":
		if gpuAvailable {
			return "nvenc_h264", false
		}
		return "x264", true
	case "nvenc_h265":
		if gpuAvailable {
			return "nvenc_h265", false
		}
		return "x265", true
	case "", "x264":
		return "x264", false
	case "x265":
		return "x265", false
	default:
		return requested, false
	}
}
