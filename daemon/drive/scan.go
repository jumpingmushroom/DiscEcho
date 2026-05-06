//go:build linux

package drive

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// InitialScan enumerates /dev/sr* and upserts each drive into the
// store. Designed to be called once at startup; subsequent state is
// driven by udev events.
func InitialScan(ctx context.Context, store *state.Store) (int, error) {
	matches, err := filepath.Glob("/dev/sr*")
	if err != nil {
		return 0, fmt.Errorf("glob /dev/sr*: %w", err)
	}
	count := 0
	for _, dev := range matches {
		model, bus := probeDriveMetadata(dev)
		d := &state.Drive{
			DevPath:    dev,
			Model:      model,
			Bus:        bus,
			State:      state.DriveStateIdle,
			LastSeenAt: time.Now(),
		}
		if err := store.UpsertDrive(ctx, d); err != nil {
			return count, fmt.Errorf("upsert %s: %w", dev, err)
		}
		count++
	}
	return count, nil
}

// probeDriveMetadata reads model + vendor from sysfs. Failures are
// non-fatal — empty strings are returned.
func probeDriveMetadata(devPath string) (model, bus string) {
	name := filepath.Base(devPath)
	model = readSysAttr("/sys/block/" + name + "/device/model")
	vendor := readSysAttr("/sys/block/" + name + "/device/vendor")
	if vendor != "" && model != "" {
		model = vendor + " " + model
	} else if vendor != "" {
		model = vendor
	}
	// Real udev would expose bus topology; for M1.1 we record the
	// kernel device name as a placeholder.
	bus = name
	return
}

func readSysAttr(path string) string {
	b, err := os.ReadFile(path) // #nosec G304 -- sysfs path built from /dev/sr* glob
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
