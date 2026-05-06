package identify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// ErrNoTracks is returned when the disc has no usable tracks (data CD,
// blank media, or read failure).
var ErrNoTracks = errors.New("identify: no tracks on disc")

// cdParanoiaTOCReader shells out to `cdparanoia -Q` and parses its
// stderr (cdparanoia writes the TOC table there). cdparanoia exits
// non-zero when the disc isn't an audio CD; we surface that as
// ErrNoTracks for the classifier to interpret.
type cdParanoiaTOCReader struct {
	bin string
}

// NewCDParanoiaTOCReader returns a TOCReader that runs `<bin> -Q -d <devPath>`
// to read the TOC. bin is typically "cdparanoia" (resolved via PATH).
func NewCDParanoiaTOCReader(bin string) TOCReader {
	if bin == "" {
		bin = "cdparanoia"
	}
	return &cdParanoiaTOCReader{bin: bin}
}

func (r *cdParanoiaTOCReader) Read(ctx context.Context, devPath string) (*TOC, error) {
	cmd := exec.CommandContext(ctx, r.bin, "-Q", "-d", devPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// cdparanoia may exit non-zero even after printing a usable TOC
		// (e.g. it sometimes warns about "data" sectors at the lead-out
		// region). If the output parses cleanly, prefer that over the
		// exec error.
		toc, perr := ParseCDParanoiaQ(string(out))
		if perr == nil {
			return toc, nil
		}
		return nil, fmt.Errorf("cdparanoia: %w", err)
	}
	return ParseCDParanoiaQ(string(out))
}

// trackLineRegex matches a row like:
//
//	"  3.    25245 [05:36.45]    37020 [08:13.45]    no   no  2"
//
// Capturing: 1=number, 2=length, 3=begin
var trackLineRegex = regexp.MustCompile(`^\s*(\d+)\.\s+(\d+)\s+\[[^\]]+\]\s+(\d+)\s+\[`)

// ParseCDParanoiaQ parses the output of `cdparanoia -Q` into a TOC.
// Audio-only — cdparanoia doesn't expose data tracks. Returns
// ErrNoTracks when no rows parse.
func ParseCDParanoiaQ(s string) (*TOC, error) {
	var toc TOC
	for _, line := range strings.Split(s, "\n") {
		m := trackLineRegex.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		num, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		length, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		begin, err := strconv.Atoi(m[3])
		if err != nil {
			continue
		}
		toc.Tracks = append(toc.Tracks, Track{
			Number: num, StartLBA: begin, LengthLBA: length, IsAudio: true,
		})
	}
	if len(toc.Tracks) == 0 {
		return nil, ErrNoTracks
	}
	last := toc.Tracks[len(toc.Tracks)-1]
	toc.LeadoutLBA = last.StartLBA + last.LengthLBA
	return &toc, nil
}
