package pipelines

import (
	"fmt"
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Registry maps disc types to handlers. Panics on duplicate
// registration: a daemon should never accidentally have two handlers
// for one disc type.
type Registry struct {
	mu       sync.RWMutex
	handlers map[state.DiscType]Handler
}

// NewRegistry returns an empty Registry.
func NewRegistry() *Registry {
	return &Registry{handlers: make(map[state.DiscType]Handler)}
}

// Register adds h. Panics if h.DiscType() is already registered.
func (r *Registry) Register(h Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	dt := h.DiscType()
	if _, exists := r.handlers[dt]; exists {
		panic(fmt.Sprintf("pipelines: duplicate handler for %s", dt))
	}
	r.handlers[dt] = h
}

// Get returns the handler for dt and whether one is registered.
func (r *Registry) Get(dt state.DiscType) (Handler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h, ok := r.handlers[dt]
	return h, ok
}
