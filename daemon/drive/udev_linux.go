//go:build linux

package drive

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/pilebones/go-udev/netlink"
)

// Watch subscribes to kernel uevents and invokes onMediaChange for every
// optical-media-change event until ctx is cancelled. Other events are
// ignored. Returns the first fatal error from the netlink reader.
func Watch(ctx context.Context, onMediaChange func(Uevent)) error {
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
			onMediaChange(ev)
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
			ev.DevName = v
		case "DEVTYPE":
			ev.DevType = v
		}
	}
	return ev
}
