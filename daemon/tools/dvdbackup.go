package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
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

// Mirror runs `dvdbackup -M -i <devPath> -o <outDir> -p` to mirror
// the entire DVD-Video structure into outDir/<VOLUME_LABEL>/. Returns
// the path to the freshly-created `<VOLUME_LABEL>/VIDEO_TS` directory
// (HandBrake's `--input` for the transcode step). Sink receives
// per-VOB progress lines parsed from dvdbackup's stdout.
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

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); parseDVDBackupStream(stdout, sink) }()
	go func() { defer wg.Done(); parseDVDBackupStream(stderr, sink) }()
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return "", fmt.Errorf("dvdbackup: %w", err)
	}

	// dvdbackup writes outDir/<VOLUME_LABEL>/VIDEO_TS. The volume
	// label is the disc's own label, lifted by libdvdread; we
	// discover it by scanning outDir for the (sole) subdir that
	// contains a VIDEO_TS folder.
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

// parseDVDBackupStream consumes dvdbackup's textual output and turns
// "copying VTS_NN_NN.VOB" lines into sink log + progress events.
// dvdbackup doesn't emit a structured percentage, so progress is
// approximate (one tick per VOB) — good enough for the dashboard
// to show motion.
func parseDVDBackupStream(r io.Reader, sink Sink) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	vobs := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.Contains(line, ".VOB") {
			vobs++
			// VOBs cap at 1 GB each; dvdbackup writes them sequentially,
			// so each completed VOB is a coarse progress tick.
			sink.Progress(0, fmt.Sprintf("%dvob", vobs), 0)
		}
	}
}
