package api

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// ActiveJobsSampler keeps an in-memory ring of the last 24 hourly
// samples of the active job count. The /api/stats handler reads from
// this for the sparkline + 1h delta. A daemon restart loses history,
// which is acceptable for a dashboard widget.
type ActiveJobsSampler struct {
	store *state.Store
	mu    sync.Mutex
	ring  [24]int // most recent at the current write index (cur-1 mod 24)
	cur   int     // index of the next slot to write
	full  bool
}

// NewActiveJobsSampler constructs a sampler bound to the given store.
// Call Start to spin up the hourly tick goroutine.
func NewActiveJobsSampler(store *state.Store) *ActiveJobsSampler {
	return &ActiveJobsSampler{store: store}
}

// Start takes the initial sample and runs an hourly ticker until ctx
// is cancelled.
func (a *ActiveJobsSampler) Start(ctx context.Context) {
	a.sample(ctx)
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.sample(ctx)
			}
		}
	}()
}

func (a *ActiveJobsSampler) sample(ctx context.Context) {
	row := a.store.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE state NOT IN ('done','failed','cancelled','interrupted')`)
	var n int
	if err := row.Scan(&n); err != nil {
		return
	}
	a.mu.Lock()
	a.ring[a.cur] = n
	a.cur = (a.cur + 1) % 24
	if a.cur == 0 {
		a.full = true
	}
	a.mu.Unlock()
}

// snapshot returns the 24-entry sparkline (oldest first) and the 1-hour
// delta. When the ring isn't filled yet, missing entries are zero.
func (a *ActiveJobsSampler) snapshot(currentValue int) (spark [24]int, delta1h int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := 0; i < 24; i++ {
		spark[i] = a.ring[(a.cur+i)%24]
	}
	if a.full || a.cur >= 2 {
		idx := (a.cur - 2 + 24) % 24
		delta1h = currentValue - a.ring[idx]
	}
	return
}

// Stats returns the dashboard's top-widgets payload, blending
// Store.Stats with the in-memory active-jobs sampler and a statfs
// walk over distinct library roots.
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats := h.computeStats(r.Context())
	writeJSON(w, http.StatusOK, stats)
}

// computeStats is shared by Stats and writeSnapshot — both surface
// the same payload.
func (h *Handlers) computeStats(ctx context.Context) state.Stats {
	stats, _ := h.Store.Stats(ctx, time.Now())
	if h.ActiveSampler != nil {
		ring, delta := h.ActiveSampler.snapshot(stats.ActiveJobs.Value)
		stats.ActiveJobs.Spark24h = ring[:]
		stats.ActiveJobs.Delta1h = delta
	}
	stats.Library.TotalBytes = h.libraryTotalBytes(ctx)
	return stats
}

// libraryTotalBytes walks the configured library roots, deduplicates
// by filesystem device id, and sums each filesystem's total bytes.
// Returns 0 on any error (the widget gracefully renders the headline
// without the "of X TB" subline).
func (h *Handlers) libraryTotalBytes(ctx context.Context) int64 {
	roots := h.libraryRoots(ctx)
	seen := map[uint64]bool{}
	var total int64
	for _, root := range roots {
		if root == "" {
			continue
		}
		var stat syscall.Statfs_t
		if err := syscall.Statfs(root, &stat); err != nil {
			continue
		}
		// Dedupe by Type+Bsize+Blocks — Fsid struct layout varies across
		// libc / kernels and isn't portable to access. The composite key
		// uniquely identifies a mounted filesystem in practice for the
		// homelab scale we care about (≤5 distinct mounts).
		key := uint64(stat.Type)<<48 | uint64(stat.Bsize)<<16 | uint64(stat.Blocks)
		if seen[key] {
			continue
		}
		seen[key] = true
		total += int64(stat.Blocks) * stat.Bsize
	}
	return total
}

func (h *Handlers) libraryRoots(ctx context.Context) []string {
	all, err := h.Store.GetAllSettings(ctx)
	if err != nil {
		return nil
	}
	keys := []string{"library.movies", "library.tv", "library.music", "library.games", "library.data"}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if v := all[k]; v != "" {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
