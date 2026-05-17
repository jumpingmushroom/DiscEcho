package state

import "strings"

// DriveErrorTip returns a human-readable tip explaining a known
// drive-error pattern, or the empty string when no tip applies.
// Surfaces in the UI under the raw error message.
func DriveErrorTip(errMsg string) string {
	if errMsg == "" {
		return ""
	}
	lower := strings.ToLower(errMsg)
	switch {
	case strings.Contains(lower, "cd-info") && strings.Contains(lower, "exit status"):
		return "Xbox game discs require a drive with Kreon firmware to read past the outer \"decoy\" partition; standard DVD drives can only see a small UDF stub and cd-info errors out. See https://kreon.dev for compatibility."
	}
	return ""
}
