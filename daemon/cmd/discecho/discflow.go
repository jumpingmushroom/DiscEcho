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
		_ = df.store.UpdateDriveState(ctx, drv.ID, state.DriveStateError)
		return
	}
	handler, ok := df.pipelines.Get(dt)
	if !ok {
		slog.Info("no handler for disc type; skipping", "type", dt)
		_ = df.store.UpdateDriveState(ctx, drv.ID, state.DriveStateIdle)
		return
	}

	disc, candidates, err := handler.Identify(ctx, drv)
	switch {
	case errors.Is(err, pipelines.ErrNoCandidates):
		// Persist the disc record anyway so the UI can show "no matches".
		if disc != nil {
			disc.DriveID = drv.ID
			if cerr := df.store.CreateDisc(ctx, disc); cerr != nil {
				slog.Warn("create disc (no cands)", "err", cerr)
				return
			}
			df.bc.Publish(state.Event{Name: "disc.detected", Payload: map[string]any{"disc": disc}})
			df.bc.Publish(state.Event{
				Name:    "disc.identified",
				Payload: map[string]any{"disc": disc, "candidates": []state.Candidate{}},
			})
		}
		_ = df.store.UpdateDriveState(ctx, drv.ID, state.DriveStateIdle)
		return
	case err != nil:
		slog.Warn("identify failed", "err", err)
		_ = df.store.UpdateDriveState(ctx, drv.ID, state.DriveStateError)
		return
	}

	disc.DriveID = drv.ID
	if err := df.store.CreateDisc(ctx, disc); err != nil {
		slog.Warn("create disc", "err", err)
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
	if err := df.store.UpdateDriveState(ctx, drv.ID, state.DriveStateIdle); err != nil {
		slog.Warn("disc-flow: reset drive state", "err", err)
	}
	df.bc.Publish(state.Event{
		Name:    "drive.changed",
		Payload: map[string]any{"drive_id": drv.ID, "state": "idle"},
	})
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
