package identify

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func writeFakeCDInfo(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake-cdinfo")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestDefaultCDInfoRunner_StopsAtDiscMode verifies the runner kills
// cd-info the moment the disc-mode marker shows up, instead of waiting
// for the (potentially hung) MCN/ISRC probes that follow on some
// drive+disc combinations.
//
// The fake prints the marker, then sleeps for 30 s, then prints another
// line. The runner should return in well under that 30 s, with the
// marker captured and the trailing line absent.
func TestDefaultCDInfoRunner_StopsAtDiscMode(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	fake := writeFakeCDInfo(t, "echo 'Disc mode is listed as: CD-DA'\nsleep 30\necho 'never printed'")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	out, err := defaultCDInfoRunner(ctx, fake, "/dev/null")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("runner: unexpected error %v", err)
	}
	if elapsed > 3*time.Second {
		t.Errorf("runner did not short-circuit: elapsed=%v (want <3s)", elapsed)
	}
	if !bytes.Contains(out, []byte("Disc mode is listed as:")) {
		t.Errorf("output missing marker: %q", out)
	}
	if bytes.Contains(out, []byte("never printed")) {
		t.Errorf("runner read past the marker: %q", out)
	}
}

// TestDefaultCDInfoRunner_ProcessExitsCleanly covers the path where the
// fake never prints the marker. The runner should wait for the process
// and return whatever it exited with.
func TestDefaultCDInfoRunner_ProcessExitsCleanly(t *testing.T) {
	fake := writeFakeCDInfo(t, "echo 'no marker here'")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := defaultCDInfoRunner(ctx, fake, "/dev/null")
	if err != nil {
		t.Errorf("expected nil error from clean exit, got %v", err)
	}
	if !bytes.Contains(out, []byte("no marker here")) {
		t.Errorf("output missing exit content: %q", out)
	}
}

// TestDefaultCDInfoRunner_ContextCancel covers the ctx-cancelled path
// where the marker never appears and the process never exits on its own.
// We invoke /bin/sleep directly (not a wrapper script) so the kill kills
// the actual long-running process rather than its parent shell, which
// would otherwise leave the orphan sleep running while Wait already
// returned.
func TestDefaultCDInfoRunner_ContextCancel(t *testing.T) {
	sleepBin, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = defaultCDInfoRunner(ctx, sleepBin, "10")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected context error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("runner did not honour ctx: elapsed=%v", elapsed)
	}
}
