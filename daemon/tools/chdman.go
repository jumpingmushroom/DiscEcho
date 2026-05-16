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

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// CHDMan wraps the chdman binary (from MAME tools). Used by the PSX
// and PS2 pipelines for the compress step (rip output → CHD).
type CHDMan struct {
	bin string
}

// NewCHDMan returns a CHDMan. Empty bin defaults to "chdman".
func NewCHDMan(bin string) *CHDMan {
	if bin == "" {
		bin = "chdman"
	}
	return &CHDMan{bin: bin}
}

// Name returns the tool name. Used for logging only — CHDMan is not
// registered in tools.Registry (typed-deps pattern, same as Redumper
// and MakeMKV).
func (c *CHDMan) Name() string { return "chdman" }

// CreateCHD converts a disc image to CHD. Auto-detects the chdman
// subcommand from the input file extension:
//
//	.cue → chdman createcd  --input <input> --output <output>
//	.iso → chdman createraw --unitsize 2048 --hunksize 8192
//	                        --compression lzma,zlib
//	                        --input <input> --output <output>
//
// The .iso path uses createraw rather than createdvd because the
// dedicated `createdvd` subcommand was only added in MAME 0.252 (April
// 2023); Debian bookworm still ships mame-tools 0.251 and falls over
// with `unknown command` for createdvd. createraw with DVD-flavored
// flags (2048-byte sectors, 4-sector hunks, lzma+zlib) produces a CHD
// PCSX2 / RetroArch / libchdr can load just fine — the missing piece
// is only the DVD-typed CHD metadata block, which players don't
// actually need to mount the image.
//
// Streams "Compressing, X.X% complete..." lines to sink as Progress.
func (c *CHDMan) CreateCHD(ctx context.Context, input, output string, sink Sink) error {
	subcmd, err := chdmanSubcommandFor(input)
	if err != nil {
		return err
	}
	args := []string{subcmd, "--input", input, "--output", output}
	if subcmd == "createraw" {
		args = append(args,
			"--unitsize", "2048",
			"--hunksize", "8192",
			"--compression", "lzma,zlib",
		)
	}
	cmd := exec.CommandContext(ctx, c.bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("chdman start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ParseCHDManProgress(stdout, sink) }()
	go func() { defer wg.Done(); ParseCHDManProgress(stderr, sink) }()
	wg.Wait()

	return cmd.Wait()
}

func chdmanSubcommandFor(input string) (string, error) {
	switch strings.ToLower(filepath.Ext(input)) {
	case ".cue":
		return "createcd", nil
	case ".iso":
		// createraw with DVD-flavored flags (see CreateCHD doc).
		return "createraw", nil
	default:
		return "", fmt.Errorf("chdman: unknown input extension for %q (want .cue or .iso)", input)
	}
}

var chdmanProgressRE = regexp.MustCompile(`Compressing,\s+([0-9.]+)%\s+complete`)

// ParseCHDManProgress reads chdman output and emits sink.Progress on
// "Compressing, X.X% complete" lines. All other non-empty lines are
// forwarded to sink.Log so they appear in the job's log tail. The
// scanner treats both '\r' and '\n' as line terminators because chdman
// overwrites its progress line with carriage returns.
func ParseCHDManProgress(r io.Reader, sink Sink) {
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		scanner.Split(splitCROrLF)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			m := chdmanProgressRE.FindStringSubmatch(line)
			if m != nil {
				pct, err := strconv.ParseFloat(m[1], 64)
				if err != nil {
					continue
				}
				sink.Progress(pct, "", 0)
				continue
			}
			if len(line) > 400 {
				line = line[:400]
			}
			sink.Log(state.LogLevelInfo, "chdman: %s", line)
		}
	})
}
