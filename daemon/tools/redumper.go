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
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
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
	// Modern redumper (b720+) emits progress as:
	//   `/ [ 2%] LBA: 60928/2161648, errors: { SCSI: 0, EDC: 0 }`
	// The leading `/`/`-`/`\`/`|` is a spinner that cycles on
	// in-place `\r` updates. The percent in `[ NN%]` is pre-computed,
	// and the `LBA: cur/max` pair follows mid-line. We capture the
	// percent directly when present (cheap, accurate), and fall back
	// to dividing cur/max when only the LBA pair is present (some
	// phase headers print `LBA: 0/N` without the percent prefix).
	redumperPercentRE = regexp.MustCompile(`\[\s*(\d+)%\]`)
	redumperLBARE     = regexp.MustCompile(`LBA:\s*(\d+)/(\d+)`)
	redumperSpeedRE   = regexp.MustCompile(`Speed:\s*([0-9.]+)x`)
)

// ParseRedumperProgress reads a redumper output stream and emits sink
// events.
//
// Recognised lines:
//
//	"/ [ NN%] LBA: <cur>/<max>"  → sink.Progress(pct, speed, etaSeconds)
//	"LBA: <cur>/<max>"           → same, computing percent from cur/max
//	"Speed: <N.N>x"              → legacy redumper format; carries forward
//
// Speed and ETA are derived because b720+ doesn't print either on the
// progress line. Speed = (deltaSectors × 2048) / deltaWallTime, formatted
// as "X.X MB/s". ETA = elapsedWallTime × (100-pct) / (pct-firstPct),
// where firstPct is the percent when we first saw a progress line — this
// extrapolates from real elapsed time rather than a static read-speed
// assumption.
//
// All other non-empty lines are forwarded to sink.Log so they appear
// in the job's log tail. The scanner treats both '\r' and '\n' as line
// terminators because redumper b720+ overwrites its progress line with
// carriage returns; the default ScanLines would buffer the entire rip
// phase as a single token.
func ParseRedumperProgress(r io.Reader, sink Sink) {
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		scanner.Split(splitCROrLF)
		state := newRedumperRate()
		var legacySpeed string
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if m := redumperSpeedRE.FindStringSubmatch(line); m != nil {
				legacySpeed = m[1] + "x"
				// don't continue — single line may carry Speed AND progress.
			}

			lbaMatch := redumperLBARE.FindStringSubmatch(line)
			var cur int
			if lbaMatch != nil {
				cur, _ = strconv.Atoi(lbaMatch[1])
			}

			emit := func(pct float64) {
				now := redumperNow()
				speed := legacySpeed
				if lbaMatch != nil {
					if s := state.observeLBA(cur, now); s != "" {
						speed = s
					}
				}
				eta := state.observePercent(pct, now)
				sink.Progress(pct, speed, eta)
			}

			// Prefer the pre-computed `[ NN%]` percent.
			if m := redumperPercentRE.FindStringSubmatch(line); m != nil {
				pct, _ := strconv.Atoi(m[1])
				emit(float64(pct))
				continue
			}
			if lbaMatch != nil {
				max, _ := strconv.Atoi(lbaMatch[2])
				if max <= 0 {
					continue
				}
				emit(float64(cur) / float64(max) * 100)
				continue
			}
			if len(line) > 400 {
				line = line[:400]
			}
			sink.Log(stateLogLevelInfoConst, "redumper: %s", line)
		}
	})
}

// redumperNow is a package var so tests can substitute a deterministic clock.
var redumperNow = func() time.Time { return time.Now() }

// stateLogLevelInfoConst aliases the state.LogLevelInfo constant. Pulled
// out into a var so the import stays in one place and tests can reference
// the same constant without a circular import.
var stateLogLevelInfoConst = state.LogLevelInfo

// redumperRateTracker derives speed (MB/s) and ETA seconds from a stream
// of LBA + percent samples. Zero-value is unusable; call newRedumperRate.
type redumperRateTracker struct {
	firstSeen      time.Time
	firstPct       float64
	lastSampleTime time.Time
	lastLBA        int
}

func newRedumperRate() *redumperRateTracker {
	return &redumperRateTracker{}
}

// observeLBA computes an instantaneous MB/s from the LBA delta since the
// previous sample. Returns the empty string when there's no usable delta
// (first sample, or no time has elapsed). 2048 bytes per DVD/CD sector.
func (r *redumperRateTracker) observeLBA(cur int, now time.Time) string {
	defer func() { r.lastLBA = cur; r.lastSampleTime = now }()
	if r.lastSampleTime.IsZero() {
		return ""
	}
	dt := now.Sub(r.lastSampleTime).Seconds()
	if dt <= 0 {
		return ""
	}
	dSec := cur - r.lastLBA
	if dSec <= 0 {
		return ""
	}
	bytesPerSec := float64(dSec) * 2048 / dt
	return fmt.Sprintf("%.1f MB/s", bytesPerSec/(1024*1024))
}

// observePercent extrapolates ETA seconds from wall-time elapsed since
// the first observation. Returns 0 until we have a measurable percent
// delta (avoids divide-by-zero and noisy first-sample ETAs).
func (r *redumperRateTracker) observePercent(pct float64, now time.Time) int {
	if r.firstSeen.IsZero() {
		r.firstSeen = now
		r.firstPct = pct
		return 0
	}
	pctDone := pct - r.firstPct
	if pctDone <= 0 {
		return 0
	}
	elapsed := now.Sub(r.firstSeen).Seconds()
	remaining := 100 - pct
	if remaining <= 0 {
		return 0
	}
	return int(elapsed * remaining / pctDone)
}
