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
	titleDuration := envInt(env, "HB_TITLE_DURATION", 0)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		ParseHandBrakeEncodeStream(stdout, titleIdx, totalTitles, titleDuration, sink)
	}()
	go func() {
		defer wg.Done()
		ParseHandBrakeEncodeStream(stderr, titleIdx, totalTitles, titleDuration, sink)
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
// titleDurationSeconds is the source title's duration — used to derive
// a realtime-multiple speed and an accurate ETA, since HandBrake omits
// its own `(avg fps, ETA)` tail when stdout is a pipe. 0 → ETA is
// extrapolated from the percentage alone, with no speed.
//
// Overall progress = ((titleIdx-1) + intraTitlePct/100) / totalTitles * 100.
//
// HandBrake separates progress lines with '\r' rather than '\n' to
// overstrike the terminal; we use splitCROrLF so each progress chunk
// becomes its own scanner token. drainAfterScan guarantees the pipe
// keeps being drained even if our parse goroutine returns early —
// without that, a >1 MB token would let the scanner exit and cause
// HandBrake to deadlock on the next pipe write.
func ParseHandBrakeEncodeStream(r io.Reader, titleIdx, totalTitles, titleDurationSeconds int, sink Sink) {
	if totalTitles <= 0 {
		totalTitles = 1
	}
	if titleIdx <= 0 {
		titleIdx = 1
	}
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		scanner.Split(splitCROrLF)
		parseHandBrakeEncodeLines(scanner, titleIdx, totalTitles, titleDurationSeconds, sink)
	})
}

func parseHandBrakeEncodeLines(scanner *bufio.Scanner, titleIdx, totalTitles, titleDurationSeconds int, sink Sink) {
	const lineCap = 200
	logSeen := 0
	capWarned := false

	// Anchor for ETA extrapolation: the wall-clock time and intra-title
	// percentage of the first progress event seen on this stream.
	var firstProgressAt time.Time
	var firstProgressIntra float64

	for scanner.Scan() {
		line := scanner.Text()
		if m := hbEncodingRE.FindStringSubmatch(line); m != nil {
			intra, _ := strconv.ParseFloat(m[3], 64)
			overall := (float64(titleIdx-1) + intra/100) / float64(totalTitles) * 100

			var speed string
			var etaSeconds int
			if m[4] != "" {
				// HandBrake gave us a real fps/ETA tail (tty-attached,
				// or a future build that emits it on a pipe).
				fps, _ := strconv.ParseFloat(m[4], 64)
				etaH, _ := strconv.Atoi(m[5])
				etaM, _ := strconv.Atoi(m[6])
				etaS, _ := strconv.Atoi(m[7])
				etaSeconds = etaH*3600 + etaM*60 + etaS
				speed = fmt.Sprintf("%.1ffps", fps)
			} else {
				// Pipe case: HandBrake omits the tail, so derive speed
				// and ETA from elapsed wall-clock since the first event.
				if firstProgressAt.IsZero() {
					firstProgressAt = time.Now()
					firstProgressIntra = intra
				}
				speed, etaSeconds = deriveEncodeETA(
					firstProgressIntra, intra,
					time.Since(firstProgressAt), titleDurationSeconds)
			}
			sink.Progress(overall, speed, etaSeconds)
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// HandBrake logs its fully-resolved job as pretty-printed JSON
		// to stderr at startup — pure noise in the job log.
		if isJSONNoise(trimmed) {
			continue
		}
		// `[hh:mm:ss] ...` activity-log lines are voluminous and not
		// actionable — skip unless the line carries an error.
		if strings.HasPrefix(trimmed, "[") && !hasErrorKeyword(trimmed) {
			continue
		}

		if logSeen >= lineCap {
			if !capWarned {
				sink.Log(state.LogLevelWarn, "HandBrake: log cap reached, dropping further lines")
				capWarned = true
			}
			continue
		}
		logSeen++
		level := state.LogLevelInfo
		if hasErrorKeyword(trimmed) || strings.Contains(strings.ToLower(trimmed), "warning") {
			level = state.LogLevelWarn
		}
		sink.Log(level, "HandBrake: %s", trimmed)
	}
}

// deriveEncodeETA computes a realtime-multiple speed string and an ETA
// in seconds for a HandBrake encode. firstIntra is the intra-title
// percentage at the first observed progress event, curIntra the
// current percentage, elapsed the wall-clock since that first event,
// and titleDurationSeconds the source title's duration.
//
// Anchoring on the first event rather than 0% avoids crediting
// pre-observation progress to the elapsed window. Returns empty/zero
// until progress has actually advanced. With titleDurationSeconds <= 0
// the speed can't be expressed as a realtime multiple, so only the
// percentage-extrapolated ETA is returned.
func deriveEncodeETA(firstIntra, curIntra float64, elapsed time.Duration, titleDurationSeconds int) (speed string, etaSeconds int) {
	deltaIntra := curIntra - firstIntra
	elapsedSec := elapsed.Seconds()
	if deltaIntra <= 0 || elapsedSec <= 0 || curIntra >= 100 {
		return "", 0
	}
	if titleDurationSeconds > 0 {
		encodedDelta := deltaIntra / 100 * float64(titleDurationSeconds)
		rate := encodedDelta / elapsedSec // encoded seconds per wall second
		if rate <= 0 {
			return "", 0
		}
		remaining := (100 - curIntra) / 100 * float64(titleDurationSeconds)
		return fmt.Sprintf("%.1fx", rate), int(remaining / rate)
	}
	pctRate := deltaIntra / elapsedSec // percent per wall second
	return "", int((100 - curIntra) / pctRate)
}

// isJSONNoise reports whether a trimmed line belongs to HandBrake's
// pretty-printed JSON job dump (it logs the fully-resolved job spec to
// stderr at startup). Those lines flood the log with no actionable
// content.
func isJSONNoise(trimmed string) bool {
	if strings.HasPrefix(trimmed, `"`) {
		return true
	}
	switch trimmed {
	case "{", "}", "[", "]", "},", "],":
		return true
	}
	return false
}

// ProbeNVENC returns true when the bundled HandBrake-CLI can actually
// use NVENC on this host. Modern HandBrake builds (1.11+) link NVENC
// at compile-time and `--encoder-preset-list nvenc_h265` succeeds and
// lists presets unconditionally — even when the runtime
// libnvidia-encode.so.1 is missing. The probe handles this by:
//
//  1. Failing on non-zero exit (binary missing, totally broken).
//  2. Returning false if HandBrake reported "Cannot load
//     libnvidia-encode.so.1" — that line signals the NVENC userland
//     is missing on the host, so any encode would dlopen-fail.
//  3. Returning true only when at least one preset name appears in
//     the output AND no library-load failure was reported.
//
// Cheap: one subprocess fork, ~50–150 ms on a configured host.
func ProbeNVENC(handBrakeBin string) bool {
	if handBrakeBin == "" {
		handBrakeBin = "HandBrakeCLI"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, handBrakeBin,
		"--encoder-preset-list", "nvenc_h265").CombinedOutput()
	if err != nil {
		return false
	}
	low := strings.ToLower(string(out))
	if strings.Contains(low, "cannot load libnvidia-encode") {
		return false
	}
	for _, p := range []string{"fast", "medium", "slow", "quality", "default"} {
		if strings.Contains(low, p) {
			return true
		}
	}
	return false
}
