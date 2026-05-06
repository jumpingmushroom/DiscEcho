//go:build !linux

package drive

import (
	"context"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// InitialScan is a no-op on non-Linux platforms. The udev-driven flow
// is Linux-only; tests and dev-on-mac builds get an empty result.
func InitialScan(_ context.Context, _ *state.Store) (int, error) {
	return 0, nil
}
