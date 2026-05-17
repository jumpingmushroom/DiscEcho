package tools

import (
	"bufio"
	"fmt"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

type recordingSink struct {
	events []string
}

func (r *recordingSink) Progress(pct float64, speed string, eta int) {
	r.events = append(r.events, fmt.Sprintf("PROGRESS pct=%.2f speed=%q eta=%d", pct, speed, eta))
}

func (r *recordingSink) Log(_ state.LogLevel, format string, args ...any) {
	r.events = append(r.events, fmt.Sprintf("LOG "+format, args...))
}

func (r *recordingSink) SubStep(name string) {
	r.events = append(r.events, "SUBSTEP "+name)
}

// Captured ddrescue 1.27 stderr from a real rip in progress. Each ANSI
// cursor-up (\x1b[A) is included verbatim; the splitter ignores them.
const ddrescueSample = "GNU ddrescue 1.27\n" +
	"About to copy 686921 kBytes from '/dev/sr0' to '/tmp/test.iso'\n" +
	"    Starting positions: infile = 0 B,  outfile = 0 B\n" +
	"    Copy block size:  32 sectors       Initial skip size: 32 sectors\n" +
	"Sector size: 2048 Bytes\n\n" +
	"Press Ctrl-C to interrupt\n\n\n\n\n\n\n" +
	"\x1b[A\x1b[A\x1b[A\x1b[A\x1b[A\x1b[A" +
	"     ipos:        0 B, non-trimmed:        0 B,  current rate:       0 B/s\n" +
	"     opos:        0 B, non-scraped:        0 B,  average rate:       0 B/s\n" +
	"non-tried:  686921 kB,  bad-sector:        0 B,    error rate:       0 B/s\n" +
	"  rescued:        0 B,   bad areas:        0,        run time:          0s\n" +
	"pct rescued:    0.00%, read errors:        0,  remaining time:         n/a\n" +
	"                              time since last successful read:         n/a\n" +
	"Copying non-tried blocks... Pass 1 (forwards)" +
	"\x1b[A\x1b[A\x1b[A\x1b[A\x1b[A\x1b[A" +
	"     ipos:   262144 B, non-trimmed:        0 B,  current rate:    196 kB/s\n" +
	"     opos:   262144 B, non-scraped:        0 B,  average rate:    131 kB/s\n" +
	"non-tried:  686659 kB,  bad-sector:        0 B,    error rate:       0 B/s\n" +
	"  rescued:   262144 B,   bad areas:        0,        run time:          2s\n" +
	"pct rescued:    0.03%, read errors:        0,  remaining time:      1h 27m\n" +
	"                              time since last successful read:          0s\n" +
	"Copying non-tried blocks... Pass 1 (forwards)"

func TestScanDDRescueOutput_EmitsProgress(t *testing.T) {
	sink := &recordingSink{}
	scan := bufio.NewScanner(strings.NewReader(ddrescueSample))
	scan.Buffer(make([]byte, 4096), 1024*1024)
	scan.Split(splitCROrLF)
	scanDDRescueOutput(scan, 686_921_728, sink)

	if len(sink.events) == 0 {
		t.Fatal("no events emitted")
	}
	var sawProgress bool
	for _, ev := range sink.events {
		t.Log(ev)
		if strings.HasPrefix(ev, "PROGRESS pct=0.03") {
			sawProgress = true
		}
	}
	if !sawProgress {
		t.Errorf("expected a PROGRESS event with pct=0.03 from the second status block")
	}
}
