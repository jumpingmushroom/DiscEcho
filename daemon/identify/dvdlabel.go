package identify

import (
	"regexp"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	dvdLabelWhitespace = regexp.MustCompile(`\s+`)
	dvdLabelGarbage    = regexp.MustCompile(`(?i)^DVD[\s_]*VIDEO$`)
	dvdTitleCaser      = cases.Title(language.Und)
)

// NormaliseDVDLabel takes a raw ISO9660 volume label and returns a
// search query, or "" if the label is too generic to bother searching.
//
// Rules:
//   - Replace '_' and '.' with spaces; collapse whitespace.
//   - Title-case the result (golang.org/x/text/cases handles Unicode
//     properly; stdlib's strings.Title is deprecated and broken on
//     non-ASCII).
//   - Reject results <= 3 chars or matching /^DVD[\s_]*VIDEO$/i.
func NormaliseDVDLabel(raw string) string {
	s := strings.ReplaceAll(raw, "_", " ")
	s = strings.ReplaceAll(s, ".", " ")
	s = dvdLabelWhitespace.ReplaceAllString(strings.TrimSpace(s), " ")
	if dvdLabelGarbage.MatchString(s) {
		return ""
	}
	s = dvdTitleCaser.String(strings.ToLower(s))
	if len(s) <= 3 {
		return ""
	}
	return s
}
