package state_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func openStore(t *testing.T) *state.Store {
	t.Helper()
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return state.NewStore(db)
}

func TestStore_Drive_UpsertAndGet(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	d := &state.Drive{
		Model:      "LG WH16NS60",
		Bus:        "USB · sr0",
		DevPath:    "/dev/sr0",
		State:      state.DriveStateIdle,
		LastSeenAt: time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
	}
	if err := s.UpsertDrive(ctx, d); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if d.ID == "" {
		t.Fatal("ID not assigned")
	}

	got, err := s.GetDrive(ctx, d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.DevPath != "/dev/sr0" || got.Model != "LG WH16NS60" {
		t.Errorf("unexpected: %+v", got)
	}
	if got.State != state.DriveStateIdle {
		t.Errorf("state: want idle, got %s", got.State)
	}
}

func TestStore_Drive_UpsertIsIdempotentByDevPath(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	d1 := &state.Drive{
		Model: "Old", Bus: "SATA", DevPath: "/dev/sr0",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d1); err != nil {
		t.Fatal(err)
	}

	d2 := &state.Drive{
		Model: "Updated", Bus: "USB", DevPath: "/dev/sr0",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d2); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListDrives(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 drive, got %d", len(all))
	}
	if all[0].Model != "Updated" {
		t.Errorf("model not updated: %q", all[0].Model)
	}
}

func TestStore_Drive_UpdateState(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	d := &state.Drive{
		DevPath: "/dev/sr0", Model: "X", Bus: "Y",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateDriveState(ctx, d.ID, state.DriveStateRipping); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetDrive(ctx, d.ID)
	if got.State != state.DriveStateRipping {
		t.Errorf("state: want ripping, got %s", got.State)
	}
}

// ClaimDriveForIdentify must succeed once on an idle drive and refuse
// concurrent re-claims. This is the lock the discFlow guard relies on
// to debounce duplicate `disc inserted` uevents — without atomicity,
// every uevent kicks off its own identify and creates a duplicate
// Disc row.
func TestStore_Drive_ClaimForIdentify(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	d := &state.Drive{
		DevPath: "/dev/sr0", Model: "X", Bus: "Y",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d); err != nil {
		t.Fatal(err)
	}

	// First claim wins.
	ok, err := s.ClaimDriveForIdentify(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("first claim must succeed against an idle drive")
	}

	// Second concurrent claim must lose — drive is now in identifying.
	ok, err = s.ClaimDriveForIdentify(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("second claim must fail while drive is identifying")
	}

	// Once identify finishes (state back to idle), a later claim succeeds.
	if err := s.UpdateDriveState(ctx, d.ID, state.DriveStateIdle); err != nil {
		t.Fatal(err)
	}
	ok, err = s.ClaimDriveForIdentify(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("claim should succeed after drive returns to idle")
	}

	// Drives in a non-idle non-error state (e.g. ripping) cannot be
	// claimed — the active-job guard upstream of this should catch
	// that case first, but defence in depth matters.
	if err := s.UpdateDriveState(ctx, d.ID, state.DriveStateRipping); err != nil {
		t.Fatal(err)
	}
	ok, err = s.ClaimDriveForIdentify(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("claim must not succeed against a ripping drive")
	}

	// Error-state drives are claimable — a fresh insert after a failed
	// classify should be able to retry.
	if err := s.UpdateDriveState(ctx, d.ID, state.DriveStateError); err != nil {
		t.Fatal(err)
	}
	ok, err = s.ClaimDriveForIdentify(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("claim should succeed against an error-state drive")
	}
}

func TestStore_Drive_GetNotFound(t *testing.T) {
	s := openStore(t)
	_, err := s.GetDrive(context.Background(), "nope")
	if err == nil {
		t.Errorf("want ErrNotFound, got nil")
	}
}

func TestStore_Drive_UpdateStateNotFound(t *testing.T) {
	s := openStore(t)
	err := s.UpdateDriveState(context.Background(), "nope", state.DriveStateRipping)
	if err == nil {
		t.Errorf("want ErrNotFound, got nil")
	}
}
