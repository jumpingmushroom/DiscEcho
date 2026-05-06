package tools_test

import (
	"context"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

func TestEject_Name(t *testing.T) {
	e := tools.NewEject("/usr/bin/eject")
	if e.Name() != "eject" {
		t.Errorf("name: %q", e.Name())
	}
}

func TestEject_RunFailureLogsButReturnsNil(t *testing.T) {
	e := tools.NewEject("/usr/bin/false")
	err := e.Run(context.Background(), []string{"/dev/sr-bogus"}, nil, "", tools.NopSink{})
	if err == nil {
		t.Errorf("eject should propagate exec failure (orchestrator handles it)")
	}
}
