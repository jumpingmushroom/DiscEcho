package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// DD wraps the standard `dd` binary for raw-data disc rips.
type DD struct {
	Bin string // "" → defaults to "dd"
}

// Copy spawns `dd if=devPath of=outPath bs=2048 conv=noerror,sync
// status=progress`. Progress is parsed off stderr and emitted to sink
// against totalBytes. If totalBytes == 0, percentage stays at 0 but
// speed is still reported.
//
// conv=noerror,sync keeps the rip going on read errors and zero-fills
// failed sectors, so the produced ISO is bit-aligned even if the disc
// is degraded. Bad-sector count surfaces from dd's final summary.
func (d *DD) Copy(ctx context.Context, devPath, outPath string, totalBytes int64, sink Sink) error {
	bin := d.Bin
	if bin == "" {
		bin = "dd"
	}
	args := []string{
		"if=" + devPath,
		"of=" + outPath,
		"bs=2048",
		"conv=noerror,sync",
		"status=progress",
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start dd: %w", err)
	}
	scan := bufio.NewScanner(stderr)
	scan.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	scanDDOutput(scan, totalBytes, sink)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("dd: %w", err)
	}
	return nil
}

// scanDDOutput reads dd stderr lines and forwards structured events to sink.
// Separated from Copy for testability via ParseDDProgress.
func scanDDOutput(scan *bufio.Scanner, totalBytes int64, sink Sink) {
	for scan.Scan() {
		line := scan.Text()
		if pct, speed, ok := ParseDDProgress(line, totalBytes); ok {
			sink.Progress(pct, speed, 0)
			continue
		}
		// Emit dd's final summary lines ("N+0 records in/out", "N bytes copied") as info.
		if strings.Contains(line, "records") || strings.Contains(line, "copied") {
			sink.Log(state.LogLevelInfo, "dd: %s", line)
		}
	}
	if err := scan.Err(); err != nil && err != io.EOF {
		sink.Log(state.LogLevelWarn, "dd stderr: %s", err.Error())
	}
}

// ddProgressRE matches dd status=progress lines in both forms:
//
//	123456789 bytes (123 MB, 117 MiB) copied, 12.3 s, 10.0 MB/s
//	987654 bytes copied, 1 s, 1.0 MB/s
var ddProgressRE = regexp.MustCompile(`^\s*(\d+)\s+bytes\b.*?,\s*([\d.]+\s*[KMG]?B/s)`)

// ParseDDProgress extracts (pct, speed) from a dd status=progress stderr line.
// Returns ok=false for empty lines, "records in/out" summary lines, or
// unrelated stderr noise.
func ParseDDProgress(line string, totalBytes int64) (pct float64, speed string, ok bool) {
	if line == "" || strings.Contains(line, "records") {
		return 0, "", false
	}
	m := ddProgressRE.FindStringSubmatch(line)
	if m == nil {
		return 0, "", false
	}
	bytesCopied, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil {
		return 0, "", false
	}
	if totalBytes > 0 {
		pct = 100.0 * float64(bytesCopied) / float64(totalBytes)
	}
	return pct, strings.TrimSpace(m[2]), true
}
