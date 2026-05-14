package tools

import (
	"testing"
	"time"
)

// TestHandBrake_ScanArgs_RequestsAllTitles is a regression test for the
// 0.3.1 bug where every multi-title DVD (Jackass: The Movie, TV season
// discs, anything menu-driven) reported a single short preview title:
// HandBrakeCLI's --scan defaults to title_index=1 and only enumerates
// title 1 unless --title 0 is also passed.
func TestHandBrake_ScanArgs_RequestsAllTitles(t *testing.T) {
	args := scanArgs("/dev/sr0")

	var hasInput, hasTitle, hasScan bool
	for i, a := range args {
		switch a {
		case "--input":
			if i+1 >= len(args) || args[i+1] != "/dev/sr0" {
				t.Errorf("--input missing devPath; got args=%v", args)
			}
			hasInput = true
		case "--title":
			if i+1 >= len(args) || args[i+1] != "0" {
				t.Errorf("--title must be 0 to enumerate all titles; got args=%v", args)
			}
			hasTitle = true
		case "--scan":
			hasScan = true
		}
	}
	if !hasInput || !hasTitle || !hasScan {
		t.Errorf("scanArgs missing required flag; args=%v", args)
	}
}

func TestDeriveEncodeETA_WithDuration(t *testing.T) {
	// 7200s title, 60s elapsed for 0→50% → encoded 3600s in 60s wall
	// = 60x realtime; 3600s of source left → 60s ETA.
	speed, eta := deriveEncodeETA(0, 50, 60*time.Second, 7200)
	if speed != "60.0x" {
		t.Errorf("speed: want 60.0x, got %q", speed)
	}
	if eta != 60 {
		t.Errorf("eta: want 60, got %d", eta)
	}
}

func TestDeriveEncodeETA_AnchorsOnFirstEvent(t *testing.T) {
	// First event was already at 20%; 20→60% took 40s. Rate must be
	// computed from the 40-point delta, not from 60% / 40s.
	speed, eta := deriveEncodeETA(20, 60, 40*time.Second, 3600)
	// encodedDelta = 0.40 * 3600 = 1440s over 40s → 36x.
	if speed != "36.0x" {
		t.Errorf("speed: want 36.0x, got %q", speed)
	}
	// remaining = 0.40 * 3600 = 1440s at 36x → 40s.
	if eta != 40 {
		t.Errorf("eta: want 40, got %d", eta)
	}
}

func TestDeriveEncodeETA_NoDuration(t *testing.T) {
	// Without a duration reference: speed is unknowable, ETA is
	// extrapolated from the percentage rate. 0→10% in 10s → 1%/s →
	// 90% left → 90s.
	speed, eta := deriveEncodeETA(0, 10, 10*time.Second, 0)
	if speed != "" {
		t.Errorf("speed: want empty without duration, got %q", speed)
	}
	if eta != 90 {
		t.Errorf("eta: want 90, got %d", eta)
	}
}

func TestDeriveEncodeETA_GuardsAgainstNoProgress(t *testing.T) {
	cases := []struct {
		name       string
		first, cur float64
		elapsed    time.Duration
		duration   int
	}{
		{"no delta", 50, 50, 10 * time.Second, 7200},
		{"negative delta", 60, 50, 10 * time.Second, 7200},
		{"zero elapsed", 0, 50, 0, 7200},
		{"already complete", 0, 100, 10 * time.Second, 7200},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			speed, eta := deriveEncodeETA(c.first, c.cur, c.elapsed, c.duration)
			if speed != "" || eta != 0 {
				t.Errorf("want empty/0, got speed=%q eta=%d", speed, eta)
			}
		})
	}
}

func TestIsJSONNoise(t *testing.T) {
	noise := []string{
		`"PresetEncoder": "av_aac",`,
		`"Quality": -3.0,`,
		"{", "}", "[", "]", "},", "],",
	}
	for _, s := range noise {
		if !isJSONNoise(s) {
			t.Errorf("want %q classified as JSON noise", s)
		}
	}
	real := []string{
		"x264 [error]: nal write failed",
		"Encoding: task 1 of 1, 50.00 %",
		"[14:30:21] sync: reached audio",
		"HandBrake has exited.",
	}
	for _, s := range real {
		if isJSONNoise(s) {
			t.Errorf("want %q NOT classified as JSON noise", s)
		}
	}
}
