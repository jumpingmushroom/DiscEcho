//go:build !linux

package drive_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/drive"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestInitialScan_NonLinux_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	n, err := drive.InitialScan(context.Background(), state.NewStore(db))
	if err != nil {
		t.Fatalf("InitialScan: %v", err)
	}
	if n != 0 {
		t.Errorf("InitialScan returned %d, want 0", n)
	}
}
