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
