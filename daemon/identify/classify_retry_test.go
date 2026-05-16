package identify

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// TestClassifyRetry_TransientFailures verifies the classifier retries
// cd-info until either it succeeds or the schedule is exhausted. This
// covers the spin-up race we hit on the homelab: udev fires the
// media-change event ~60 ms after insert, well before the drive can
// answer a SCSI INQUIRY.
func TestClassifyRetry_TransientFailures(t *testing.T) {
	cdda, err := os.ReadFile("testdata/cdinfo-cdda.txt")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("succeeds after 2 transient failures", func(t *testing.T) {
		attempts := 0
		runner := func(_ context.Context, _ string, _ string) ([]byte, error) {
			attempts++
			if attempts <= 2 {
				return nil, errors.New("exit status 1")
			}
			return cdda, nil
		}

		c := &multiProbeClassifier{
			cdInfoBin: "stub",
			fs:        &fakeFSProberInternal{},
			bd:        &fakeBDProberInternal{},
			runner:    runner,
			backoff:   []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond},
		}

		got, err := c.Classify(context.Background(), "/dev/sr0")
		if err != nil {
			t.Fatalf("Classify: unexpected error %v", err)
		}
		if got != state.DiscTypeAudioCD {
			t.Errorf("disc type: want AUDIO_CD, got %s", got)
		}
		if attempts != 3 {
			t.Errorf("attempts: want 3, got %d", attempts)
		}
	})

	t.Run("gives up after schedule exhausted", func(t *testing.T) {
		attempts := 0
		runner := func(_ context.Context, _ string, _ string) ([]byte, error) {
			attempts++
			return nil, errors.New("exit status 1")
		}

		c := &multiProbeClassifier{
			cdInfoBin: "stub",
			runner:    runner,
			backoff:   []time.Duration{time.Microsecond, time.Microsecond},
		}

		_, err := c.Classify(context.Background(), "/dev/sr0")
		if err == nil {
			t.Fatal("Classify: want error after exhausting retries")
		}
		// 1 initial attempt + 2 backoff entries = 3 total tries.
		if attempts != 3 {
			t.Errorf("attempts: want 3, got %d", attempts)
		}
	})

	t.Run("respects context cancellation between attempts", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		runner := func(_ context.Context, _ string, _ string) ([]byte, error) {
			attempts++
			if attempts == 1 {
				cancel()
			}
			return nil, errors.New("exit status 1")
		}

		c := &multiProbeClassifier{
			cdInfoBin: "stub",
			runner:    runner,
			backoff:   []time.Duration{time.Hour, time.Hour}, // long; cancel must short-circuit
		}

		_, err := c.Classify(ctx, "/dev/sr0")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err: want context.Canceled, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("attempts: want 1 (cancel must stop retry loop), got %d", attempts)
		}
	})
}

// TestRetryingFSProber covers the filesystem-probe retry decorator
// directly: the ISO9660 listing can come back empty for a beat in the
// disc spin-up window (isoinfo exits 0 but lists nothing), and the
// decorator must retry that the same way the cd-info probe is retried.
func TestRetryingFSProber(t *testing.T) {
	t.Run("retries until the listing is non-empty", func(t *testing.T) {
		attempts := 0
		r := &retryingFSProber{
			inner: &fakeFSProberInternal{listFn: func() ([]string, error) {
				attempts++
				if attempts <= 2 {
					return nil, nil // exit 0, empty listing — not ready
				}
				return []string{"/SYSTEM.CNF"}, nil
			}},
			backoff: []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond},
		}
		files, err := r.List(context.Background(), "/dev/sr0")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(files) != 1 || files[0] != "/SYSTEM.CNF" {
			t.Errorf("files: want [/SYSTEM.CNF], got %v", files)
		}
		if attempts != 3 {
			t.Errorf("attempts: want 3, got %d", attempts)
		}
	})

	t.Run("gives up and returns the empty listing when never ready", func(t *testing.T) {
		attempts := 0
		r := &retryingFSProber{
			inner: &fakeFSProberInternal{listFn: func() ([]string, error) {
				attempts++
				return nil, nil
			}},
			backoff: []time.Duration{time.Microsecond, time.Microsecond},
		}
		files, err := r.List(context.Background(), "/dev/sr0")
		if err != nil {
			t.Errorf("List: want nil error for a genuinely empty disc, got %v", err)
		}
		if len(files) != 0 {
			t.Errorf("files: want empty, got %v", files)
		}
		// 1 initial attempt + 2 backoff entries = 3 total tries.
		if attempts != 3 {
			t.Errorf("attempts: want 3, got %d", attempts)
		}
	})

	t.Run("surfaces a persistent probe error", func(t *testing.T) {
		r := &retryingFSProber{
			inner: &fakeFSProberInternal{listFn: func() ([]string, error) {
				return nil, errors.New("isoinfo: exit status 1")
			}},
			backoff: []time.Duration{time.Microsecond},
		}
		_, err := r.List(context.Background(), "/dev/sr0")
		if err == nil {
			t.Fatal("List: want error after exhausting retries")
		}
	})

	t.Run("respects context cancellation between retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		r := &retryingFSProber{
			inner: &fakeFSProberInternal{listFn: func() ([]string, error) {
				attempts++
				if attempts == 1 {
					cancel()
				}
				return nil, nil
			}},
			backoff: []time.Duration{time.Hour, time.Hour}, // long; cancel must short-circuit
		}
		_, err := r.List(ctx, "/dev/sr0")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err: want context.Canceled, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("attempts: want 1 (cancel must stop retry loop), got %d", attempts)
		}
	})
}

// TestRetryingSystemCNFProber covers the SYSTEM.CNF retry decorator.
// isoinfo -x SYSTEM.CNF can exit 0 with an empty body for a beat in
// the spin-up window even after the directory listing already shows
// the file. ParseSystemCNF then returns nil, which the classifier
// would otherwise silently downgrade to DATA — retry until a parseable
// BOOT/BOOT2 line lands.
func TestRetryingSystemCNFProber(t *testing.T) {
	t.Run("retries until SystemCNF parses", func(t *testing.T) {
		attempts := 0
		r := &retryingSystemCNFProber{
			inner: &fakeSysCNFProberInternal{probeFn: func() (*SystemCNF, error) {
				attempts++
				if attempts <= 2 {
					return nil, nil // isoinfo -x exit 0, empty body
				}
				return &SystemCNF{BootCode: "SCES_534.09", IsPS2: true}, nil
			}},
			backoff: []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond},
		}
		info, err := r.Probe(context.Background(), "/dev/sr0")
		if err != nil {
			t.Fatalf("Probe: %v", err)
		}
		if info == nil || info.BootCode != "SCES_534.09" || !info.IsPS2 {
			t.Errorf("info: want {SCES_534.09, IsPS2=true}, got %+v", info)
		}
		if attempts != 3 {
			t.Errorf("attempts: want 3, got %d", attempts)
		}
	})

	t.Run("gives up and returns nil when never parses", func(t *testing.T) {
		attempts := 0
		r := &retryingSystemCNFProber{
			inner: &fakeSysCNFProberInternal{probeFn: func() (*SystemCNF, error) {
				attempts++
				return nil, nil
			}},
			backoff: []time.Duration{time.Microsecond, time.Microsecond},
		}
		info, err := r.Probe(context.Background(), "/dev/sr0")
		if err != nil {
			t.Errorf("Probe: want nil error for an unparseable SYSTEM.CNF, got %v", err)
		}
		if info != nil {
			t.Errorf("info: want nil, got %+v", info)
		}
		// 1 initial + 2 backoff entries = 3 total attempts.
		if attempts != 3 {
			t.Errorf("attempts: want 3, got %d", attempts)
		}
	})

	t.Run("surfaces a persistent probe error", func(t *testing.T) {
		r := &retryingSystemCNFProber{
			inner: &fakeSysCNFProberInternal{probeFn: func() (*SystemCNF, error) {
				return nil, errors.New("isoinfo: exit status 1")
			}},
			backoff: []time.Duration{time.Microsecond},
		}
		_, err := r.Probe(context.Background(), "/dev/sr0")
		if err == nil {
			t.Fatal("Probe: want error after exhausting retries")
		}
	})

	t.Run("respects context cancellation between retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		attempts := 0
		r := &retryingSystemCNFProber{
			inner: &fakeSysCNFProberInternal{probeFn: func() (*SystemCNF, error) {
				attempts++
				if attempts == 1 {
					cancel()
				}
				return nil, nil
			}},
			backoff: []time.Duration{time.Hour, time.Hour}, // long; cancel must short-circuit
		}
		_, err := r.Probe(ctx, "/dev/sr0")
		if !errors.Is(err, context.Canceled) {
			t.Errorf("err: want context.Canceled, got %v", err)
		}
		if attempts != 1 {
			t.Errorf("attempts: want 1 (cancel must stop retry loop), got %d", attempts)
		}
	})
}

// TestClassifyRetry_SystemCNFSpinUp is the end-to-end regression for
// the PS2-PAL Sly 3 misclassification: cd-info and the ISO9660 listing
// both report ready, /SYSTEM.CNF is in the listing, but the first
// SYSTEM.CNF extract comes back empty because the drive hasn't fully
// caught up. Without the retry the disc silently falls through to
// generic DATA at classify.go:449; with it, Classify waits out the
// spin-up window and the disc resolves to PS2.
func TestClassifyRetry_SystemCNFSpinUp(t *testing.T) {
	dataMode, err := os.ReadFile("testdata/cdinfo-data.txt")
	if err != nil {
		t.Fatal(err)
	}
	sysCNFAttempts := 0
	c := &multiProbeClassifier{
		cdInfoBin: "stub",
		fs:        &fakeFSProberInternal{files: []string{"/SYSTEM.CNF", "/SCES_534.09"}},
		bd:        &fakeBDProberInternal{},
		sysCNF: &fakeSysCNFProberInternal{probeFn: func() (*SystemCNF, error) {
			sysCNFAttempts++
			if sysCNFAttempts <= 2 {
				return nil, nil // isoinfo -x exit 0, empty body — disc not ready
			}
			return &SystemCNF{BootCode: "SCES_534.09", IsPS2: true}, nil
		}},
		runner:  func(_ context.Context, _, _ string) ([]byte, error) { return dataMode, nil },
		backoff: []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond},
	}
	got, err := c.Classify(context.Background(), "/dev/sr0")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != state.DiscTypePS2 {
		t.Errorf("disc type: want PS2, got %s", got)
	}
	if sysCNFAttempts != 3 {
		t.Errorf("system.cnf probe attempts: want 3, got %d", sysCNFAttempts)
	}
}

// TestClassifyRetry_FSProbeSpinUp is the end-to-end regression for the
// PS2-disc misclassification: cd-info reports ready a beat before
// isoinfo can list the ISO9660 filesystem, so the first fs probe comes
// back empty. Without the retry the disc silently falls through to
// generic DATA; with it, Classify waits out the spin-up window and the
// SYSTEM.CNF probe correctly resolves it to PS2.
func TestClassifyRetry_FSProbeSpinUp(t *testing.T) {
	dataMode, err := os.ReadFile("testdata/cdinfo-data.txt")
	if err != nil {
		t.Fatal(err)
	}
	fsAttempts := 0
	c := &multiProbeClassifier{
		cdInfoBin: "stub",
		fs: &fakeFSProberInternal{listFn: func() ([]string, error) {
			fsAttempts++
			if fsAttempts <= 2 {
				return nil, nil // isoinfo exit 0, empty listing — disc not ready
			}
			return []string{"/SYSTEM.CNF", "/SCES_534.09"}, nil
		}},
		bd:      &fakeBDProberInternal{},
		sysCNF:  &fakeSysCNFProberInternal{info: &SystemCNF{BootCode: "SCES_534.09", IsPS2: true}},
		runner:  func(_ context.Context, _, _ string) ([]byte, error) { return dataMode, nil },
		backoff: []time.Duration{time.Microsecond, time.Microsecond, time.Microsecond},
	}
	got, err := c.Classify(context.Background(), "/dev/sr0")
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got != state.DiscTypePS2 {
		t.Errorf("disc type: want PS2, got %s", got)
	}
	if fsAttempts != 3 {
		t.Errorf("fs probe attempts: want 3, got %d", fsAttempts)
	}
}

// Internal fakes mirroring the external test fakes — duplicated because
// the external test fakes live in package identify_test and can't be
// reused from package identify.
type fakeFSProberInternal struct {
	files []string
	// listFn, when set, overrides files — lets a test script per-call
	// behaviour (e.g. empty for the first N calls, then a real listing).
	listFn func() ([]string, error)
}

func (f *fakeFSProberInternal) List(_ context.Context, _ string) ([]string, error) {
	if f.listFn != nil {
		return f.listFn()
	}
	return f.files, nil
}

type fakeBDProberInternal struct {
	info *BDInfo
	err  error
}

func (f *fakeBDProberInternal) Probe(_ context.Context, _ string) (*BDInfo, error) {
	return f.info, f.err
}

type fakeSysCNFProberInternal struct {
	info *SystemCNF
	err  error
	// probeFn, when set, overrides info/err — lets a test script per-call
	// behaviour (e.g. nil for the first N calls, then a real SystemCNF).
	probeFn func() (*SystemCNF, error)
}

func (f *fakeSysCNFProberInternal) Probe(_ context.Context, _ string) (*SystemCNF, error) {
	if f.probeFn != nil {
		return f.probeFn()
	}
	return f.info, f.err
}
