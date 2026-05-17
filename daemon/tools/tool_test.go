package tools_test

import (
	"context"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

type fakeTool struct{ name string }

func (f *fakeTool) Name() string { return f.name }
func (f *fakeTool) Run(_ context.Context, _ []string, _ map[string]string, _ string, _ tools.Sink) error {
	return nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&fakeTool{name: "whipper"})
	r.Register(&fakeTool{name: "apprise"})

	got, ok := r.Get("whipper")
	if !ok || got.Name() != "whipper" {
		t.Errorf("Get(whipper): ok=%v, name=%v", ok, got)
	}
	if _, ok := r.Get("missing"); ok {
		t.Errorf("Get(missing) should be false")
	}
}

func TestRegistry_RegisterIsIdempotent(t *testing.T) {
	r := tools.NewRegistry()
	r.Register(&fakeTool{name: "x"})
	r.Register(&fakeTool{name: "x"})
	got, _ := r.Get("x")
	if got.Name() != "x" {
		t.Errorf("got %s", got.Name())
	}
}

func TestSink_NopSink(t *testing.T) {
	var s tools.Sink = tools.NopSink{}
	s.Progress(50, "5×", 30)
	s.Log(state.LogLevelInfo, "hello %s", "world")
	s.SubStep("REFINE")
}
