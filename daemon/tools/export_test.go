package tools

import "time"

// SetWhipperClockForTest swaps the whipper parser's clock with the
// provided function and returns a restore closure. Tests use this to
// produce deterministic ETA values without sleeping.
func SetWhipperClockForTest(now func() time.Time) (restore func()) {
	prev := whipperNow
	whipperNow = now
	return func() { whipperNow = prev }
}

// RedumperNowForTest / SetRedumperNowForTest swap the redumper parser's
// clock function. Same shape as the whipper variant; lets the derived
// speed + ETA tests use a deterministic clock without sleeping.
func RedumperNowForTest() func() time.Time { return redumperNow }
func SetRedumperNowForTest(now func() time.Time) {
	redumperNow = now
}
