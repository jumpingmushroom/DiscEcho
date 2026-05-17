package tools

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// DDRescue wraps GNU ddrescue (Debian package gddrescue) for raw-data
// disc rips. Replaces the older dd-based copier for data discs because
// dd with `conv=noerror,sync` retries each bad sector synchronously
// (the drive's per-sector retry can be ~25 s on a damaged CD); a disc
// with a few hundred bad sectors stalls dd for hours. ddrescue does a
// fast forward pass that skips on error, then revisits the gaps in a
// second pass with trim + scrape phases, recovering far more data in
// far less wall time on the same disc.
type DDRescue struct {
	Bin string // "" → defaults to "ddrescue"
}

// Copy runs `ddrescue -b 2048 -n -d <devPath> <outPath> <outPath>.map`
// and parses its stderr for progress events.
//
// Flags:
//   - `-b 2048`: CD/DVD logical sector size. ddrescue's default 512 issues
//     four reads per sector and crawls.
//   - `-n`: skip the scraping phase (single-sector retries after trim).
//     Scraping is the slow part on heavily damaged media; the trim phase
//     still recovers most short error runs without it. Discs that need
//     scraping can be re-run with a longer-running profile later.
//   - `-d`: direct disc access (bypass kernel block cache). Stops the
//     kernel from prefetching across bad sectors and amplifying their
//     wall-time cost.
//
// The mapfile is written alongside the ISO. It is required for ddrescue
// to resume across phases; callers can discard or keep it.
//
// totalBytes is used purely so the parser can attribute progress to the
// expected disc size when ddrescue itself prints status updates. When 0
// the parser falls back to ddrescue's own `pct rescued` percentage.
func (d *DDRescue) Copy(ctx context.Context, devPath, outPath string, totalBytes int64, sink Sink) error {
	bin := d.Bin
	if bin == "" {
		bin = "ddrescue"
	}
	mapPath := outPath + ".map"
	args := []string{
		"-b", "2048",
		"-n",
		"-d",
		"--force",
		devPath,
		outPath,
		mapPath,
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ddrescue: %w", err)
	}
	drainAfterScan(stderr, func(scan *bufio.Scanner) {
		// ddrescue overstrikes its status block with \r between updates;
		// splitCROrLF makes each line within a block (and each block)
		// its own token.
		scan.Split(splitCROrLF)
		scanDDRescueOutput(scan, totalBytes, sink)
	})
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ddrescue: %w", err)
	}
	return nil
}

// scanDDRescueOutput parses ddrescue's multi-line status block and emits
// one Progress event per refresh once enough of the block has been seen.
// Separated from Copy for testability via ParseDDRescueLine.
func scanDDRescueOutput(scan *bufio.Scanner, totalBytes int64, sink Sink) {
	var pct float64
	var speed string
	var eta int
	var haveRescued bool
	for scan.Scan() {
		line := scan.Text()
		ev, ok := ParseDDRescueLine(line, totalBytes)
		if !ok {
			// Surface ddrescue's terminal-line messages so the user can see
			// what phase it's in ("Copying non-tried blocks...", etc.).
			lt := strings.TrimSpace(line)
			if strings.HasPrefix(lt, "GNU ddrescue") ||
				strings.HasPrefix(lt, "Copying ") ||
				strings.HasPrefix(lt, "Trimming ") ||
				strings.HasPrefix(lt, "Scraping ") ||
				strings.HasPrefix(lt, "Retrying ") ||
				strings.HasPrefix(lt, "Finished") {
				sink.Log(state.LogLevelInfo, "ddrescue: %s", lt)
			}
			continue
		}
		if ev.pct > 0 {
			pct = ev.pct
			haveRescued = true
		}
		if ev.speed != "" {
			speed = ev.speed
		}
		if ev.eta > 0 {
			eta = ev.eta
		}
		// Emit a Progress event when we have at least a rescued percentage.
		// Speed and ETA come from sibling lines in the same status block;
		// if they haven't landed yet we send what we have.
		if haveRescued {
			sink.Progress(pct, speed, eta)
		}
	}
}

// ddRescueEvent bundles the three datapoints we extract from a ddrescue
// status block. Empty/zero fields mean the corresponding line wasn't
// matched in the current scan.
type ddRescueEvent struct {
	pct   float64
	speed string
	eta   int
}

var (
	ddRescuePctRE = regexp.MustCompile(`pct rescued:\s+([\d.]+)%`)
	// ddrescue uses SI units: lowercase k for 1000, uppercase MGT for
	// 1e6 / 1e9 / 1e12. Accept both cases for k so a parser change in a
	// future version (or a locale tweak) doesn't silently drop matches.
	ddRescueRateRE   = regexp.MustCompile(`current rate:\s+([\d.]+\s*[kKMGT]?B/s)`)
	ddRescueETARE    = regexp.MustCompile(`remaining time:\s+([^,\n]+?)\s*(?:,|$)`)
	ddRescueRescued  = regexp.MustCompile(`rescued:\s+([\d.]+)\s*([kKMGT]?B)\b`)
	ddRescueOposByte = regexp.MustCompile(`opos:\s+([\d.]+)\s*([kKMGT]?B)`)
)

// ParseDDRescueLine extracts whatever fields are present on the line.
// Returns ok=false for lines that match nothing (so the caller can keep
// scanning the status block). When totalBytes > 0 and the line carries
// a rescued-bytes count but no explicit `pct rescued`, the percentage
// is computed against totalBytes as a fallback.
func ParseDDRescueLine(line string, totalBytes int64) (ddRescueEvent, bool) {
	ev := ddRescueEvent{}
	matched := false

	if m := ddRescuePctRE.FindStringSubmatch(line); m != nil {
		if v, err := strconv.ParseFloat(m[1], 64); err == nil {
			ev.pct = v
			matched = true
		}
	}
	if m := ddRescueRateRE.FindStringSubmatch(line); m != nil {
		ev.speed = strings.TrimSpace(m[1])
		matched = true
	}
	if m := ddRescueETARE.FindStringSubmatch(line); m != nil {
		raw := strings.TrimSpace(m[1])
		if raw != "" && raw != "n/a" {
			ev.eta = parseDDRescueDuration(raw)
		}
		matched = true
	}
	// Fallback for `pct rescued` not yet seen: derive from rescued+totalBytes.
	if ev.pct == 0 && totalBytes > 0 {
		if m := ddRescueRescued.FindStringSubmatch(line); m != nil {
			b := parseDDRescueBytes(m[1], m[2])
			if b > 0 {
				ev.pct = 100.0 * float64(b) / float64(totalBytes)
				matched = true
			}
		} else if m := ddRescueOposByte.FindStringSubmatch(line); m != nil {
			b := parseDDRescueBytes(m[1], m[2])
			if b > 0 {
				ev.pct = 100.0 * float64(b) / float64(totalBytes)
				matched = true
			}
		}
	}
	return ev, matched
}

// parseDDRescueDuration converts ddrescue's "2m 30s" / "1h 5m" / "15s"
// format into seconds. Returns 0 on parse failure.
func parseDDRescueDuration(s string) int {
	s = strings.TrimSpace(s)
	total := 0
	cur := ""
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			cur += string(r)
		case r == 'h':
			n, _ := strconv.Atoi(cur)
			total += n * 3600
			cur = ""
		case r == 'm':
			n, _ := strconv.Atoi(cur)
			total += n * 60
			cur = ""
		case r == 's':
			n, _ := strconv.Atoi(cur)
			total += n
			cur = ""
		}
	}
	return total
}

// parseDDRescueBytes converts ("1234", "MB") into bytes. ddrescue uses
// SI multipliers (kB=1000, MB=1e6, ...), matching its own display.
// Returns 0 on parse failure.
func parseDDRescueBytes(num, unit string) int64 {
	v, err := strconv.ParseFloat(num, 64)
	if err != nil {
		return 0
	}
	mult := int64(1)
	switch strings.ToUpper(unit) {
	case "B":
		mult = 1
	case "KB":
		mult = 1000
	case "MB":
		mult = 1000 * 1000
	case "GB":
		mult = 1000 * 1000 * 1000
	case "TB":
		mult = 1000 * 1000 * 1000 * 1000
	}
	return int64(v * float64(mult))
}
