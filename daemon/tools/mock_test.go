package tools_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

func TestMockTool_RecordsCallAndEmitsScripted(t *testing.T) {
	m := tools.NewMockTool("whipper", []tools.MockEvent{
		{Progress: &tools.MockProgress{Pct: 50, Speed: "5×", ETA: 30}},
		{Log: &tools.MockLog{Level: state.LogLevelInfo, Format: "step done"}},
	})

	sink := &recordingSink{}
	err := m.Run(context.Background(), []string{"-R", "abc"}, nil, "/tmp", sink)
	if err != nil {
		t.Fatal(err)
	}

	calls := m.Calls()
	if len(calls) != 1 {
		t.Fatalf("want 1 call, got %d", len(calls))
	}
	if calls[0].Workdir != "/tmp" || len(calls[0].Args) != 2 {
		t.Errorf("call mismatch: %+v", calls[0])
	}

	if len(sink.events) != 2 {
		t.Fatalf("want 2 events, got %d", len(sink.events))
	}
	if sink.events[0].kind != "progress" || sink.events[0].pct != 50 {
		t.Errorf("event[0] mismatch: %+v", sink.events[0])
	}
}

func TestMockTool_ReturnsConfiguredError(t *testing.T) {
	m := tools.NewMockTool("whipper", nil)
	want := errors.New("boom")
	m.SetError(want)
	err := m.Run(context.Background(), nil, nil, "", tools.NopSink{})
	if err != want {
		t.Errorf("got %v", err)
	}
}
