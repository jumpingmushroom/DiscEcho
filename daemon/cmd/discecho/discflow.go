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
	if err := df.store.UpdateDriveState(ctx, drv.ID, state.DriveStateIdentifying); err != nil {
		slog.Warn("disc-flow: drive state", "err", err)
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
