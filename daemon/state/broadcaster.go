package state

import "sync"

// Event is a single pub/sub message. Name maps to the SSE event name;
// Payload is JSON-marshalled before being sent on the wire.
type Event struct {
	Name    string
	Payload any
}

// Broadcaster is an in-process pub/sub. Subscribers receive every event
// published after they subscribed. A subscriber whose buffer fills up
// is dropped (channel closed, removed from the set) so a stuck client
// never backs up the publisher.
type Broadcaster struct {
	mu     sync.Mutex
	subs   map[chan Event]struct{}
	closed bool
}

// NewBroadcaster returns an empty Broadcaster ready for Subscribe/Publish.
func NewBroadcaster() *Broadcaster {
	return &Broadcaster{subs: make(map[chan Event]struct{})}
}

// Subscribe returns a receive-only channel and a cancel function.
// `buffer` is the per-subscriber channel capacity; the broadcaster
// drops the subscriber if Publish would block on this channel. cancel
// is idempotent and safe to call from any goroutine.
func (b *Broadcaster) Subscribe(buffer int) (<-chan Event, func()) {
	if buffer < 1 {
		buffer = 1
	}
	ch := make(chan Event, buffer)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		close(ch)
		return ch, func() {}
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			if _, ok := b.subs[ch]; ok {
				delete(b.subs, ch)
				close(ch)
			}
		})
	}
	return ch, cancel
}

// Publish delivers ev to every subscriber. Non-blocking: if a
// subscriber's channel is full, that subscriber is removed.
func (b *Broadcaster) Publish(ev Event) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	var dropped []chan Event
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
			dropped = append(dropped, ch)
		}
	}
	for _, ch := range dropped {
		delete(b.subs, ch)
		close(ch)
	}
	b.mu.Unlock()
}

// Close removes every subscriber and rejects future Publish/Subscribe.
// Safe to call multiple times.
func (b *Broadcaster) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for ch := range b.subs {
		close(ch)
	}
	b.subs = nil
}
