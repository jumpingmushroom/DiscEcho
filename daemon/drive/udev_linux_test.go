//go:build linux

package drive

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestSuperviseWatch_RestartsAfterExit is the regression for the
// permanently-deaf-daemon bug: the go-udev netlink monitor stops on
// its first read error (e.g. ENOBUFS when a uevent burst overflows the
// socket buffer), so without supervision one transient failure left
// the daemon unable to see any further disc insertions. superviseWatch
// must reconnect after every non-shutdown exit.
func TestSuperviseWatch_RestartsAfterExit(t *testing.T) {
	var calls atomic.Int32
	watch := func(ctx context.Context) error {
		if calls.Add(1) < 3 {
			return errors.New("netlink monitor: boom")
		}
		<-ctx.Done() // 3rd session stays healthy until shutdown
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- superviseWatch(ctx, watch, time.Microsecond, time.Microsecond) }()

	deadline := time.Now().Add(2 * time.Second)
	for calls.Load() < 3 {
		if time.Now().After(deadline) {
			t.Fatalf("superviseWatch did not restart the watcher: calls=%d", calls.Load())
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("superviseWatch: want nil on ctx cancel, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("superviseWatch did not return after ctx cancel")
	}
}

// TestSuperviseWatch_StopsOnContextCancel verifies the supervisor loop
// exits promptly when ctx is cancelled and stops restarting the
// watcher rather than spinning forever.
func TestSuperviseWatch_StopsOnContextCancel(t *testing.T) {
	var calls atomic.Int32
	watch := func(_ context.Context) error {
		calls.Add(1)
		return errors.New("boom")
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- superviseWatch(ctx, watch, time.Millisecond, time.Millisecond) }()

	time.Sleep(12 * time.Millisecond) // let it restart a few times
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("want nil, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("superviseWatch did not stop after ctx cancel")
	}

	settled := calls.Load()
	time.Sleep(20 * time.Millisecond)
	if calls.Load() != settled {
		t.Errorf("superviseWatch kept calling watch after ctx cancel: %d -> %d", settled, calls.Load())
	}
}
