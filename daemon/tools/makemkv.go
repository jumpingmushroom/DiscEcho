package tools

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// MakeMKVTitle is one title from a `makemkvcon info` scan.
type MakeMKVTitle struct {
	ID          int
	DurationSec int
	SizeBytes   int64
	Chapters    int
	SourceFile  string
	Tracks      []MakeMKVTrack
}

// MakeMKVTrack is one stream within a title (video, audio, or subs).
type MakeMKVTrack struct {
	Index   int
	Type    string // "Video" | "Audio" | "Subtitles"
	Codec   string
	Lang    string // ISO 639-2; empty for video
	Default bool
	Forced  bool
}

// MakeMKV wraps the makemkvcon binary. Exposes typed Scan + Rip methods
// rather than implementing tools.Tool — handlers receive MakeMKV via
// typed deps, not via tools.Registry lookup.
type MakeMKV struct {
	bin     string
	dataDir string
}

// NewMakeMKV returns a MakeMKV tool. Empty bin defaults to "makemkvcon"
// (resolved via PATH); empty dataDir lets makemkvcon use its own
// default of $HOME/.MakeMKV.
func NewMakeMKV(bin, dataDir string) *MakeMKV {
	if bin == "" {
		bin = "makemkvcon"
	}
	return &MakeMKV{bin: bin, dataDir: dataDir}
}

// Name returns the tool name. Used for logging only — MakeMKV is not
// registered in tools.Registry.
func (m *MakeMKV) Name() string { return "makemkv" }

// Scan runs `makemkvcon -r --cache=1 info dev:<devPath>` and parses
// the output into titles.
func (m *MakeMKV) Scan(ctx context.Context, devPath string) ([]MakeMKVTitle, error) {
	args := []string{"-r", "--cache=1", "info", "dev:" + devPath}
	cmd := exec.CommandContext(ctx, m.bin, args...)
	if m.dataDir != "" {
		cmd.Env = append(cmd.Environ(), "HOME="+m.dataDir+"/..")
	}
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("makemkvcon info: %w", err)
	}
	return ParseMakeMKVInfo(string(out))
}

// Rip runs `makemkvcon -r --decrypt --noscan mkv dev:<devPath> <titleID> <outDir>`
// and streams progress to the sink. outDir gets one .mkv per title
// ripped (we always rip a single title, so one file).
func (m *MakeMKV) Rip(ctx context.Context, devPath string, titleID int, outDir string, sink Sink) error {
	args := []string{
		"-r", "--decrypt", "--noscan",
		"mkv", "dev:" + devPath, strconv.Itoa(titleID), outDir,
	}
	cmd := exec.CommandContext(ctx, m.bin, args...)
	if m.dataDir != "" {
		cmd.Env = append(cmd.Environ(), "HOME="+m.dataDir+"/..")
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
		return fmt.Errorf("makemkvcon start: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); ParseMakeMKVProgressStream(stdout, sink) }()
	go func() { defer wg.Done(); ParseMakeMKVProgressStream(stderr, sink) }()
	wg.Wait()

	return cmd.Wait()
}

// ParseMakeMKVInfo parses the robot-mode output of `makemkvcon info`
// into a slice of titles. Returns an error only on empty input. Lines
// that don't match the expected shape are ignored.
//
// Recognised codes (TINFO):
//
//	8  → chapters count
//	9  → duration "HH:MM:SS"
//	11 → size in bytes
//	27 → source filename (e.g. "00800.mpls")
//
// Recognised codes (SINFO):
//
//	1  → type ("Video" | "Audio" | "Subtitles")
//	3  → language code (ISO 639-2)
//	6  → codec
//	28 → "default" if flagged default
//	30 → "forced" if flagged forced
func ParseMakeMKVInfo(s string) ([]MakeMKVTitle, error) {
	if strings.TrimSpace(s) == "" {
		return nil, fmt.Errorf("ParseMakeMKVInfo: empty input")
	}

	titles := map[int]*MakeMKVTitle{}
	tracks := map[int]map[int]*MakeMKVTrack{} // titleID → trackIdx → track

	scanner := bufio.NewScanner(strings.NewReader(s))
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "TINFO:"):
			parseTINFO(strings.TrimPrefix(line, "TINFO:"), titles)
		case strings.HasPrefix(line, "SINFO:"):
			parseSINFO(strings.TrimPrefix(line, "SINFO:"), tracks)
		}
	}

	// Merge tracks into titles, sorted by index.
	for tid, perTrack := range tracks {
		t := titles[tid]
		if t == nil {
			continue
		}
		idxs := make([]int, 0, len(perTrack))
		for i := range perTrack {
			idxs = append(idxs, i)
		}
		slices.Sort(idxs)
		for _, i := range idxs {
			t.Tracks = append(t.Tracks, *perTrack[i])
		}
	}

	out := make([]MakeMKVTitle, 0, len(titles))
	tids := make([]int, 0, len(titles))
	for id := range titles {
		tids = append(tids, id)
	}
	slices.Sort(tids)
	for _, id := range tids {
		out = append(out, *titles[id])
	}
	return out, nil
}

// ParseMakeMKVProgressStream reads PRGV/PRGC lines and emits sink
// events. PRGV → progress percent. PRGC → log line with the operation
// label ("Saving to MKV file"). PRGT is ignored (mirrors PRGC for our
// single-title rips).
func ParseMakeMKVProgressStream(r io.Reader, sink Sink) {
	drainAfterScan(r, func(scanner *bufio.Scanner) {
		for scanner.Scan() {
			line := scanner.Text()
			switch {
			case strings.HasPrefix(line, "PRGV:"):
				parts := strings.Split(strings.TrimPrefix(line, "PRGV:"), ",")
				if len(parts) < 3 {
					continue
				}
				cur, _ := strconv.Atoi(parts[0])
				max, _ := strconv.Atoi(parts[2])
				if max <= 0 {
					continue
				}
				pct := float64(cur) / float64(max) * 100
				sink.Progress(pct, "", 0)
			case strings.HasPrefix(line, "PRGC:"):
				label := unquoteMakeMKVLast(strings.TrimPrefix(line, "PRGC:"))
				if label != "" {
					// Pass the label as a %s argument, not as the format
					// string. Production sinks call fmt.Sprintf on the
					// format, so a literal '%' in a future MakeMKV label
					// would render as %!<verb>(MISSING) and corrupt logs.
					sink.Log(state.LogLevelInfo, "%s", label)
				}
			}
		}
	})
}

// parseTINFO splits a TINFO row payload (the part after "TINFO:") and
// updates the titles map. Format: `tid,id,code,"value"`.
func parseTINFO(payload string, titles map[int]*MakeMKVTitle) {
	parts := splitMakeMKVRow(payload, 4)
	if len(parts) < 4 {
		return
	}
	tid, _ := strconv.Atoi(parts[0])
	id, _ := strconv.Atoi(parts[1])
	val := parts[3]
	t := titles[tid]
	if t == nil {
		t = &MakeMKVTitle{ID: tid}
		titles[tid] = t
	}
	switch id {
	case 8:
		t.Chapters, _ = strconv.Atoi(val)
	case 9:
		t.DurationSec = parseDurationHHMMSS(val)
	case 11:
		t.SizeBytes, _ = strconv.ParseInt(val, 10, 64)
	case 27:
		t.SourceFile = val
	}
}

func parseSINFO(payload string, tracks map[int]map[int]*MakeMKVTrack) {
	parts := splitMakeMKVRow(payload, 5)
	if len(parts) < 5 {
		return
	}
	tid, _ := strconv.Atoi(parts[0])
	sid, _ := strconv.Atoi(parts[1])
	id, _ := strconv.Atoi(parts[2])
	val := parts[4]
	if _, ok := tracks[tid]; !ok {
		tracks[tid] = map[int]*MakeMKVTrack{}
	}
	tr := tracks[tid][sid]
	if tr == nil {
		tr = &MakeMKVTrack{Index: sid}
		tracks[tid][sid] = tr
	}
	switch id {
	case 1:
		tr.Type = val
	case 3:
		tr.Lang = val
	case 6:
		tr.Codec = val
	case 28:
		tr.Default = val != ""
	case 30:
		tr.Forced = val != ""
	}
}

// splitMakeMKVRow splits a comma-separated robot-mode row, respecting
// double-quotes around the trailing value field. n is the number of
// fields expected. Returns nil when the row doesn't have at least n-1
// commas before the (possibly-quoted) last field.
func splitMakeMKVRow(row string, n int) []string {
	out := make([]string, 0, n)
	for i := 0; i < n-1; i++ {
		comma := strings.Index(row, ",")
		if comma < 0 {
			return nil
		}
		out = append(out, row[:comma])
		row = row[comma+1:]
	}
	if strings.HasPrefix(row, `"`) {
		end := strings.LastIndex(row, `"`)
		if end <= 0 {
			out = append(out, row)
		} else {
			out = append(out, row[1:end])
		}
	} else {
		out = append(out, row)
	}
	return out
}

// unquoteMakeMKVLast extracts the final quoted field from a robot-mode
// row payload. Used for PRGC/PRGT labels: `5018,0,"Saving to MKV file"`.
func unquoteMakeMKVLast(s string) string {
	idx := strings.Index(s, `"`)
	if idx < 0 {
		return ""
	}
	end := strings.LastIndex(s, `"`)
	if end <= idx {
		return ""
	}
	return s[idx+1 : end]
}

func parseDurationHHMMSS(s string) int {
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0
	}
	h, _ := strconv.Atoi(parts[0])
	m, _ := strconv.Atoi(parts[1])
	sec, _ := strconv.Atoi(parts[2])
	return h*3600 + m*60 + sec
}
