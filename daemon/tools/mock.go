package tools

import (
	"context"
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// MockEvent is one scripted event emitted during MockTool.Run.
// Exactly one of Progress / Log should be non-nil.
type MockEvent struct {
	Progress *MockProgress
	Log      *MockLog
}

type MockProgress struct {
	Pct   float64
	Speed string
	ETA   int
}

type MockLog struct {
	Level  state.LogLevel
	Format string
	Args   []any
}

// MockCall captures one invocation of MockTool.Run.
type MockCall struct {
	Args    []string
	Env     map[string]string
	Workdir string
}

// MockTool is a test double satisfying Tool. It records every Run call
// in Calls() and emits the Events scripted at construction time.
type MockTool struct {
	name   string
	events []MockEvent

	mu    sync.Mutex
	calls []MockCall
	err   error
}

// NewMockTool constructs a MockTool that emits events on every Run.
func NewMockTool(name string, events []MockEvent) *MockTool {
	return &MockTool{name: name, events: events}
}

// SetError configures the next (and subsequent) Run to return err.
func (m *MockTool) SetError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.err = err
}

// Calls returns all recorded invocations.
func (m *MockTool) Calls() []MockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MockCall, len(m.calls))
	copy(out, m.calls)
	return out
}

func (m *MockTool) Name() string { return m.name }

func (m *MockTool) Run(_ context.Context, args []string, env map[string]string,
	workdir string, sink Sink) error {

	m.mu.Lock()
	m.calls = append(m.calls, MockCall{Args: args, Env: env, Workdir: workdir})
	err := m.err
	events := m.events
	m.mu.Unlock()

	for _, e := range events {
		switch {
		case e.Progress != nil:
			sink.Progress(e.Progress.Pct, e.Progress.Speed, e.Progress.ETA)
		case e.Log != nil:
			sink.Log(e.Log.Level, e.Log.Format, e.Log.Args...)
		}
	}
	return err
}
