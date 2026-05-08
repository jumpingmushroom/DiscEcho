package tools_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

func TestApprise_DryRun_OK(t *testing.T) {
	stub := writeStubBin(t, 0, "")
	a := tools.NewApprise(stub)
	if err := a.DryRun(context.Background(), "ntfy://example.com/topic"); err != nil {
		t.Fatalf("expected nil; got %v", err)
	}
}

func TestApprise_DryRun_BadURL(t *testing.T) {
	stub := writeStubBin(t, 1, "Could not load URL\n")
	a := tools.NewApprise(stub)
	err := a.DryRun(context.Background(), "bogus://nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "Could not load URL") {
		t.Fatalf("error must surface stderr; got %q", err.Error())
	}
}

func TestApprise_Send_OK(t *testing.T) {
	stub := writeStubBin(t, 0, "")
	a := tools.NewApprise(stub)
	if err := a.Send(context.Background(), []string{"ntfy://x"}, "title", "body"); err != nil {
		t.Fatalf("expected nil; got %v", err)
	}
}

func TestApprise_Send_Failure(t *testing.T) {
	stub := writeStubBin(t, 1, "delivery failed\n")
	a := tools.NewApprise(stub)
	err := a.Send(context.Background(), []string{"ntfy://x"}, "title", "body")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "delivery failed") {
		t.Fatalf("error must surface stderr; got %q", err.Error())
	}
}

// writeStubBin writes a tiny shell script to a temp dir that exits with
// the given code and prints stderr to stderr. Returns the path.
func writeStubBin(t *testing.T, exitCode int, stderr string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "apprise-stub.sh")
	script := fmt.Sprintf("#!/bin/sh\nprintf %%s %q >&2\nexit %d\n", stderr, exitCode)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return path
}
