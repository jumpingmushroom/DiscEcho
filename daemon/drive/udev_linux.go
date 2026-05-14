//go:build linux

package drive

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/pilebones/go-udev/netlink"
)

// Watch subscribes to kernel uevents and invokes onMediaChange for every
// optical-media-change event until ctx is cancelled. It supervises the
// underlying netlink session: the go-udev monitor stops on its first
// read error (e.g. ENOBUFS when a uevent burst overflows the socket
// buffer), so Watch reconnects after any non-shutdown exit — without
// that, one transient failure left the daemon permanently deaf to disc
// insertions. Returns nil on clean shutdown.
func Watch(ctx context.Context, onMediaChange func(Uevent)) error {
	return superviseWatch(ctx, func(c context.Context) error {
		return watchOnce(c, onMediaChange)
	}, time.Second, 30*time.Second)
}

// superviseWatch runs watch in a loop, reconnecting after every exit
// that isn't a clean ctx cancellation. Backoff between restarts grows
// exponentially from minBackoff to maxBackoff so a persistently broken
// netlink socket doesn't become a busy-loop. Returns nil once ctx is
// done.
func superviseWatch(ctx context.Context, watch func(context.Context) error, minBackoff, maxBackoff time.Duration) error {
	backoff := minBackoff
	for {
		err := watch(ctx)
		if ctx.Err() != nil {
			return nil
		}
		slog.Warn("udev watcher exited; reconnecting", "err", err, "backoff", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return nil
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// watchOnce runs a single netlink monitor session: connect, stream
// uevents, and return on the first monitor error or ctx cancellation.
// The go-udev monitor goroutine stops on its first read error, so a
// session can't be resumed in place — superviseWatch reconnects.
func watchOnce(ctx context.Context, onMediaChange func(Uevent)) error {
	conn := new(netlink.UEventConn)
	if err := conn.Connect(netlink.UdevEvent); err != nil {
		return fmt.Errorf("netlink connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	queue := make(chan netlink.UEvent, 16)
	errs := make(chan error, 1)
	stop := conn.Monitor(queue, errs, nil)
	defer close(stop)

	slog.Info("udev watcher started")
	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-errs:
			return fmt.Errorf("netlink monitor: %w", err)
		case raw := <-queue:
			ev := translate(raw)
			if !ev.IsOpticalMediaChange() {
				continue
			}
			// Dispatch async: classify+identify (handle) can run for
			// many seconds — cd-info alone retries for up to ~13s.
			// Running it inline stalls this read loop, the netlink
			// socket buffer overflows, and the monitor dies with
			// ENOBUFS. handle is concurrency-safe; ClaimDriveForIdentify
			// dedups the uevent burst a single insertion produces.
			go onMediaChange(ev)
		}
	}
}

func translate(raw netlink.UEvent) Uevent {
	ev := Uevent{
		Action:     string(raw.Action),
		Properties: make(map[string]string, len(raw.Env)),
	}
	for k, v := range raw.Env {
		ev.Properties[k] = v
		switch k {
		case "DEVPATH":
			ev.DevPath = v
		case "SUBSYSTEM":
			ev.Subsystem = v
		case "DEVNAME":
			ev.DevName = strings.TrimPrefix(v, "/dev/")
		case "DEVTYPE":
			ev.DevType = v
		}
	}
	return ev
}
