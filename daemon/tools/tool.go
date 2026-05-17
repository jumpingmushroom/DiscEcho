// Package tools wraps the external binaries DiscEcho composes into
// pipelines: whipper, apprise, eject, and (later) MakeMKV / HandBrake.
// Each Tool exposes the same Run signature so the orchestrator can
// substitute mocks in tests.
package tools

import (
	"context"
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Tool wraps one external binary. Implementations parse stdout/stderr
// and emit structured progress through Sink.
type Tool interface {
	Name() string
	Run(ctx context.Context, args []string, env map[string]string,
		workdir string, sink Sink) error
}

// Sink receives progress + log events from a tool. Implementations:
//   - jobs.PersistentSink (production: writes to DB + Broadcaster)
//   - tools.NopSink       (callers that don't care)
//   - test recordingSinks (per-package test types)
type Sink interface {
	Progress(pct float64, speed string, etaSeconds int)
	Log(level state.LogLevel, format string, args ...any)
	// SubStep records a long-running sub-phase such as redumper's REFINE
	// or SPLIT phase. Empty name clears the current sub-step.
	SubStep(name string)
}

// NopSink discards everything.
type NopSink struct{}

func (NopSink) Progress(float64, string, int)      {}
func (NopSink) Log(state.LogLevel, string, ...any) {}
func (NopSink) SubStep(string)                     {}

// Registry maps tool names to implementations. Goroutine-safe.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds (or overwrites) the entry for t.Name(). Last writer wins.
func (r *Registry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get returns the tool registered under name and whether it was found.
func (r *Registry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}
