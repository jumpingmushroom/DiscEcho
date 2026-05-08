package state

import (
	"context"
	"log/slog"
	"time"
)

// SettingsReader is the small interface the sweeper consults for
// retention configuration. *Store satisfies it via GetBool/GetInt.
type SettingsReader interface {
	GetBool(ctx context.Context, key string) (bool, error)
	GetInt(ctx context.Context, key string) (int, error)
}

// Sweeper periodically deletes old history rows according to retention
// settings. It runs immediately on Start, then daily at 03:00 daemon-local.
type Sweeper struct {
	Store    *Store
	Settings SettingsReader
	Now      func() time.Time
	Logger   *slog.Logger
}

// Start launches the sweeper goroutine. Cancel ctx to stop.
func (s *Sweeper) Start(ctx context.Context) {
	go s.run(ctx)
}

func (s *Sweeper) run(ctx context.Context) {
	s.Tick(ctx)
	for {
		next := NextThreeAM(s.Now())
		timer := time.NewTimer(next.Sub(s.Now()))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.Tick(ctx)
		}
	}
}

// Tick runs one sweep iteration. Exported so tests can drive it directly.
func (s *Sweeper) Tick(ctx context.Context) {
	logger := s.Logger
	if logger == nil {
		logger = slog.Default()
	}

	forever, _ := s.Settings.GetBool(ctx, "retention.forever")
	if forever {
		return
	}

	days, _ := s.Settings.GetInt(ctx, "retention.days")
	if days <= 0 {
		return
	}

	cutoff := s.Now().Add(-time.Duration(days) * 24 * time.Hour)
	n, err := s.Store.PruneHistoryBefore(ctx, cutoff)
	if err != nil {
		logger.Error("retention sweep failed", "err", err)
		return
	}
	logger.Info("retention sweep", "deleted_jobs", n, "cutoff", cutoff)
}

// NextThreeAM returns the next 03:00 in now's location, strictly after now.
func NextThreeAM(now time.Time) time.Time {
	target := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if !target.After(now) {
		target = target.Add(24 * time.Hour)
	}
	return target
}
