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

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// FormatLog applies fmt.Sprintf and is exposed so test sinks can format
// log lines the same way production ones do.
func FormatLog(format string, args ...any) string {
	return fmt.Sprintf(format, args...)
}

// Whipper wraps the `whipper` CLI for audio CD ripping.
type Whipper struct {
	bin string
}

// NewWhipper returns a Whipper that runs <bin>. Empty defaults to
// "whipper" (resolved via PATH).
func NewWhipper(bin string) *Whipper {
	if bin == "" {
		bin = "whipper"
	}
	return &Whipper{bin: bin}
}

func (w *Whipper) Name() string { return "whipper" }

// Run shells out to whipper and parses its combined stdout/stderr
// stream into Sink events. Returns the exec error (or nil) verbatim.
func (w *Whipper) Run(ctx context.Context, args []string, env map[string]string,
	workdir string, sink Sink) error {

	cmd := exec.CommandContext(ctx, w.bin, args...)
	if workdir != "" {
		cmd.Dir = workdir
	}
	if len(env) > 0 {
		envSlice := cmd.Environ()
		for k, v := range env {
			envSlice = append(envSlice, k+"="+v)
		}
		cmd.Env = envSlice
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("whipper start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ParseWhipperStream(stdout, sink)
	}()
	go func() {
		defer wg.Done()
		ParseWhipperStream(stderr, sink)
	}()
	wg.Wait()

	return cmd.Wait()
}

var (
	whipperTrackStartRE = regexp.MustCompile(`^Ripping track (\d+) of (\d+)`)
	whipperReadingRE    = regexp.MustCompile(`Reading:\s+([0-9.]+)%,\s*([0-9.]+×),\s*ETA:\s*(\d+):(\d+)`)
	whipperErrorRE      = regexp.MustCompile(`(?i)^(ERROR|FATAL):\s*(.+)$`)
	whipperTrackOKRE    = regexp.MustCompile(`^Track (\d+) OK \(AccurateRip:\s*(\d+)/\d+`)
	// whipperPyLogRE matches Python `logging` framework output
	// (`LEVEL:logger.path:message`) which whipper uses for all its
	// status messages during the startup phase — AccurateRip lookup,
	// drive offset detection, TOC re-read, etc. Without forwarding
	// these to the sink the user sees an empty Log tab for the first
	// 1–3 minutes of every audio rip while whipper warms up.
	whipperPyLogRE = regexp.MustCompile(`^(INFO|WARNING|ERROR|FATAL|CRITICAL|DEBUG):[^:]+:(.+)$`)
)

// ParseWhipperStream scans r line-by-line and emits events to sink.
// Exposed for testing.
func ParseWhipperStream(r io.Reader, sink Sink) {
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		parseWhipperLines(scanner, sink)
	})
}

func parseWhipperLines(scanner *bufio.Scanner, sink Sink) {
	currentTrack := 0
	totalTracks := 0
	announcedStart := false

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := stripLeadingSpaces(line)

		// First non-empty line we see — emit a "preparing" hint so the
		// dashboard's drive card doesn't sit at "0% / no speed / no
		// ETA" with no apparent activity while whipper warms up.
		if !announcedStart && trimmed != "" {
			announcedStart = true
			sink.Log(state.LogLevelInfo, "whipper: preparing drive (this can take 1–3 min)")
		}

		if m := whipperTrackStartRE.FindStringSubmatch(trimmed); m != nil {
			t, _ := strconv.Atoi(m[1])
			n, _ := strconv.Atoi(m[2])
			currentTrack = t
			totalTracks = n
			sink.Log(state.LogLevelInfo, "whipper: starting track %d/%d", t, n)
			continue
		}

		if m := whipperReadingRE.FindStringSubmatch(trimmed); m != nil {
			if currentTrack == 0 || totalTracks == 0 {
				continue
			}
			pct, _ := strconv.ParseFloat(m[1], 64)
			speed := m[2]
			etaMin, _ := strconv.Atoi(m[3])
			etaSec, _ := strconv.Atoi(m[4])
			overall := (float64(currentTrack-1) + pct/100) / float64(totalTracks) * 100
			sink.Progress(overall, speed, etaMin*60+etaSec)
			continue
		}

		if m := whipperTrackOKRE.FindStringSubmatch(trimmed); m != nil {
			tNum, _ := strconv.Atoi(m[1])
			conf, _ := strconv.Atoi(m[2])
			sink.Log(state.LogLevelInfo, "whipper: track %d OK (AccurateRip %d)", tNum, conf)
			continue
		}

		if m := whipperErrorRE.FindStringSubmatch(trimmed); m != nil {
			sink.Log(state.LogLevelError, "whipper: %s", m[2])
			continue
		}

		// Forward Python-logging-formatted output so the user sees
		// whipper's startup phase activity in the Log tab. DEBUG is
		// dropped (cdparanoia chatter). Everything else maps to the
		// matching DiscEcho log level.
		if m := whipperPyLogRE.FindStringSubmatch(trimmed); m != nil {
			lvl := m[1]
			if lvl == "DEBUG" {
				continue
			}
			level := state.LogLevelInfo
			switch lvl {
			case "WARNING":
				level = state.LogLevelWarn
			case "ERROR", "FATAL", "CRITICAL":
				level = state.LogLevelError
			}
			sink.Log(level, "whipper: %s", strings.TrimSpace(m[2]))
		}
	}
}

func stripLeadingSpaces(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:]
}
