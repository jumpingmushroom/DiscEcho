package tools

import (
	"bufio"
	"io"
)

// drainAfterScan runs scan against r, then unconditionally copies any
// remaining bytes from r to io.Discard. This is the critical part:
// every parser we plug into a subprocess StdoutPipe / StderrPipe must
// keep reading the pipe until EOF, otherwise the subprocess blocks on
// pipe_write once the kernel pipe buffer (default 64 KB) fills up.
//
// The encode-progress hang we hit in 0.3.2 was exactly this — HandBrake
// emits long carriage-return-separated progress strings that pushed
// bufio.Scanner past its 64 KB buffer cap, the parse goroutine exited
// on bufio.ErrTooLong, the pipe stopped being drained, and HandBrake
// deadlocked on its next status write while the encoded file sat
// finalised but with the moov-atom rewrite (--optimize) blocked
// mid-stream.
//
// Callers pass a scan function that contains their parsing loop
// (typically `for s.Scan() { ... }`); they can break out of it on
// errors. The drain runs regardless of whether scan returned cleanly.
func drainAfterScan(r io.Reader, scan func(*bufio.Scanner)) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	scan(scanner)
	_, _ = io.Copy(io.Discard, r)
}

// splitCROrLF is a bufio.SplitFunc that treats both '\r' and '\n' as
// line terminators. Tools that drive interactive progress meters
// (HandBrakeCLI, redumper, makemkvcon to a lesser extent) emit lines
// separated by '\r' so the terminal overstrikes them in place; the
// default bufio.ScanLines treats '\r' as ordinary content, so a long
// run of \r-separated progress chunks accumulates as a single mega-
// token until the scanner buffer cap, then the scanner aborts.
//
// Using this splitter, each progress chunk becomes its own line that
// the per-tool regex can match and turn into a Sink.Progress event.
func splitCROrLF(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	for i, b := range data {
		if b == '\n' || b == '\r' {
			return i + 1, data[:i], nil
		}
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
