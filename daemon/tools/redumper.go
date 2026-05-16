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
// "cd", "dvd", or "xbox"; selects the right --disc-type override and
// invokes the `disc` aggregate subcommand (which runs dump+refine+split
// in one pass).
//
//	cd   → redumper disc --disc-type=CD  --drive <devPath> --image-path <outDir> --image-name <name>
//	       → produces <outDir>/<name>.bin + <outDir>/<name>.cue (after the split phase)
//	dvd  → redumper disc --disc-type=DVD --drive <devPath> --image-path <outDir> --image-name <name>
//	       → produces <outDir>/<name>.iso
//	xbox → redumper disc --disc-type=DVD --drive <devPath> --image-path <outDir> --image-name <name>
//	       → produces <outDir>/<name>.iso  (XGD discs are DVD media;
//	         redumper's security-sector handling kicks in automatically
//	         when it detects the XGD structure)
//
// Older redumper releases shipped per-media subcommands (`redumper cd`,
// `redumper dvd`, `redumper xbox`); current builds (b720+) use a single
// `disc` aggregate. `--image-path` is the OUTPUT DIRECTORY (redumper
// creates it if missing) and `--image-name` is the file prefix the
// daemon uses to find the output afterwards. Streams progress to sink
// via ParseRedumperProgress.
func (r *Redumper) Rip(ctx context.Context, devPath, outDir, name, mode string, sink Sink) error {
	discType, ok := redumperDiscType(mode)
	if !ok {
		return fmt.Errorf("redumper: unknown mode %q (want cd|dvd|xbox)", mode)
	}
	args := []string{
		"disc",
		"--disc-type=" + discType,
		"--drive", devPath,
		"--image-path", outDir,
		"--image-name", name,
	}
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

// redumperDiscType maps the daemon's pipeline-side mode string to
// redumper's --disc-type value. Xbox uses DVD media (XGD); redumper
// detects the XGD security structure on its own.
func redumperDiscType(mode string) (string, bool) {
	switch mode {
	case "cd":
		return "CD", true
	case "dvd":
		return "DVD", true
	case "xbox":
		return "DVD", true
	}
	return "", false
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
