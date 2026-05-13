package tools

import (
	"bufio"
	"io"
	"strings"
	"testing"
)

// TestDrainAfterScan_DrainsLeftoverBytes is the regression for the
// 0.3.2 HandBrake-deadlock incident: the parse loop returned early
// (e.g. on ErrTooLong) and the pipe stopped being read. drainAfterScan
// must consume every remaining byte after the parse function returns,
// regardless of how it returned.
func TestDrainAfterScan_DrainsLeftoverBytes(t *testing.T) {
	// 200 KB of carriage-return-separated progress chunks — way past
	// any sensible scanner buffer cap if the parser had been left with
	// the default 64 KB limit and no drain.
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString("Encoding: task 1 of 1, 50.00 % (avg fps 30.0, ETA 00h00m10s)\r")
	}
	src := strings.NewReader(sb.String())

	scanned := 0
	drainAfterScan(src, func(s *bufio.Scanner) {
		// Simulate a parser that bails after one line — the rest must
		// still be drained.
		if s.Scan() {
			scanned++
		}
	})

	if scanned != 1 {
		t.Errorf("scan() should have returned one token, got %d", scanned)
	}
	// If drain didn't run, src would still have unread bytes — reading
	// it now should return 0 bytes + io.EOF.
	buf := make([]byte, 16)
	n, err := src.Read(buf)
	if n != 0 || err != io.EOF {
		t.Errorf("source not fully drained: n=%d err=%v", n, err)
	}
}

// TestSplitCROrLF_BothTerminators verifies HandBrake-style \r-only
// progress chunks become individual tokens.
func TestSplitCROrLF_BothTerminators(t *testing.T) {
	in := "first\rsecond\nthird\r\nfourth"
	scanner := bufio.NewScanner(strings.NewReader(in))
	scanner.Split(splitCROrLF)

	var got []string
	for scanner.Scan() {
		got = append(got, scanner.Text())
	}

	// "third\r\n" → "third" + "" (empty token between \r and \n)
	// "fourth"   → final token at EOF
	want := []string{"first", "second", "third", "", "fourth"}
	if len(got) != len(want) {
		t.Fatalf("token count: want %d, got %d (%v)", len(want), len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token[%d]: want %q, got %q", i, want[i], got[i])
		}
	}
}
