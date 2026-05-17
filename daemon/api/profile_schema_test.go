package api_test

import (
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func validProfile() *state.Profile {
	return &state.Profile{
		Name:               "Test",
		DiscType:           state.DiscTypeAudioCD,
		Engine:             "whipper",
		Format:             "FLAC",
		Preset:             "AccurateRip",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}}.flac`,
	}
}

func TestValidateProfile_Valid(t *testing.T) {
	if errs := api.ValidateProfile(validProfile()); len(errs) != 0 {
		t.Errorf("want no errors, got %+v", errs)
	}
}

func TestValidateProfile_UnknownEngine(t *testing.T) {
	p := validProfile()
	p.Engine = "ffmpeg"
	errs := api.ValidateProfile(p)
	if len(errs) == 0 {
		t.Fatal("want errors")
	}
	if errs[0].Field != "engine" {
		t.Errorf("first error field = %s, want engine", errs[0].Field)
	}
	if !strings.Contains(errs[0].Msg, "unknown engine") {
		t.Errorf("msg = %q, want 'unknown engine' substring", errs[0].Msg)
	}
}

func TestValidateProfile_BadFormatForEngine(t *testing.T) {
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = "MP3"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "format" {
			found = true
		}
	}
	if !found {
		t.Errorf("want format error, got %+v", errs)
	}
}

func TestValidateProfile_UnknownOption(t *testing.T) {
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = "MP4"
	p.Options = map[string]any{"bitrate": 8000}
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "options.bitrate" {
			found = true
		}
	}
	if !found {
		t.Errorf("want options.bitrate error, got %+v", errs)
	}
}

func TestValidateProfile_WrongOptionType(t *testing.T) {
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = "MP4"
	p.Options = map[string]any{"min_title_seconds": "not-a-number"}
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "options.min_title_seconds" && strings.Contains(e.Msg, "int") {
			found = true
		}
	}
	if !found {
		t.Errorf("want options.min_title_seconds expected-int error, got %+v", errs)
	}
}

func TestValidateProfile_BadTemplate(t *testing.T) {
	p := validProfile()
	p.OutputPathTemplate = "{{.Title"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "output_path_template" {
			found = true
		}
	}
	if !found {
		t.Errorf("want output_path_template error, got %+v", errs)
	}
}

func TestValidateProfile_AcceptsFloat64ForInt(t *testing.T) {
	// JSON-decoded numbers come in as float64. min_title_seconds is
	// declared as OptInt; the validator must accept whole-number
	// float64 values.
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = "MP4"
	p.Options = map[string]any{"min_title_seconds": float64(3600)}
	errs := api.ValidateProfile(p)
	for _, e := range errs {
		if e.Field == "options.min_title_seconds" {
			t.Errorf("float64 should be accepted for int, got %+v", errs)
		}
	}
}

func TestValidateProfile_RedumperEngine(t *testing.T) {
	p := &state.Profile{
		DiscType:           state.DiscTypeXBOX,
		Name:               "X",
		Engine:             "redumper",
		Format:             "ISO",
		Options:            map[string]any{},
		OutputPathTemplate: "{{.Title}}.iso",
		StepCount:          5,
	}
	if errs := api.ValidateProfile(p); len(errs) != 0 {
		t.Fatalf("expected valid; got %v", errs)
	}
}

func TestValidateProfile_RedumperRejectsBadFormat(t *testing.T) {
	p := &state.Profile{
		DiscType:           state.DiscTypeXBOX,
		Name:               "X",
		Engine:             "redumper",
		Format:             "MKV",
		Options:            map[string]any{},
		OutputPathTemplate: "{{.Title}}.mkv",
		StepCount:          5,
	}
	if errs := api.ValidateProfile(p); len(errs) == 0 {
		t.Fatal("expected validation errors")
	}
}

func TestValidateProfile_DDRescueEngine(t *testing.T) {
	p := &state.Profile{
		DiscType:           state.DiscTypeData,
		Name:               "D",
		Engine:             "ddrescue",
		Format:             "ISO",
		Options:            map[string]any{},
		OutputPathTemplate: "{{.Title}}.iso",
		StepCount:          6,
	}
	if errs := api.ValidateProfile(p); len(errs) != 0 {
		t.Fatalf("expected valid; got %v", errs)
	}
}

func TestValidateProfile_TypedContainerWins(t *testing.T) {
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = ""
	p.Container = "MP4"
	p.VideoCodec = "x264"
	p.HDRPipeline = "passthrough"
	if errs := api.ValidateProfile(p); len(errs) != 0 {
		t.Fatalf("expected valid; got %v", errs)
	}
}

func TestValidateProfile_BadContainerForEngine(t *testing.T) {
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = ""
	p.Container = "FLAC"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "container" {
			found = true
		}
	}
	if !found {
		t.Errorf("want container error, got %+v", errs)
	}
}

func TestValidateProfile_BadVideoCodecForEngine(t *testing.T) {
	p := validProfile()
	p.Engine = "HandBrake"
	p.Format = ""
	p.Container = "MP4"
	p.VideoCodec = "vp9"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "video_codec" {
			found = true
		}
	}
	if !found {
		t.Errorf("want video_codec error, got %+v", errs)
	}
}

func TestValidateProfile_VideoCodecRejectedForAudioEngine(t *testing.T) {
	p := validProfile()
	p.VideoCodec = "x265"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "video_codec" {
			found = true
		}
	}
	if !found {
		t.Errorf("want video_codec error for whipper, got %+v", errs)
	}
}

func TestValidateProfile_BadHDRPipeline(t *testing.T) {
	p := validProfile()
	p.HDRPipeline = "wat"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "hdr_pipeline" {
			found = true
		}
	}
	if !found {
		t.Errorf("want hdr_pipeline error, got %+v", errs)
	}
}

func TestValidateProfile_BadDrivePolicy(t *testing.T) {
	p := validProfile()
	p.DrivePolicy = "drv-99"
	errs := api.ValidateProfile(p)
	var found bool
	for _, e := range errs {
		if e.Field == "drive_policy" {
			found = true
		}
	}
	if !found {
		t.Errorf("want drive_policy error, got %+v", errs)
	}
}

// TestValidateProfile_SeededDVDProfileOptions guards against the
// seeder and the validation schema drifting apart. The DVD-Movie and
// DVD-Series profiles are seeded (settings.go) and migrated
// (003_dvd_default_mkv.sql) with a dvd_selection_mode option; if the
// HandBrake engine schema doesn't list it, every UI save of a DVD
// profile 422s.
func TestValidateProfile_SeededDVDProfileOptions(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
	}{
		{"DVD-Movie", map[string]any{"dvd_selection_mode": "main_feature"}},
		{"DVD-Series", map[string]any{
			"min_title_seconds":  float64(300),
			"season":             float64(1),
			"dvd_selection_mode": "per_title",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &state.Profile{
				Name:               tc.name,
				DiscType:           state.DiscTypeDVD,
				Engine:             "HandBrake",
				Container:          "MKV",
				VideoCodec:         "x264",
				Options:            tc.opts,
				OutputPathTemplate: `{{.Title}}.mkv`,
				StepCount:          7,
			}
			if errs := api.ValidateProfile(p); len(errs) != 0 {
				t.Errorf("seeded %s profile must validate; got %+v", tc.name, errs)
			}
		})
	}
}

func TestValidateProfile_DDRescueRejectsOptions(t *testing.T) {
	p := &state.Profile{
		DiscType:           state.DiscTypeData,
		Name:               "D",
		Engine:             "ddrescue",
		Format:             "ISO",
		Options:            map[string]any{"foo": "bar"},
		OutputPathTemplate: "{{.Title}}.iso",
		StepCount:          6,
	}
	if errs := api.ValidateProfile(p); len(errs) == 0 {
		t.Fatal("expected validation errors for unknown option")
	}
}
