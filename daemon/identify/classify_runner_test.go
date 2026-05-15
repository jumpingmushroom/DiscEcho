package identify

import (
	"bytes"
	"context"
	"errors"
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
// cd-info the moment the full disc-mode line shows up, instead of
// waiting for the (potentially hung) MCN/ISRC probes that follow on
// some drive+disc combinations.
//
// The fake prints the line, then sleeps for 30 s, then prints another
// line. The runner should return in well under that 30 s, with the
// line captured and the trailing line absent.
func TestDefaultCDInfoRunner_StopsAtDiscMode(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	// Use a 30s context window. The fake sleeps 30s. If the runner
	// short-circuits within ~15s we know the marker logic fired. The wide
	// window leaves slack for slow CI runners under `-race`.
	fake := writeFakeCDInfo(t, "echo 'Disc mode is listed as: CD-DA'\nsleep 30\necho 'never printed'")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	start := time.Now()
	out, err := defaultCDInfoRunner(ctx, fake, "/dev/null")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("runner: unexpected error %v", err)
	}
	if elapsed > 15*time.Second {
		t.Errorf("runner did not short-circuit: elapsed=%v (want <15s)", elapsed)
	}
	if !bytes.Contains(out, []byte("Disc mode is listed as:")) {
		t.Errorf("output missing marker: %q", out)
	}
	if bytes.Contains(out, []byte("never printed")) {
		t.Errorf("runner read past the marker: %q", out)
	}
}

// TestDefaultCDInfoRunner_PartialLineDoesNotFire reproduces the bug
// where a flush of just `Disc mode is listed as:` (no value, no
// newline) tricked an earlier substring-based watcher into killing
// cd-info before the value (`CD-DA`) followed. The runner must wait
// for the newline and a non-empty value before short-circuiting.
func TestDefaultCDInfoRunner_PartialLineDoesNotFire(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	// First write: prefix only, no newline. Sleep. Then value + newline.
	// If the runner fires on the partial line, it will read 0 lines and
	// return before the value lands.
	fake := writeFakeCDInfo(t, "printf 'Disc mode is listed as:'\nsleep 1\nprintf ' CD-DA\\n'")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := defaultCDInfoRunner(ctx, fake, "/dev/null")
	if err != nil {
		t.Fatalf("runner: %v", err)
	}
	if !bytes.Contains(out, []byte("Disc mode is listed as: CD-DA")) {
		t.Errorf("runner killed before value landed; out=%q", out)
	}
}

// TestDefaultCDInfoRunner_ProcessExitsCleanly_NoMarker covers the
// path where the fake exits with status 0 but never prints a usable
// disc-mode line. The runner must return errCDInfoDiscNotReady so the
// retry loop can re-run cd-info — feeding the incomplete output to
// the parser would silently mis-classify the disc as DATA.
func TestDefaultCDInfoRunner_ProcessExitsCleanly_NoMarker(t *testing.T) {
	fake := writeFakeCDInfo(t, "echo 'no marker here'")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := defaultCDInfoRunner(ctx, fake, "/dev/null")
	if !errors.Is(err, errCDInfoDiscNotReady) {
		t.Fatalf("want errCDInfoDiscNotReady, got %v", err)
	}
	if !bytes.Contains(out, []byte("no marker here")) {
		t.Errorf("output missing exit content: %q", out)
	}
}

// TestDefaultCDInfoRunner_ErrorValueRetries verifies that when cd-info
// exits cleanly but the disc-mode value is an error string (the
// ASUS SDRW-08D2S-U "Error in getting information" spin-up race), the
// runner returns errCDInfoDiscNotReady so the retry loop kicks in
// instead of feeding the bad output to the parser.
func TestDefaultCDInfoRunner_ErrorValueRetries(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	fake := writeFakeCDInfo(t, "echo 'Disc mode is listed as: Error in getting information'")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := defaultCDInfoRunner(ctx, fake, "/dev/null")
	if !errors.Is(err, errCDInfoDiscNotReady) {
		t.Fatalf("runner: want errCDInfoDiscNotReady, got %v", err)
	}
	if !bytes.Contains(out, []byte("Error in getting information")) {
		t.Errorf("expected buffer to retain the error line; got %q", out)
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
