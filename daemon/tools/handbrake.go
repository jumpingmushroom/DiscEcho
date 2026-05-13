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

// HandBrakeTitle is one title from a `HandBrakeCLI --scan` output.
type HandBrakeTitle struct {
	Number          int
	DurationSeconds int
}

// HandBrake wraps the HandBrakeCLI binary.
type HandBrake struct {
	bin string
}

// NewHandBrake returns a HandBrake tool. Empty bin defaults to
// "HandBrakeCLI" (resolved via PATH).
func NewHandBrake(bin string) *HandBrake {
	if bin == "" {
		bin = "HandBrakeCLI"
	}
	return &HandBrake{bin: bin}
}

// Name implements Tool.
func (h *HandBrake) Name() string { return "handbrake" }

// Run shells out to HandBrakeCLI. The caller passes appropriate args
// (full encode flags). Stdout+stderr are merged and parsed for encode
// progress. Title context comes from env: HB_TITLE_IDX (1-based) and
// HB_TOTAL_TITLES; both default to 1 if unset.
func (h *HandBrake) Run(ctx context.Context, args []string, env map[string]string,
	workdir string, sink Sink) error {

	cmd := exec.CommandContext(ctx, h.bin, args...)
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
		return fmt.Errorf("handbrake start: %w", err)
	}

	titleIdx := envInt(env, "HB_TITLE_IDX", 1)
	totalTitles := envInt(env, "HB_TOTAL_TITLES", 1)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ParseHandBrakeEncodeStream(stdout, titleIdx, totalTitles, sink)
	}()
	go func() {
		defer wg.Done()
		ParseHandBrakeEncodeStream(stderr, titleIdx, totalTitles, sink)
	}()
	wg.Wait()

	return cmd.Wait()
}

// Scan runs HandBrakeCLI with --scan and parses the title list.
func (h *HandBrake) Scan(ctx context.Context, devPath string) ([]HandBrakeTitle, error) {
	cmd := exec.CommandContext(ctx, h.bin, scanArgs(devPath)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		titles, perr := ParseHandBrakeScan(string(out))
		if perr == nil {
			return titles, nil
		}
		return nil, fmt.Errorf("handbrake --scan: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return ParseHandBrakeScan(string(out))
}

// scanArgs builds the argv for a full-title-list HandBrakeCLI scan.
// `--title 0` is the critical bit — without it, HandBrakeCLI defaults
// to title_index=1 and only ever enumerates title 1, which left every
// multi-title DVD (Jackass: The Movie, season-set TV discs, anything
// with menu-driven structure) reporting a single short preview title
// instead of the real feature.
func scanArgs(devPath string) []string {
	return []string{"--input", devPath, "--title", "0", "--scan"}
}

func envInt(env map[string]string, key string, fallback int) int {
	v, ok := env[key]
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

var (
	hbTitleStartRE = regexp.MustCompile(`^\+ title (\d+):`)
	hbDurationRE   = regexp.MustCompile(`\+ duration:\s+(\d+):(\d+):(\d+)`)
	// hbEncodingRE matches HandBrake's `Encoding: task N of M, P %` line.
	// The `(avg fps X, ETA YhYmYs)` tail is optional — HandBrake omits it
	// entirely when stdout isn't a tty (which is our case, since we read
	// from a pipe). We still capture it when present so newer versions or
	// terminal-attached runs give us speed/ETA.
	hbEncodingRE = regexp.MustCompile(
		`Encoding:\s+task\s+(\d+)\s+of\s+(\d+),\s+([0-9.]+)\s*%` +
			`(?:\s+\(avg fps\s+([0-9.]+),\s+ETA\s+(\d+)h(\d+)m(\d+)s\))?`)
)

// ParseHandBrakeScan extracts the title list from `HandBrakeCLI --scan`
// output. Returns an error when nothing parses.
func ParseHandBrakeScan(s string) ([]HandBrakeTitle, error) {
	var titles []HandBrakeTitle
	current := -1
	for _, line := range strings.Split(s, "\n") {
		if m := hbTitleStartRE.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[1])
			titles = append(titles, HandBrakeTitle{Number: n})
			current = len(titles) - 1
			continue
		}
		if current < 0 {
			continue
		}
		if m := hbDurationRE.FindStringSubmatch(line); m != nil {
			h, _ := strconv.Atoi(m[1])
			min, _ := strconv.Atoi(m[2])
			sec, _ := strconv.Atoi(m[3])
			titles[current].DurationSeconds = h*3600 + min*60 + sec
		}
	}
	if len(titles) == 0 {
		return nil, fmt.Errorf("no titles found in HandBrake scan output")
	}
	return titles, nil
}

// ParseHandBrakeEncodeStream scans an encoding output stream and emits
// Sink progress events. titleIdx is the 1-based index of the current
// title within the job's encode set; totalTitles is the count.
//
// Overall progress = ((titleIdx-1) + intraTitlePct/100) / totalTitles * 100.
//
// HandBrake separates progress lines with '\r' rather than '\n' to
// overstrike the terminal; we use splitCROrLF so each progress chunk
// becomes its own scanner token. drainAfterScan guarantees the pipe
// keeps being drained even if our parse goroutine returns early —
// without that, a >1 MB token would let the scanner exit and cause
// HandBrake to deadlock on the next pipe write.
func ParseHandBrakeEncodeStream(r io.Reader, titleIdx, totalTitles int, sink Sink) {
	if totalTitles <= 0 {
		totalTitles = 1
	}
	if titleIdx <= 0 {
		titleIdx = 1
	}
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		scanner.Split(splitCROrLF)
		parseHandBrakeEncodeLines(scanner, titleIdx, totalTitles, sink)
	})
}

func parseHandBrakeEncodeLines(scanner *bufio.Scanner, titleIdx, totalTitles int, sink Sink) {
	const stderrCap = 200
	stderrSeen := 0
	capWarned := false

	for scanner.Scan() {
		line := scanner.Text()
		m := hbEncodingRE.FindStringSubmatch(line)
		if m != nil {
			intra, _ := strconv.ParseFloat(m[3], 64)
			overall := (float64(titleIdx-1) + intra/100) / float64(totalTitles) * 100

			// fps/ETA group is optional; m[4]…m[7] are empty when the tail
			// wasn't in the line (HandBrake 1.6.x on a pipe).
			var speed string
			var etaSeconds int
			if m[4] != "" {
				fps, _ := strconv.ParseFloat(m[4], 64)
				etaH, _ := strconv.Atoi(m[5])
				etaM, _ := strconv.Atoi(m[6])
				etaS, _ := strconv.Atoi(m[7])
				etaSeconds = etaH*3600 + etaM*60 + etaS
				speed = fmt.Sprintf("%.1ffps", fps)
			}
			sink.Progress(overall, speed, etaSeconds)
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// HandBrake's `[hh:mm:ss] ...` config-dump lines are voluminous
		// and not actionable for end users — skip unless they contain
		// an error keyword. Real warnings/errors usually surface as
		// `x264 [error]: ...` or `[hh:mm:ss] err: ...`.
		if strings.HasPrefix(trimmed, "[") && !strings.Contains(strings.ToLower(trimmed), "error") {
			continue
		}

		if stderrSeen >= stderrCap {
			if !capWarned {
				sink.Log(state.LogLevelWarn, "HandBrake: stderr cap reached, dropping further lines")
				capWarned = true
			}
			continue
		}
		stderrSeen++
		sink.Log(state.LogLevelWarn, "HandBrake: %s", trimmed)
	}
}
