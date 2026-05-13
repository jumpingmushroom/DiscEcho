package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// Redumper wraps the redumper binary. Used by the PSX, PS2, and Xbox
// pipelines for the rip step. Output is .bin/.cue (CD media) or .iso
// (DVD/Xbox media); the caller passes the media mode at Rip time.
type Redumper struct {
	bin string
}

// RedumperOutputExt returns the primary output file extension for the
// given redumper mode: ".cue" for cd, ".iso" for dvd and xbox.
func RedumperOutputExt(mode string) string {
	if mode == "cd" {
		return ".cue"
	}
	return ".iso"
}

// NewRedumper returns a Redumper. Empty bin defaults to "redumper".
func NewRedumper(bin string) *Redumper {
	if bin == "" {
		bin = "redumper"
	}
	return &Redumper{bin: bin}
}

// Name returns the tool name. Used for logging only — Redumper is not
// registered in tools.Registry (its Rip signature doesn't fit
// tools.Tool.Run).
func (r *Redumper) Name() string { return "redumper" }

// Rip dumps the disc to outDir using the given base name. mode is
// "cd", "dvd", or "xbox"; chooses the right redumper subcommand and
// output extension.
//
//	cd   → redumper cd   --drive <devPath> --image-path <outDir>/<name>
//	       → produces <name>.bin + <name>.cue
//	dvd  → redumper dvd  --drive <devPath> --image-path <outDir>/<name>
//	       → produces <name>.iso
//	xbox → redumper xbox --drive <devPath> --image-path <outDir>/<name>
//	       → produces <name>.iso  (XGD security sectors handled by redumper)
//
// Streams progress to sink via ParseRedumperProgress.
func (r *Redumper) Rip(ctx context.Context, devPath, outDir, name, mode string, sink Sink) error {
	if mode != "cd" && mode != "dvd" && mode != "xbox" {
		return fmt.Errorf("redumper: unknown mode %q (want cd|dvd|xbox)", mode)
	}
	imagePath := filepath.Join(outDir, name)
	args := []string{mode, "--drive", devPath, "--image-path", imagePath}
	cmd := exec.CommandContext(ctx, r.bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("redumper start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ParseRedumperProgress(stdout, sink) }()
	go func() { defer wg.Done(); ParseRedumperProgress(stderr, sink) }()
	wg.Wait()

	return cmd.Wait()
}

var (
	redumperLBARE   = regexp.MustCompile(`^LBA:\s*(\d+)/(\d+)`)
	redumperSpeedRE = regexp.MustCompile(`^Speed:\s*([0-9.]+)x`)
)

// ParseRedumperProgress reads a redumper output stream and emits sink
// events.
//
// Recognised lines:
//
//	"LBA: <current>/<max>"   → sink.Progress(percent, speed, 0)
//	"Speed: <N.N>x"          → carries forward as the speed string on
//	                           the next progress event
//
// Unrecognised lines are ignored. The function returns when the
// reader EOFs.
func ParseRedumperProgress(r io.Reader, sink Sink) {
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		var speed string
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if m := redumperSpeedRE.FindStringSubmatch(line); m != nil {
				speed = m[1] + "x"
				continue
			}
			if m := redumperLBARE.FindStringSubmatch(line); m != nil {
				cur, _ := strconv.Atoi(m[1])
				max, _ := strconv.Atoi(m[2])
				if max <= 0 {
					continue
				}
				pct := float64(cur) / float64(max) * 100
				sink.Progress(pct, speed, 0)
			}
		}
	})
}
