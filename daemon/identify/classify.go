package identify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// cdInfoRunner runs the cd-info binary and returns its combined output.
// Exposed for tests; real callers use defaultCDInfoRunner.
type cdInfoRunner func(ctx context.Context, bin, devPath string) ([]byte, error)

// cdInfoDiscModePrefix is the prefix the runner watches for to decide
// it has enough output to stop cd-info early. The classifier only needs
// the disc-mode line; everything after it (track listing, MCN, ISRC,
// CD-Text) is discarded by ClassifyFromCDInfo. We wait for the full
// line — prefix plus value plus newline — because cd-info on some drives
// flushes the label and the value (`CD-DA`, `Mode 2`, etc.) in separate
// writes, and killing in between leaves the parser without the value.
var cdInfoDiscModePrefix = []byte("Disc mode is listed as:")

// errCDInfoDiscNotReady is returned by defaultCDInfoRunner when cd-info
// completed a run but the disc wasn't ready: either the disc-mode line
// never landed, or it landed with a value matching an error string
// (e.g. `Error in getting information` on the ASUS SDRW-08D2S-U when
// the TOC read races the spin-up). The runCDInfo retry loop treats this
// as a transient failure and re-invokes cd-info on the existing
// spin-up backoff schedule. Without this, cd-info's exit code 0 on
// these aborted runs would short-circuit the retry loop and feed
// garbage to the parser.
var errCDInfoDiscNotReady = errors.New("cd-info: disc not ready (no valid disc-mode line)")

// cdInfoErrorMarkers lists strings that, when they appear as the
// disc-mode value in cd-info's output, indicate the run aborted before
// the drive could answer. They're treated as a retry trigger.
var cdInfoErrorMarkers = []string{
	"error in getting information",
	"unknown",
}

// defaultCDInfoRunner runs cd-info and kills it the moment the disc-mode
// line lands in stdout. Some drive+disc combinations hang cd-info for
// 60–90 s reading the optional Media Catalog Number that follows the
// track listing, even though the disc itself is fine — the kernel
// returns "ILLEGAL MODE FOR THIS TRACK" sense for the MCN probe and the
// drive retries internally. The classifier has everything it needs as
// soon as cd-info prints "Disc mode is listed as: …", so we don't wait
// for the rest of the run.
func defaultCDInfoRunner(ctx context.Context, bin, devPath string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, bin, devPath) //nolint:gosec // bin path is configured by the operator.
	w := newCDInfoWatcher(cdInfoDiscModePrefix)
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		// cd-info exited on its own. Some drives report a clean exit
		// (status 0) even when the TOC read failed and the disc-mode
		// line came back as an error string; runCDInfo's retry loop
		// needs a non-nil error to trigger the spin-up backoff, so we
		// upgrade "exited clean but no usable disc-mode line" into
		// errCDInfoDiscNotReady.
		if err == nil && !w.Fired() {
			return w.Bytes(), errCDInfoDiscNotReady
		}
		return w.Bytes(), err
	case <-w.Found():
		_ = cmd.Process.Kill()
		<-done
		return w.Bytes(), nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		return w.Bytes(), ctx.Err()
	}
}

// cdInfoWatcher is a thread-safe io.Writer that closes a channel the
// first time a complete newline-terminated line starting with `prefix`
// (with a non-empty value after the prefix) lands in the accumulated
// buffer. Used by defaultCDInfoRunner to drop cd-info as soon as the
// disc-mode line is fully written — we only close `found` after the
// line's trailing `\n` so a partial flush like `Disc mode is listed as:`
// without its value (`CD-DA`, `Mode 2`, …) doesn't fire the kill.
type cdInfoWatcher struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	line   bytes.Buffer // accumulates the current in-progress line
	prefix []byte
	found  chan struct{}
	once   sync.Once
}

func newCDInfoWatcher(prefix []byte) *cdInfoWatcher {
	return &cdInfoWatcher{prefix: prefix, found: make(chan struct{})}
}

func (w *cdInfoWatcher) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	if err != nil {
		return n, err
	}
	for _, b := range p {
		if b == '\n' {
			line := w.line.Bytes()
			if bytes.HasPrefix(line, w.prefix) {
				value := bytes.TrimSpace(line[len(w.prefix):])
				if len(value) > 0 && !looksLikeCDInfoError(value) {
					w.once.Do(func() { close(w.found) })
				}
			}
			w.line.Reset()
		} else {
			w.line.WriteByte(b)
		}
	}
	return n, nil
}

// Fired reports whether the disc-mode line has been observed (with a
// non-error value). Safe to call from any goroutine. Used by
// defaultCDInfoRunner to upgrade a clean-but-incomplete cd-info run
// into a retry-triggering error.
func (w *cdInfoWatcher) Fired() bool {
	select {
	case <-w.found:
		return true
	default:
		return false
	}
}

// looksLikeCDInfoError matches disc-mode values that cd-info writes
// when it couldn't read the TOC. Catches "Error in getting information"
// (the ASUS SDRW-08D2S-U spin-up race) and "Unknown" (a few other
// drives' transient state). Real disc modes — CD-DA, CD-ROM, CD-ROM XA,
// Mode 2, … — are short and never contain these strings.
func looksLikeCDInfoError(value []byte) bool {
	low := bytes.ToLower(value)
	for _, marker := range cdInfoErrorMarkers {
		if bytes.Contains(low, []byte(marker)) {
			return true
		}
	}
	return false
}

func (w *cdInfoWatcher) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}

func (w *cdInfoWatcher) Found() <-chan struct{} { return w.found }

// cdInfoBackoff is the wait between cd-info retries. Exposed for tests
// so the table-driven retry test can shrink it to a few microseconds
// without coupling to the production schedule.
var cdInfoBackoff = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	2 * time.Second,
	2 * time.Second,
	2 * time.Second,
	2 * time.Second,
}

// ErrUnknownDiscType is returned when none of the probes recognise the
// disc. The orchestrator turns this into a job-state failure with a
// clear "unsupported disc" message.
var ErrUnknownDiscType = errors.New("identify: unknown disc type")

// Classifier probes a drive and returns its disc type.
type Classifier interface {
	Classify(ctx context.Context, devPath string) (state.DiscType, error)
}

// SaturnProber probes a CD device for Saturn IP.BIN at sector 0.
type SaturnProber interface {
	Probe(ctx context.Context, devPath string) (*SaturnInfo, error)
}

// XboxProber probes a mounted Xbox disc for the XBE certificate.
type XboxProber interface {
	Probe(ctx context.Context, mountPoint string) (*XboxInfo, error)
}

// DCProber probes a CD device's TOC for the multi-session layout that
// indicates a Dreamcast GD-ROM.
type DCProber interface {
	Probe(ctx context.Context, devPath string) (bool, error)
}

// ClassifierConfig configures NewClassifier. FSProber, BDProber, and
// SystemCNFProber are optional dependencies — when nil, the classifier
// defaults to using NewFSProber / NewBDProber / NewSystemCNFProber with
// default binaries. SaturnProber, XboxProber, and DCProber are optional;
// when nil those branches are skipped.
type ClassifierConfig struct {
	CDInfoBin       string          // default "cd-info"
	FSProber        FSProber        // default NewFSProber({}) — distinguishes DVD/BDMV/DATA
	BDProber        BDProber        // default NewBDProber({}) — distinguishes BDMV vs UHD
	SystemCNFProber SystemCNFProber // default NewSystemCNFProber("") — distinguishes PSX vs PS2
	SaturnProber    SaturnProber    // optional — detects Sega Saturn via IP.BIN
	XboxProber      XboxProber      // optional — detects Xbox via default.xbe
	DCProber        DCProber        // optional — detects Dreamcast via GD-ROM TOC heuristic

	// CDInfoBackoff overrides the per-attempt wait schedule between
	// retries. nil → use the production schedule (~13 s total). A
	// non-nil empty slice disables retries entirely (useful in tests).
	CDInfoBackoff []time.Duration
}

// NewClassifier returns a Classifier that runs the multi-probe pipeline.
func NewClassifier(c ClassifierConfig) Classifier {
	if c.CDInfoBin == "" {
		c.CDInfoBin = "cd-info"
	}
	if c.FSProber == nil {
		c.FSProber = NewFSProber(FSProberConfig{})
	}
	if c.BDProber == nil {
		c.BDProber = NewBDProber(BDProberConfig{})
	}
	if c.SystemCNFProber == nil {
		c.SystemCNFProber = NewSystemCNFProber("")
	}
	backoff := cdInfoBackoff
	if c.CDInfoBackoff != nil {
		backoff = c.CDInfoBackoff
	}
	return &multiProbeClassifier{
		cdInfoBin: c.CDInfoBin,
		fs:        c.FSProber,
		bd:        c.BDProber,
		sysCNF:    c.SystemCNFProber,
		saturn:    c.SaturnProber,
		xbox:      c.XboxProber,
		dc:        c.DCProber,
		runner:    defaultCDInfoRunner,
		backoff:   backoff,
	}
}

type multiProbeClassifier struct {
	cdInfoBin string
	fs        FSProber
	bd        BDProber
	sysCNF    SystemCNFProber
	saturn    SaturnProber
	xbox      XboxProber
	dc        DCProber
	runner    cdInfoRunner
	backoff   []time.Duration
}

// Classify shells out to cd-info, with retry-with-backoff to absorb the
// 1–8 s spin-up window after a disc insertion. Without the retry, the
// first cd-info call lands 60–100 ms after the udev event and fails
// with "exit status 1" on most drives.
func (c *multiProbeClassifier) Classify(ctx context.Context, devPath string) (state.DiscType, error) {
	out, lastErr := c.runCDInfo(ctx, devPath)
	if lastErr != nil {
		return "", fmt.Errorf("cd-info: %w", lastErr)
	}
	base, err := ClassifyFromCDInfo(string(out))
	if err != nil {
		return "", err
	}
	fs := c.fs
	if fs != nil {
		// The ISO9660 listing can be momentarily empty in the disc
		// spin-up window even after cd-info already succeeds; retry it
		// on the same backoff schedule so the disc isn't silently
		// downgraded to DATA. See retryingFSProber.
		fs = &retryingFSProber{inner: c.fs, backoff: c.backoff}
	}
	return RefineDiscType(ctx, base, fs, c.bd, c.sysCNF, c.saturn, c.xbox, c.dc, devPath), nil
}

func (c *multiProbeClassifier) runCDInfo(ctx context.Context, devPath string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= len(c.backoff); attempt++ {
		out, err := c.runner(ctx, c.cdInfoBin, devPath)
		if err == nil {
			if attempt > 0 {
				slog.Info("cd-info succeeded after retry", "dev", devPath, "attempts", attempt+1)
			}
			return out, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == len(c.backoff) {
			break
		}
		select {
		case <-time.After(c.backoff[attempt]):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// retryingFSProber wraps an FSProber with the same retry-with-backoff
// the cd-info probe uses. cd-info can report a disc ready a beat
// before isoinfo can list its ISO9660 filesystem — isoinfo then exits
// 0 with an empty listing, which would otherwise silently downgrade
// the disc to generic DATA (and from there it's invisible in the UI).
// Retrying while the listing stays empty absorbs that spin-up window.
// A genuinely blank disc exhausts the schedule and returns the empty
// listing, which RefineDiscType resolves to DATA — same as before.
type retryingFSProber struct {
	inner   FSProber
	backoff []time.Duration
}

func (r *retryingFSProber) List(ctx context.Context, devPath string) ([]string, error) {
	var (
		files   []string
		lastErr error
	)
	for attempt := 0; attempt <= len(r.backoff); attempt++ {
		files, lastErr = r.inner.List(ctx, devPath)
		if lastErr == nil && len(files) > 0 {
			if attempt > 0 {
				slog.Info("fs probe succeeded after retry", "dev", devPath, "attempts", attempt+1)
			}
			return files, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if attempt == len(r.backoff) {
			break
		}
		select {
		case <-time.After(r.backoff[attempt]):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	// Retries exhausted. A persistent error wins; otherwise return the
	// (empty) listing — a genuinely blank disc.
	if lastErr != nil {
		return nil, lastErr
	}
	return files, nil
}

// ClassifyFromCDInfo parses cd-info stdout/stderr and returns the
// disc-mode-level type. Recognises CD-DA → AUDIO_CD; everything else
// falls through as DATA. The higher-level classifier (RefineDiscType)
// inspects the filesystem to upgrade DATA → DVD / BDMV / UHD.
func ClassifyFromCDInfo(s string) (state.DiscType, error) {
	if strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("ClassifyFromCDInfo: empty input")
	}
	for _, line := range strings.Split(s, "\n") {
		l := strings.ToLower(line)
		if !strings.Contains(l, "disc mode") {
			continue
		}
		if strings.Contains(l, "cd-da") {
			return state.DiscTypeAudioCD, nil
		}
	}
	return state.DiscTypeData, nil
}

// RefineDiscType upgrades a cd-info-level disc type using filesystem
// listing, bd_info (Blu-ray AACS2 detection), SYSTEM.CNF (PSX/PS2
// discrimination), and dedicated probers for Xbox, Saturn, and Dreamcast.
// AUDIO_CD short-circuits.
//
// Decision tree:
//   - AUDIO_CD → AUDIO_CD (passthrough)
//   - DATA + /VIDEO_TS → DVD
//   - DATA + /BDMV/index.bdmv:
//     bd_info AACS2 → UHD
//     else          → BDMV
//   - DATA + /SYSTEM.CNF:
//     SYSTEM.CNF readable + IsPS2 → PS2
//     SYSTEM.CNF readable + !IsPS2 → PSX
//     SYSTEM.CNF unreadable → DATA
//   - DATA + /default.xbe + xbox probe ok → XBOX
//   - saturn probe ok (raw sector 0) → SAT
//   - dc probe ok (TOC heuristic) → DC
//   - else → DATA
//
// Probes that error are logged and treated as a negative result.
func RefineDiscType(ctx context.Context, base state.DiscType, fs FSProber, bd BDProber, sysCNF SystemCNFProber, saturn SaturnProber, xbox XboxProber, dc DCProber, devPath string) state.DiscType {
	if base == state.DiscTypeAudioCD {
		return state.DiscTypeAudioCD
	}
	if fs == nil {
		return base
	}
	files, err := fs.List(ctx, devPath)
	if err != nil {
		slog.Warn("classify: fs probe failed", "dev", devPath, "err", err)
		return base
	}
	if hasPath(files, "/VIDEO_TS") {
		return state.DiscTypeDVD
	}
	if hasPath(files, "/BDMV/index.bdmv") {
		if bd == nil {
			return state.DiscTypeBDMV
		}
		info, err := bd.Probe(ctx, devPath)
		if err != nil {
			slog.Warn("classify: bd_info failed; defaulting to BDMV", "dev", devPath, "err", err)
			return state.DiscTypeBDMV
		}
		if info != nil && info.HasAACS2 {
			return state.DiscTypeUHD
		}
		return state.DiscTypeBDMV
	}
	if hasPath(files, "/SYSTEM.CNF") {
		if sysCNF == nil {
			slog.Warn("classify: SYSTEM.CNF present but no prober; treating as DATA", "dev", devPath)
			return state.DiscTypeData
		}
		info, err := sysCNF.Probe(ctx, devPath)
		if err != nil {
			slog.Warn("classify: system.cnf probe failed; treating as DATA", "dev", devPath, "err", err)
			return state.DiscTypeData
		}
		if info == nil {
			return state.DiscTypeData
		}
		if info.IsPS2 {
			return state.DiscTypePS2
		}
		return state.DiscTypePSX
	}
	// Xbox: /default.xbe at root signals a candidate; XBE probe confirms.
	if hasPath(files, "/default.xbe") && xbox != nil {
		info, err := xbox.Probe(ctx, devPath)
		if err != nil {
			if !errors.Is(err, ErrNotXbox) {
				slog.Warn("classify: xbox probe failed", "dev", devPath, "err", err)
			}
		} else if info != nil {
			return state.DiscTypeXBOX
		}
	}
	// Saturn: probe raw sector 0 regardless of fs listing; Saturn discs
	// often have no ISO9660 filesystem so files may be empty.
	if saturn != nil {
		info, err := saturn.Probe(ctx, devPath)
		if err != nil {
			if !errors.Is(err, ErrNotSaturn) {
				slog.Warn("classify: saturn probe failed", "dev", devPath, "err", err)
			}
		} else if info != nil {
			return state.DiscTypeSAT
		}
	}
	// Dreamcast: multi-session TOC heuristic (GD-ROM HD area at LBA ≥ 45000).
	if dc != nil {
		ok, err := dc.Probe(ctx, devPath)
		if err != nil {
			slog.Warn("classify: dc probe failed", "dev", devPath, "err", err)
		} else if ok {
			return state.DiscTypeDC
		}
	}
	// Breadcrumb: nothing recognised the disc. fs_entries==0 points at a
	// still-spinning-up read (the retry should normally absorb that);
	// a non-zero count means a genuinely unsupported data disc.
	slog.Info("classify: disc not recognised by any probe; treating as DATA",
		"dev", devPath, "fs_entries", len(files))
	return state.DiscTypeData
}

func hasPath(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
