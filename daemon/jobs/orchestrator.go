package jobs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// OrchestratorConfig configures NewOrchestrator.
type OrchestratorConfig struct {
	Store       *state.Store
	Broadcaster *state.Broadcaster
	Pipelines   *pipelines.Registry
}

// Orchestrator owns job lifecycle: queueing, per-drive serialization,
// ctx cancellation, and the PersistentSink wiring.
type Orchestrator struct {
	cfg OrchestratorConfig

	mu       sync.Mutex
	queues   map[string]chan jobItem // per-drive
	cancels  map[string]context.CancelFunc
	stopOnce sync.Once
	stopped  chan struct{}
	wg       sync.WaitGroup
}

type jobItem struct {
	jobID string
}

// NewOrchestrator constructs an Orchestrator. Marks any
// queued/identifying/running jobs as interrupted (crash recovery).
func NewOrchestrator(cfg OrchestratorConfig) *Orchestrator {
	o := &Orchestrator{
		cfg:     cfg,
		queues:  make(map[string]chan jobItem),
		cancels: make(map[string]context.CancelFunc),
		stopped: make(chan struct{}),
	}
	if n, err := cfg.Store.MarkInterruptedJobs(context.Background()); err != nil {
		slog.Warn("orchestrator: MarkInterruptedJobs", "err", err)
	} else if n > 0 {
		slog.Info("orchestrator: marked interrupted jobs", "count", n)
	}
	return o
}

// Close stops every per-drive worker. Idempotent.
//
// We don't close the per-drive channels; closing them races with
// Submit (which sends on them). Instead we close o.stopped, which
// every worker selects on, and discard the queues map so future
// Submits return errStopped without sending. Pending items in the
// channels are dropped — the worker observes <-o.stopped first.
func (o *Orchestrator) Close() {
	o.stopOnce.Do(func() {
		o.mu.Lock()
		o.queues = nil
		o.mu.Unlock()
		close(o.stopped)
		o.wg.Wait()
	})
}

// Submit creates a job and enqueues it on the disc's drive's worker.
// Returns the persisted Job (state=queued).
func (o *Orchestrator) Submit(ctx context.Context, discID, profileID string) (*state.Job, error) {
	disc, err := o.cfg.Store.GetDisc(ctx, discID)
	if err != nil {
		return nil, fmt.Errorf("get disc: %w", err)
	}
	prof, err := o.cfg.Store.GetProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("get profile: %w", err)
	}
	handler, ok := o.cfg.Pipelines.Get(disc.Type)
	if !ok {
		return nil, fmt.Errorf("no handler registered for %s", disc.Type)
	}

	driveID := disc.DriveID
	if driveID == "" {
		return nil, errors.New("submit: disc has no drive_id; cannot queue")
	}

	plan := handler.Plan(disc, prof)
	steps := make([]state.JobStep, 0, len(plan))
	for _, sp := range plan {
		st := state.JobStepStatePending
		if sp.Skip {
			st = state.JobStepStateSkipped
		}
		steps = append(steps, state.JobStep{Step: sp.ID, State: st})
	}
	job := &state.Job{
		DiscID:    disc.ID,
		DriveID:   driveID,
		ProfileID: prof.ID,
		State:     state.JobStateQueued,
		Steps:     steps,
	}
	if err := o.cfg.Store.CreateJob(ctx, job); err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}
	o.cfg.Broadcaster.Publish(state.Event{
		Name: "job.created", Payload: map[string]any{"job": job},
	})

	if err := o.enqueue(driveID, job.ID); err != nil {
		return nil, err
	}
	return job, nil
}

// Cancel signals the running job to stop. If the job is queued (not
// yet picked up) it's flipped to cancelled in the store so the worker
// skips it when it pops.
func (o *Orchestrator) Cancel(jobID string) error {
	o.mu.Lock()
	cancel, ok := o.cancels[jobID]
	o.mu.Unlock()
	if !ok {
		if err := o.cfg.Store.UpdateJobState(context.Background(), jobID, state.JobStateCancelled, ""); err != nil {
			return fmt.Errorf("cancel: %w", err)
		}
		return nil
	}
	cancel()
	return nil
}

// errStopped is returned by enqueue when Submit races shutdown.
var errStopped = errors.New("orchestrator: stopped")

func (o *Orchestrator) enqueue(driveID, jobID string) error {
	o.mu.Lock()
	if o.queues == nil {
		o.mu.Unlock()
		return errStopped
	}
	q, ok := o.queues[driveID]
	if !ok {
		q = make(chan jobItem, 64)
		o.queues[driveID] = q
		o.wg.Add(1)
		go o.worker(driveID, q)
	}
	o.mu.Unlock()
	// Send outside the lock so Close/Cancel stay responsive when a
	// drive's queue is full. The queue is never closed (see Close),
	// so there's no closed-channel-send risk here; Close signals via
	// o.stopped instead.
	select {
	case q <- jobItem{jobID: jobID}:
		return nil
	case <-o.stopped:
		return errStopped
	}
}

// worker drains one drive's queue serially. Exits when o.stopped is
// closed; the queue itself is never closed (see Close).
func (o *Orchestrator) worker(driveID string, q chan jobItem) {
	defer o.wg.Done()
	for {
		select {
		case <-o.stopped:
			return
		case item := <-q:
			o.runJob(item.jobID)
		}
	}
}

// runJob dispatches one job through its handler.
func (o *Orchestrator) runJob(jobID string) {
	ctx, cancel := context.WithCancel(context.Background())
	o.mu.Lock()
	o.cancels[jobID] = cancel
	o.mu.Unlock()
	defer func() {
		cancel()
		o.mu.Lock()
		delete(o.cancels, jobID)
		o.mu.Unlock()
	}()

	job, err := o.cfg.Store.GetJob(ctx, jobID)
	if err != nil {
		slog.Error("orchestrator: get job", "id", jobID, "err", err)
		return
	}
	if job.State == state.JobStateCancelled {
		// User cancelled before the worker picked it up.
		o.cfg.Broadcaster.Publish(state.Event{Name: "job.failed", Payload: map[string]any{"job_id": jobID}})
		return
	}

	disc, err := o.cfg.Store.GetDisc(ctx, job.DiscID)
	if err != nil {
		slog.Error("orchestrator: get disc", "id", jobID, "err", err)
		return
	}
	drv, err := o.cfg.Store.GetDrive(ctx, job.DriveID)
	if err != nil {
		slog.Error("orchestrator: get drive", "id", jobID, "err", err)
		return
	}
	prof, err := o.cfg.Store.GetProfile(ctx, job.ProfileID)
	if err != nil {
		slog.Error("orchestrator: get profile", "id", jobID, "err", err)
		return
	}
	handler, ok := o.cfg.Pipelines.Get(disc.Type)
	if !ok {
		err := fmt.Errorf("no handler for %s", disc.Type)
		_ = o.cfg.Store.UpdateJobState(ctx, jobID, state.JobStateFailed, err.Error())
		o.cfg.Broadcaster.Publish(state.Event{Name: "job.failed", Payload: map[string]any{"job_id": jobID}})
		return
	}

	// Transition: queued → running
	if err := o.cfg.Store.UpdateJobState(ctx, jobID, state.JobStateRunning, ""); err != nil {
		slog.Error("orchestrator: state running", "id", jobID, "err", err)
		return
	}
	if err := o.cfg.Store.UpdateDriveState(ctx, drv.ID, state.DriveStateRipping); err != nil {
		slog.Warn("orchestrator: drive state ripping", "drv", drv.ID, "err", err)
	}
	o.cfg.Broadcaster.Publish(state.Event{Name: "drive.changed", Payload: map[string]any{"drive_id": drv.ID, "state": "ripping"}})

	sink := NewPersistentSink(o.cfg.Store, o.cfg.Broadcaster, jobID)
	runErr := handler.Run(ctx, drv, disc, prof, sink)

	// Determine final state
	var final state.JobState
	errMsg := ""
	switch {
	case runErr == nil:
		final = state.JobStateDone
	case errors.Is(runErr, context.Canceled), errors.Is(runErr, context.DeadlineExceeded):
		final = state.JobStateCancelled
	default:
		final = state.JobStateFailed
		errMsg = runErr.Error()
	}
	// Final state writes use a fresh context: the per-job ctx may have
	// been cancelled (cancellation path) and we still need to persist
	// the terminal state.
	if err := o.cfg.Store.UpdateJobState(context.Background(), jobID, final, errMsg); err != nil {
		slog.Error("orchestrator: state final", "id", jobID, "err", err)
	}
	if err := o.cfg.Store.UpdateDriveState(context.Background(), drv.ID, state.DriveStateIdle); err != nil {
		slog.Warn("orchestrator: drive state idle", "drv", drv.ID, "err", err)
	}
	o.cfg.Broadcaster.Publish(state.Event{Name: "drive.changed", Payload: map[string]any{"drive_id": drv.ID, "state": "idle"}})

	switch final {
	case state.JobStateDone:
		o.cfg.Broadcaster.Publish(state.Event{Name: "job.done", Payload: map[string]any{"job_id": jobID}})
	case state.JobStateCancelled:
		o.cfg.Broadcaster.Publish(state.Event{Name: "job.failed", Payload: map[string]any{"job_id": jobID, "state": "cancelled"}})
	case state.JobStateFailed:
		o.cfg.Broadcaster.Publish(state.Event{Name: "job.failed", Payload: map[string]any{"job_id": jobID, "error": errMsg}})
	}
}
