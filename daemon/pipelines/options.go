package pipelines

import "github.com/jumpingmushroom/DiscEcho/daemon/state"

// IntOption reads an integer-valued key from a profile's Options blob,
// returning def when the key is absent or not a whole number. JSON
// decoding turns numbers into float64, so both int and whole-valued
// float64 are accepted — callers don't have to care which shape the
// value arrived in.
func IntOption(prof *state.Profile, key string, def int) int {
	if prof == nil {
		return def
	}
	switch n := prof.Options[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return def
}

// StringOption reads a string-valued key from a profile's Options
// blob, returning def when the key is absent, empty, or not a string.
func StringOption(prof *state.Profile, key, def string) string {
	if prof == nil {
		return def
	}
	if s, ok := prof.Options[key].(string); ok && s != "" {
		return s
	}
	return def
}
