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

// Internal fakes mirroring the external test fakes — duplicated because
// the external test fakes live in package identify_test and can't be
// reused from package identify.
type fakeFSProberInternal struct{ files []string }

func (f *fakeFSProberInternal) List(_ context.Context, _ string) ([]string, error) {
	return f.files, nil
}

type fakeBDProberInternal struct {
	info *BDInfo
	err  error
}

func (f *fakeBDProberInternal) Probe(_ context.Context, _ string) (*BDInfo, error) {
	return f.info, f.err
}
