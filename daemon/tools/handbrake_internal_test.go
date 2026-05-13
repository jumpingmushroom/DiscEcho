package tools

import "testing"

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
