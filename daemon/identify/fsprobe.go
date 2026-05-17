package identify

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// FSProber lists filenames present on an optical disc's ISO9660/UDF
// filesystem. Used by the classifier to distinguish DVD-Video (has
// /VIDEO_TS/) from BDMV (has /BDMV/index.bdmv) from generic DATA.
type FSProber interface {
	List(ctx context.Context, devPath string) ([]string, error)
}

// FSProberConfig configures NewFSProber.
type FSProberConfig struct {
	IsoInfoBin string // default "isoinfo"
}

// NewFSProber returns an FSProber that shells out to `isoinfo -R -l`.
func NewFSProber(c FSProberConfig) FSProber {
	if c.IsoInfoBin == "" {
		c.IsoInfoBin = "isoinfo"
	}
	return &isoinfoFSProber{bin: c.IsoInfoBin}
}

type isoinfoFSProber struct{ bin string }

// perCallIsoinfoTimeout caps each isoinfo invocation so a single hang
// on a finicky disc can't eat the caller's whole deadline. The retry
// decorator (retryingFSProber) then moves on to the next attempt after
// its own backoff, instead of being stuck on one slow call.
const perCallIsoinfoTimeout = 20 * time.Second

func (p *isoinfoFSProber) List(ctx context.Context, devPath string) ([]string, error) {
	cctx, cancel := context.WithTimeout(ctx, perCallIsoinfoTimeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, p.bin, "-R", "-l", "-i", devPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("isoinfo -R -l: %w", err)
	}
	return ParseIsoInfoListing(string(out)), nil
}

// ParseIsoInfoListing flattens the ls -lR-style output of `isoinfo -R -l`
// into absolute paths. Directory headers ("Directory listing of /BDMV/")
// fix the current path; entries inside contribute one path each. Trailing
// ";N" version suffixes are stripped (ISO9660 file-version artefacts) and
// trailing "/" on directory entries is dropped before recording.
//
// Returns a deduplicated slice in encounter order.
func ParseIsoInfoListing(s string) []string {
	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Buffer(make([]byte, 4096), 64*1024)

	var (
		current = "/"
		paths   []string
		seen    = map[string]bool{}
	)
	add := func(p string) {
		if seen[p] {
			return
		}
		seen[p] = true
		paths = append(paths, p)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Directory listing of ") {
			current = strings.TrimSuffix(strings.TrimPrefix(line, "Directory listing of "), "/")
			if current == "" {
				current = "/"
			}
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Entry rows look like:
		//   d---------   0    0    0    2048 Jan  1 1970 [   23 02]  BDMV/
		//   ----------   0    0    0   12288 Jan  1 1970 [   23 04]  index.bdmv;1
		// Filename sits after the closing "]" of the LBA bracket.
		idx := strings.LastIndex(line, "]")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(line[idx+1:])
		if name == "" {
			continue
		}
		if semi := strings.LastIndex(name, ";"); semi > 0 {
			name = name[:semi]
		}
		name = strings.TrimSuffix(name, "/")
		if name == "" {
			continue
		}
		if name == "." || name == ".." {
			continue // ISO9660 self/parent dir entries — never a useful path
		}
		var full string
		if current == "/" {
			full = "/" + name
		} else {
			full = current + "/" + name
		}
		add(full)
	}
	return paths
}
