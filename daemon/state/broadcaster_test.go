package state_test

import (
	"sync"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestBroadcaster_FanOut(t *testing.T) {
	b := state.NewBroadcaster()
	defer b.Close()

	var wg sync.WaitGroup
	const nSubscribers = 3
	got := make([][]state.Event, nSubscribers)

	for i := 0; i < nSubscribers; i++ {
		i := i
		ch, cancel := b.Subscribe(8)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()
			for ev := range ch {
				got[i] = append(got[i], ev)
				if len(got[i]) == 2 {
					return
				}
			}
		}()
	}

	// Tiny delay so subscribers are all listening before publish
	time.Sleep(10 * time.Millisecond)

	b.Publish(state.Event{Name: "drive.changed", Payload: map[string]any{"a": 1}})
	b.Publish(state.Event{Name: "job.progress", Payload: map[string]any{"pct": 42}})

	wg.Wait()
	for i := 0; i < nSubscribers; i++ {
		if len(got[i]) != 2 {
			t.Errorf("subscriber %d: want 2 events, got %d", i, len(got[i]))
		}
	}
}

func TestBroadcaster_DropsSlowSubscriber(t *testing.T) {
	b := state.NewBroadcaster()
	defer b.Close()

	// Slow subscriber: buffer 1, never read
	slowCh, _ := b.Subscribe(1)

	// Fast subscriber: buffer 16, drained
	fastCh, fastCancel := b.Subscribe(16)
	defer fastCancel()

	for i := 0; i < 10; i++ {
		b.Publish(state.Event{Name: "ping"})
	}

	// Drain fast subscriber until done or timeout
	got := 0
	timeout := time.After(200 * time.Millisecond)
loop:
	for {
		select {
		case _, ok := <-fastCh:
			if !ok {
				break loop
			}
			got++
			if got == 10 {
				break loop
			}
		case <-timeout:
			break loop
		}
	}
	if got != 10 {
		t.Errorf("fast subscriber received %d/10 events", got)
	}

	// Slow channel should be closed (broadcaster gave up on it)
	closed := false
	deadline := time.After(100 * time.Millisecond)
slowloop:
	for {
		select {
		case _, ok := <-slowCh:
			if !ok {
				closed = true
				break slowloop
			}
			// got an earlier buffered event; keep reading
		case <-deadline:
			break slowloop
		}
	}
	if !closed {
		t.Errorf("slow subscriber channel was not closed after backpressure")
	}
}

func TestBroadcaster_Cancel(t *testing.T) {
	b := state.NewBroadcaster()
	defer b.Close()

	ch, cancel := b.Subscribe(4)
	cancel()

	// Publishing after cancel must not block or panic
	done := make(chan struct{})
	go func() {
		b.Publish(state.Event{Name: "test"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Errorf("publish blocked after cancel")
	}

	// Channel is closed
	if _, ok := <-ch; ok {
		t.Errorf("channel not closed after cancel")
	}
}

func TestBroadcaster_CancelIdempotent(t *testing.T) {
	b := state.NewBroadcaster()
	defer b.Close()

	_, cancel := b.Subscribe(4)
	cancel()
	cancel() // must not panic
}

func TestBroadcaster_AfterClose(t *testing.T) {
	b := state.NewBroadcaster()
	b.Close()

	// Subscribe after close returns a closed channel
	ch, _ := b.Subscribe(4)
	if _, ok := <-ch; ok {
		t.Errorf("Subscribe after Close should return a closed channel")
	}

	// Publish after close is a no-op (no panic)
	b.Publish(state.Event{Name: "noop"})

	// Close after Close is a no-op (no panic)
	b.Close()
}
