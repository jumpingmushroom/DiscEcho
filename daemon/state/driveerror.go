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
		// cd-info's exit-1 can come from a dirty/scratched surface, a
		// disc the drive can't physically read (XGD originals need
		// Kreon firmware), or a transient spin-up race that exhausted
		// the classifier's retry budget. The tip lists the common
		// causes in rough likelihood order rather than naming Xbox
		// outright — most "cd-info exit 1" failures we see are dirty
		// discs, not XGDs.
		return "The drive couldn't read this disc. Common causes: dirty or scratched surface (clean and re-insert), the drive is spinning the disc down before cd-info finishes (eject and re-insert), or this is an original Xbox game disc that requires Kreon firmware (https://kreon.dev)."
	case strings.Contains(lower, "deadline exceeded"):
		return "The drive is reading this disc very slowly — the identify step timed out before cd-info or isoinfo could finish. Try ejecting and re-inserting (often the second spin-up is faster), clean the disc surface, or try a different drive."
	}
	return ""
}
