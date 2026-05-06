//go:build !linux

package drive

import (
	"context"
	"log/slog"
)

// Watch is a no-op on non-Linux platforms. The daemon is only supported
// on Linux at runtime; this stub exists so devs on macOS/Windows can
// build and run unit tests locally.
func Watch(ctx context.Context, _ func(Uevent)) error {
	slog.Warn("udev watcher disabled: non-linux build")
	<-ctx.Done()
	return nil
}
