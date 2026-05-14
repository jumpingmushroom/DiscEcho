package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// DVDBackup wraps the dvdbackup binary. It mirrors a DVD's VIDEO_TS
// structure to a local directory using libdvdread + libdvdcss, with
// no registration / beta-key overhead. The DVD pipeline calls Mirror
// in the rip step and then hands the resulting VIDEO_TS path to
// HandBrake for the scan + transcode steps.
type DVDBackup struct {
	bin string
}

// NewDVDBackup returns a DVDBackup tool. Empty bin defaults to
// "dvdbackup" (resolved via PATH).
func NewDVDBackup(bin string) *DVDBackup {
	if bin == "" {
		bin = "dvdbackup"
	}
	return &DVDBackup{bin: bin}
}

// Name returns the tool name. Used for logging only — DVDBackup is
// not registered in tools.Registry.
func (d *DVDBackup) Name() string { return "dvdbackup" }

// progressPollInterval is the cadence at which Mirror polls the
// workdir size to compute and emit progress. dvdbackup itself doesn't
// publish a structured percentage, so we derive one from
// (bytes-written / disc-total) using /sys/block/<dev>/size as the
// denominator. Polling every 2 s gives smooth motion without
// hammering the filesystem.
var progressPollInterval = 2 * time.Second

// Mirror runs `dvdbackup -M -i <devPath> -o <outDir> -p` to mirror
// the entire DVD-Video structure into outDir/<VOLUME_LABEL>/. Returns
// the path to the freshly-created `<VOLUME_LABEL>/VIDEO_TS` parent
// directory (HandBrake's `--input` for the transcode step).
//
// Progress is emitted to sink as a true percentage derived from the
// running sum of bytes written under outDir vs. the disc's total
// size (read once from /sys/block/<dev>/size). dvdbackup's textual
// stdout is also scanned so each VOB-mention becomes a coarse
// log/speed tick — useful when the size-based path can't read
// /sys/block (e.g. devPath that's not a block device).
func (d *DVDBackup) Mirror(ctx context.Context, devPath, outDir string, sink Sink) (string, error) {
	if devPath == "" {
		return "", errors.New("dvdbackup: empty devPath")
	}
	if outDir == "" {
		return "", errors.New("dvdbackup: empty outDir")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("dvdbackup mkdir: %w", err)
	}

	totalBytes, totalErr := discTotalBytes(devPath)
	// totalErr is non-fatal — we just fall back to VOB-tick-only progress.

	args := []string{"-M", "-i", devPath, "-o", outDir, "-p"}
	cmd := exec.CommandContext(ctx, d.bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("dvdbackup start: %w", err)
	}

	pollCtx, cancelPoll := context.WithCancel(ctx)
	defer cancelPoll()

	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); parseDVDBackupStream(stdout, sink) }()
	go func() { defer wg.Done(); parseDVDBackupStream(stderr, sink) }()
	go func() { defer wg.Done(); pollProgress(pollCtx, outDir, totalBytes, sink) }()

	waitErr := cmd.Wait()
	cancelPoll() // stop the progress poll before we close the streams
	wg.Wait()

	if waitErr != nil {
		return "", fmt.Errorf("dvdbackup: %w", waitErr)
	}

	// Final snap to 100% — the size-based poll caps at 99 % to leave
	// headroom for sector-alignment slack.
	if totalErr == nil && totalBytes > 0 {
		sink.Progress(100, formatSize(totalBytes), 0)
	}

	videoTS, err := findVideoTSDir(outDir)
	if err != nil {
		return "", err
	}
	return videoTS, nil
}

func findVideoTSDir(outDir string) (string, error) {
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return "", fmt.Errorf("dvdbackup: read %s: %w", outDir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(outDir, e.Name())
		if st, err := os.Stat(filepath.Join(candidate, "VIDEO_TS")); err == nil && st.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("dvdbackup: no VIDEO_TS produced under %s", outDir)
}

// discTotalBytes returns the total size of the block device, read
// from /sys/block/<name>/size (which is in 512-byte sectors). Returns
// 0 / error if devPath isn't a block device or sysfs isn't mounted.
func discTotalBytes(devPath string) (int64, error) {
	name := filepath.Base(devPath)
	sysPath := filepath.Join("/sys/block", name, "size")
	body, err := os.ReadFile(sysPath) // #nosec G304 -- path built from /dev/<basename>
	if err != nil {
		return 0, err
	}
	sectors, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", sysPath, err)
	}
	return sectors * 512, nil
}

// pollProgress samples the running size of outDir until ctx is
// cancelled (i.e. dvdbackup has exited). totalBytes is the disc-side
// reference; with 0 / unknown we still emit a speed reading but no
// percentage.
func pollProgress(ctx context.Context, outDir string, totalBytes int64, sink Sink) {
	tick := time.NewTicker(progressPollInterval)
	defer tick.Stop()

	type sample struct {
		t     time.Time
		bytes int64
	}
	const windowSec = 10
	var window []sample

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}

		written, err := dirSize(outDir)
		if err != nil {
			continue
		}
		now := time.Now()
		window = append(window, sample{t: now, bytes: written})
		// trim window to last `windowSec` seconds
		cutoff := now.Add(-windowSec * time.Second)
		for len(window) > 1 && window[0].t.Before(cutoff) {
			window = window[1:]
		}

		var speed string
		var eta int
		if len(window) >= 2 {
			d := window[len(window)-1].bytes - window[0].bytes
			t := window[len(window)-1].t.Sub(window[0].t).Seconds()
			if t > 0 {
				bps := float64(d) / t
				speed = formatRate(bps)
				if totalBytes > 0 && bps > 0 {
					remaining := totalBytes - written
					if remaining < 0 {
						remaining = 0
					}
					eta = int(float64(remaining) / bps)
				}
			}
		}
		if speed == "" {
			speed = formatSize(written)
		}

		var pct float64
		if totalBytes > 0 {
			pct = float64(written) / float64(totalBytes) * 100
			// Cap at 99 % until dvdbackup exits — the disc-total
			// always overshoots the actual VIDEO_TS bytes because of
			// non-DVD-Video tracks, sector alignment, and trailing
			// zero-fill. We snap to 100 % in Mirror after Wait().
			if pct > 99 {
				pct = 99
			}
		}
		sink.Progress(pct, speed, eta)
	}
}

// dirSize sums the sizes of all regular files under root. Symlinks
// and special files are skipped.
func dirSize(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// A transient ENOENT can happen while dvdbackup is
			// creating subdirs; skip and keep walking.
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			if errors.Is(ierr, fs.ErrNotExist) {
				return nil
			}
			return ierr
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func formatSize(b int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)
	switch {
	case b >= GB:
		return fmt.Sprintf("%.1fGB", float64(b)/float64(GB))
	case b >= MB:
		return fmt.Sprintf("%.0fMB", float64(b)/float64(MB))
	case b >= KB:
		return fmt.Sprintf("%.0fKB", float64(b)/float64(KB))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatRate(bps float64) string {
	const (
		KB = 1024.0
		MB = KB * 1024
	)
	switch {
	case bps >= MB:
		return fmt.Sprintf("%.1fMB/s", bps/MB)
	case bps >= KB:
		return fmt.Sprintf("%.0fKB/s", bps/KB)
	default:
		return fmt.Sprintf("%.0fB/s", bps)
	}
}

// dvdBackupProgressRE matches dvdbackup's `-p` progress chatter
// ("Copying menu: 9% done (1/11 MiB)", "Copying Title, part 1/1: 2%
// done ..."). These overstrike the terminal with '\r' and carry no
// information the size-based poller doesn't already surface.
var dvdBackupProgressRE = regexp.MustCompile(`% done`)

// dvdBackupErrorKeywords flag a line as a genuine warning rather than
// routine chatter. Matched case-insensitively as substrings.
var dvdBackupErrorKeywords = []string{
	"error", "cannot", "could not", "couldn't",
	"failed", "no such", "unable", "permission denied",
}

// hasErrorKeyword reports whether line looks like an actual error or
// failure rather than informational tool output.
func hasErrorKeyword(line string) bool {
	l := strings.ToLower(line)
	for _, kw := range dvdBackupErrorKeywords {
		if strings.Contains(l, kw) {
			return true
		}
	}
	return false
}

// parseDVDBackupStream consumes dvdbackup's textual output (stdout and
// stderr). Progress is handled separately by the size-based poller, so
// '% done' chatter is dropped; libdvdread's per-VOB key/seek trace is
// likewise dropped unless it carries an error keyword. Remaining lines
// are forwarded at info level, escalated to warn only when they look
// like a real error. splitCROrLF keeps dvdbackup's '\r'-overstrike
// progress from accumulating into multi-KB scanner tokens. Capped at
// 200 forwarded lines per stream to keep the SQLite log_lines ring
// bounded — exceeding the cap emits a single marker.
func parseDVDBackupStream(r io.Reader, sink Sink) {
	const lineCap = 200
	seen := 0
	capWarned := false

	drainAfterScan(r, func(scanner *bufio.Scanner) {
		scanner.Split(splitCROrLF)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			if dvdBackupProgressRE.MatchString(line) {
				continue
			}
			if strings.HasPrefix(line, "libdvdread:") && !hasErrorKeyword(line) {
				continue
			}
			if seen >= lineCap {
				if !capWarned {
					sink.Log(state.LogLevelWarn, "dvdbackup: log cap reached, dropping further lines")
					capWarned = true
				}
				continue
			}
			seen++
			level := state.LogLevelInfo
			if hasErrorKeyword(line) {
				level = state.LogLevelWarn
			}
			sink.Log(level, "dvdbackup: %s", line)
		}
	})
}
