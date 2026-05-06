package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/tools"
)

func TestApprise_BuildArgs_SingleURL(t *testing.T) {
	args := tools.BuildAppriseArgs("Job done", "Kind of Blue ripped", "music", []string{"ntfys://discecho"})
	got := strings.Join(args, " ")
	want := "-t Job done -b Kind of Blue ripped --tag music ntfys://discecho"
	if got != want {
		t.Errorf("args mismatch:\nwant %s\ngot  %s", want, got)
	}
}

func TestApprise_BuildArgs_NoTag(t *testing.T) {
	args := tools.BuildAppriseArgs("t", "b", "", []string{"ntfys://x"})
	for _, a := range args {
		if a == "--tag" {
			t.Errorf("--tag should be omitted when empty")
		}
	}
}

func TestApprise_Name(t *testing.T) {
	a := tools.NewApprise("/usr/local/bin/apprise")
	if a.Name() != "apprise" {
		t.Errorf("name: %q", a.Name())
	}
}

func TestApprise_FailureSwallowed(t *testing.T) {
	a := tools.NewApprise("/usr/bin/false")
	err := a.Run(context.Background(), []string{"-t", "x", "-b", "y", "ntfys://z"}, nil, "", tools.NopSink{})
	if err != nil {
		t.Errorf("apprise should swallow exec errors, got %v", err)
	}
}
