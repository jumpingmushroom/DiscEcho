package main

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/drive"
	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// discFlowCooldown is the window after a job ends during which we
// ignore further media-change uevents on the same drive. Closes the
// race where the spurious mid-rip uevent fires at the instant
// HandBrake exits — by then the job is `done` so the active-job
// guard returns false, but the disc is still spinning down and
// re-classifying just wastes effort. 10 s comfortably covers
// HandBrake/makemkvcon teardown and udev's own quiesce time.
const discFlowCooldown = 10 * time.Second

// discDedupWindow is how far back we look when deduplicating game-disc rows
// by (drive_id, metadata_id). Slow drives (e.g. ASUS SDRW-08D2S-U) emit
// 2-3 media-change uevents per physical insertion; without this window each
// uevent creates a fresh disc row and queues a separate auto-rip job.
// 2 minutes is wide enough to absorb burst uevents from one insertion yet
// narrow enough that a genuine eject-and-reinsert gets a fresh row.
const discDedupWindow = 2 * time.Minute

// discFlow handles one optical-media-change uevent: classify the disc,
// pick the matching pipeline handler, run Identify, persist the disc
// row, and broadcast disc.detected / disc.identified events.
type discFlow struct {
	store       *state.Store
	bc          *state.Broadcaster
	classifier  identify.Classifier
	pipelines   *pipelines.Registry
	identifyDur time.Duration
}

func (df *discFlow) handle(ev drive.Uevent) {
	if !ev.IsOpticalMediaChange() {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), df.identifyDur)
	defer cancel()

	devPath := "/dev/" + ev.DevName
	slog.Info("disc inserted", "dev", devPath)

	drv, err := df.findDriveByDevPath(ctx, devPath)
	if err != nil {
		slog.Warn("disc-flow: no drive registered", "dev", devPath, "err", err)
		return
	}
	// Drives emit spurious media-change uevents during a long rip when
	// HandBrake / makemkvcon hammer the SCSI bus. Re-classifying then
	// races the running step's exclusive hold on /dev/sr0 and the
	// classifier always fails ("cd-info: exit status 1"), which then
	// flips the drive to Error and kills the in-flight job. Bail out
	// early if there's already a job in flight for this drive.
	if busy, err := df.store.HasActiveJobOnDrive(ctx, drv.ID); err != nil {
		slog.Warn("disc-flow: HasActiveJobOnDrive", "err", err)
	} else if busy {
		slog.Info("disc-flow: drive busy, ignoring media-change", "dev", devPath, "drive_id", drv.ID)
		return
	}
	if recent, err := df.store.HasRecentJobOnDrive(ctx, drv.ID, discFlowCooldown); err != nil {
		slog.Warn("disc-flow: HasRecentJobOnDrive", "err", err)
	} else if recent {
		slog.Info("disc-flow: drive in post-job cooldown, ignoring media-change", "dev", devPath, "drive_id", drv.ID)
		return
	}
	// Atomic CAS: only proceed if the drive is currently idle/error.
	// Closes the race where multiple media-change uevents fire in quick
	// succession (Hollywood DVDs emit 2–3 per insertion as the drive
	// settles) and would otherwise each kick off an independent identify
	// + create a duplicate Disc row.
	claimed, err := df.store.ClaimDriveForIdentify(ctx, drv.ID)
	if err != nil {
		slog.Warn("disc-flow: ClaimDriveForIdentify", "err", err)
		return
	}
	if !claimed {
		slog.Info("disc-flow: drive already identifying, ignoring media-change", "dev", devPath, "drive_id", drv.ID)
		return
	}
	df.bc.Publish(state.Event{
		Name:    "drive.changed",
		Payload: map[string]any{"drive_id": drv.ID, "state": "identifying"},
	})

	dt, err := df.classifier.Classify(ctx, devPath)
	if err != nil {
		slog.Warn("classify failed", "dev", devPath, "err", err)
		df.releaseDriveState(drv.ID, state.DriveStateError)
		return
	}
	handler, ok := df.pipelines.Get(dt)
	if !ok {
		slog.Info("no handler for disc type; skipping", "type", dt)
		df.releaseDriveState(drv.ID, state.DriveStateIdle)
		return
	}

	disc, candidates, err := handler.Identify(ctx, drv)
	switch {
	case errors.Is(err, pipelines.ErrNoCandidates):
		// Persist the disc record anyway so the UI can show "no matches".
		if disc != nil {
			disc.DriveID = drv.ID
			if perr := df.persistDisc(ctx, disc, nil); perr != nil {
				slog.Warn("persist disc (no cands)", "err", perr)
				df.releaseDriveState(drv.ID, state.DriveStateError)
				return
			}
			df.bc.Publish(state.Event{Name: "disc.detected", Payload: map[string]any{"disc": disc}})
			df.bc.Publish(state.Event{
				Name:    "disc.identified",
				Payload: map[string]any{"disc": disc, "candidates": []state.Candidate{}},
			})
		}
		df.releaseDriveState(drv.ID, state.DriveStateIdle)
		return
	case err != nil:
		slog.Warn("identify failed", "err", err)
		df.releaseDriveState(drv.ID, state.DriveStateError)
		return
	}

	disc.DriveID = drv.ID
	if err := df.persistDisc(ctx, disc, candidates); err != nil {
		slog.Warn("persist disc", "err", err)
		df.releaseDriveState(drv.ID, state.DriveStateError)
		return
	}
	df.bc.Publish(state.Event{Name: "disc.detected", Payload: map[string]any{"disc": disc}})
	df.bc.Publish(state.Event{
		Name:    "disc.identified",
		Payload: map[string]any{"disc": disc, "candidates": candidates},
	})
	// Identify is done. The disc is now waiting for the user to pick a
	// candidate and start a job; the drive itself is no longer doing
	// any work, so flip it back to idle. Leaving it in `identifying`
	// makes the dashboard lie ("Identifying disc…") and blocks future
	// uevents from being processed cleanly.
	df.releaseDriveState(drv.ID, state.DriveStateIdle)
	df.bc.Publish(state.Event{
		Name:    "drive.changed",
		Payload: map[string]any{"drive_id": drv.ID, "state": "idle"},
	})
}

// persistDisc inserts a new disc row, or — when the drive already has a
// matching disc — refreshes that existing row's metadata fields and rebinds
// disc.ID to it. The caller then publishes events with the canonical
// (possibly preexisting) ID so downstream listeners (and the disc-decision
// UI) attach a job to the reused row rather than spawning yet another
// duplicate.
//
// Dedup is two-tiered:
//   - Tier 1 (TOC hash): audio CDs and data discs that compute a content hash.
//   - Tier 2 (metadata_id within discDedupWindow): game discs (PSX/PS2/SAT/DC/XBOX)
//     that lack a TOC hash but carry a stable boot code / product number / title ID.
//     Slow drives emit 2-3 uevents per insertion; this tier prevents each from
//     creating its own disc row.
//
// candidates can be nil for the no-candidates branch; in that case we
// don't overwrite the existing row's candidates JSON.
func (df *discFlow) persistDisc(ctx context.Context, disc *state.Disc, candidates []state.Candidate) error {
	if disc.DriveID == "" {
		return df.store.CreateDisc(ctx, disc)
	}
	// Tier 1: TOC hash (audio CDs, data discs).
	if disc.TOCHash != "" {
		existing, err := df.store.GetDiscByDriveTOC(ctx, disc.DriveID, disc.TOCHash)
		if err == nil {
			return df.reuseDiscRow(ctx, existing, disc, candidates)
		}
		if !errors.Is(err, state.ErrNotFound) {
			return err
		}
	}
	// Tier 2: metadata_id within a short window (game discs).
	// Same drive + same metadata_id within discDedupWindow is the same
	// physical disc. Without this, the slow ASUS SDRW-08D2S-U drive's
	// 3-uevent-per-insertion behaviour creates 3 disc rows.
	if disc.MetadataID != "" {
		existing, err := df.store.GetDiscByDriveAndMetadataID(ctx, disc.DriveID, disc.MetadataID, discDedupWindow)
		if err == nil {
			return df.reuseDiscRow(ctx, existing, disc, candidates)
		}
		if !errors.Is(err, state.ErrNotFound) {
			return err
		}
	}
	return df.store.CreateDisc(ctx, disc)
}

// reuseDiscRow refreshes an existing disc row's metadata from a fresh
// identify pass and rebinds the in-memory disc to the existing ID so
// jobs.disc_id references remain coherent. candidates can be nil, in which
// case the existing row's candidates JSON is left untouched.
func (df *discFlow) reuseDiscRow(ctx context.Context, existing, disc *state.Disc, candidates []state.Candidate) error {
	// Found a prior row for this physical disc. Refresh the metadata
	// fields from the fresh identify pass so a re-identify (after the
	// user picks a different MB release, or after TMDB enriches a TV
	// series later) sticks. Reuse the existing ID so jobs.disc_id
	// references stay coherent.
	if err := df.store.UpdateDiscMetadata(ctx, existing.ID, disc.Title, disc.Year, disc.MetadataProvider, disc.MetadataID); err != nil {
		return err
	}
	if disc.MetadataJSON != "" {
		if err := df.store.UpdateDiscMetadataBlob(ctx, existing.ID, disc.MetadataJSON); err != nil {
			return err
		}
	}
	if candidates != nil {
		if err := df.store.UpdateDiscCandidates(ctx, existing.ID, candidates); err != nil {
			return err
		}
	}
	disc.ID = existing.ID
	disc.CreatedAt = existing.CreatedAt
	return nil
}

// releaseDriveState writes the drive's terminal state for this handle()
// invocation using a fresh background context. The identify ctx
// (df.identifyDur, 30s) is cancelled the moment classify or identify
// times out, and ExecContext on the original ctx then returns
// context.Canceled before the SQL runs — silently leaving the drive
// stuck in `identifying` and locking the daemon out of every later
// uevent on that drive. Always use a clean context for the cleanup
// write, and surface failures via the log instead of discarding them.
func (df *discFlow) releaseDriveState(driveID string, st state.DriveState) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := df.store.UpdateDriveState(ctx, driveID, st); err != nil {
		slog.Warn("disc-flow: release drive state", "err", err, "drive_id", driveID, "target_state", st)
	}
}

func (df *discFlow) findDriveByDevPath(ctx context.Context, dev string) (*state.Drive, error) {
	drives, err := df.store.ListDrives(ctx)
	if err != nil {
		return nil, err
	}
	for i := range drives {
		if drives[i].DevPath == dev {
			return &drives[i], nil
		}
	}
	return nil, errors.New("no drive with dev_path " + dev)
}
