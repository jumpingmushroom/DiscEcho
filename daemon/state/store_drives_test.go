package state_test

import (
	"context"
	"path/filepath"
	"strings"
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

// TestStore_ResetIdentifyingDrives is the recovery for a stuck-identify
// crash: if a previous run left a drive in `identifying` (because the
// classify ctx timed out and the cleanup UpdateDriveState was called
// with that same cancelled ctx, silently failing), every subsequent
// uevent on that drive hits "already identifying, ignoring" and the
// daemon stays deaf. Boot-time recovery resets stuck rows so a fresh
// uevent can claim the drive again. Drives in other states must be
// left alone.
func TestStore_ResetIdentifyingDrives(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	stuck := &state.Drive{
		Model: "ASUS SDRW", Bus: "sr0", DevPath: "/dev/sr0",
		State: state.DriveStateIdentifying, LastSeenAt: time.Now(),
	}
	idle := &state.Drive{
		Model: "Pioneer BDR", Bus: "sr1", DevPath: "/dev/sr1",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	ripping := &state.Drive{
		Model: "LG WH16", Bus: "sr2", DevPath: "/dev/sr2",
		State: state.DriveStateRipping, LastSeenAt: time.Now(),
	}
	for _, d := range []*state.Drive{stuck, idle, ripping} {
		if err := s.UpsertDrive(ctx, d); err != nil {
			t.Fatalf("upsert %s: %v", d.DevPath, err)
		}
	}

	n, err := s.ResetIdentifyingDrives(ctx)
	if err != nil {
		t.Fatalf("ResetIdentifyingDrives: %v", err)
	}
	if n != 1 {
		t.Errorf("rows reset: want 1, got %d", n)
	}

	cases := []struct {
		d    *state.Drive
		want state.DriveState
	}{
		{stuck, state.DriveStateIdle},
		{idle, state.DriveStateIdle},
		{ripping, state.DriveStateRipping},
	}
	for _, c := range cases {
		got, err := s.GetDrive(ctx, c.d.ID)
		if err != nil {
			t.Fatalf("get %s: %v", c.d.DevPath, err)
		}
		if got.State != c.want {
			t.Errorf("%s: state want %s, got %s", c.d.DevPath, c.want, got.State)
		}
	}
}

func mustStore(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestStore_UpdateDriveLastError_RoundTrip(t *testing.T) {
	s := openStore(t)
	drv := newDrive(t, s, "/dev/sr0")
	if err := s.UpdateDriveLastError(context.Background(), drv.ID, "cd-info: exit status 1"); err != nil {
		t.Fatalf("UpdateDriveLastError: %v", err)
	}
	got, err := s.GetDrive(context.Background(), drv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError != "cd-info: exit status 1" {
		t.Errorf("LastError = %q", got.LastError)
	}
	if got.LastErrorTip == "" {
		t.Errorf("LastErrorTip empty; expected a tip for cd-info error")
	}
}

func TestStore_UpdateDriveState_ClearsLastErrorOnIdle(t *testing.T) {
	s := openStore(t)
	drv := newDrive(t, s, "/dev/sr0")
	mustStore(t, s.UpdateDriveLastError(context.Background(), drv.ID, "some error"))
	mustStore(t, s.UpdateDriveState(context.Background(), drv.ID, state.DriveStateIdle))
	got, err := s.GetDrive(context.Background(), drv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError != "" {
		t.Errorf("LastError = %q; want empty after state->idle", got.LastError)
	}
}

func TestStore_UpdateDriveState_PreservesLastErrorOnError(t *testing.T) {
	s := openStore(t)
	drv := newDrive(t, s, "/dev/sr0")
	mustStore(t, s.UpdateDriveLastError(context.Background(), drv.ID, "some error"))
	mustStore(t, s.UpdateDriveState(context.Background(), drv.ID, state.DriveStateError))
	got, err := s.GetDrive(context.Background(), drv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastError != "some error" {
		t.Errorf("LastError = %q; want preserved when state->error", got.LastError)
	}
}

func TestStore_UpdateDriveReadOffset_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	d := &state.Drive{
		Model: "LG WH16NS60", Bus: "USB", DevPath: "/dev/sr0",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetDrive(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadOffset != 0 || got.ReadOffsetSource != "" {
		t.Errorf("fresh drive: want offset=0 source='', got offset=%d source=%q",
			got.ReadOffset, got.ReadOffsetSource)
	}

	if err := s.UpdateDriveReadOffset(ctx, d.ID, +102, "manual"); err != nil {
		t.Fatalf("update offset: %v", err)
	}

	got, err = s.GetDrive(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadOffset != 102 || got.ReadOffsetSource != "manual" {
		t.Errorf("after update: want offset=102 source='manual', got offset=%d source=%q",
			got.ReadOffset, got.ReadOffsetSource)
	}

	// Negative offsets are real (e.g. Pioneer drives ~ -1164). Make sure
	// the column round-trips the sign.
	if err := s.UpdateDriveReadOffset(ctx, d.ID, -1164, "auto"); err != nil {
		t.Fatal(err)
	}
	got, err = s.GetDrive(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadOffset != -1164 || got.ReadOffsetSource != "auto" {
		t.Errorf("negative round-trip: got offset=%d source=%q", got.ReadOffset, got.ReadOffsetSource)
	}
}

func TestStore_UpdateDriveReadOffset_NotFound(t *testing.T) {
	s := openStore(t)
	if err := s.UpdateDriveReadOffset(context.Background(), "ghost", 0, "manual"); err != state.ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// TestStore_UpsertDrive_PreservesReadOffset guards against a regression
// where the device-rescan-on-boot UpsertDrive path included the
// read_offset / read_offset_source columns in its UPDATE branch, which
// would zero out a calibration the user had explicitly set. The upsert
// must touch model/bus/state/last_seen_at/notes only.
func TestStore_UpsertDrive_PreservesReadOffset(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// First scan: device shows up.
	d1 := &state.Drive{
		Model: "Old", Bus: "USB", DevPath: "/dev/sr0",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d1); err != nil {
		t.Fatal(err)
	}

	// User calibrates the drive.
	if err := s.UpdateDriveReadOffset(ctx, d1.ID, +667, "auto"); err != nil {
		t.Fatal(err)
	}

	// Daemon restarts; device-scanner re-upserts the same dev_path.
	d2 := &state.Drive{
		Model: "Updated firmware", Bus: "USB", DevPath: "/dev/sr0",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(ctx, d2); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetDrive(ctx, d1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadOffset != 667 || got.ReadOffsetSource != "auto" {
		t.Errorf("offset clobbered on re-scan: got offset=%d source=%q (want 667/auto)",
			got.ReadOffset, got.ReadOffsetSource)
	}
	if got.Model != "Updated firmware" {
		t.Errorf("model not refreshed: %q", got.Model)
	}
}

func TestDriveErrorTip(t *testing.T) {
	tests := []struct{ in, wantContains string }{
		{"cd-info: exit status 1", "couldn't read this disc"},
		{"isoinfo: timed out", ""},
		{"", ""},
	}
	for _, tt := range tests {
		got := state.DriveErrorTip(tt.in)
		if tt.wantContains == "" {
			if got != "" {
				t.Errorf("DriveErrorTip(%q) = %q, want empty", tt.in, got)
			}
		} else if !strings.Contains(got, tt.wantContains) {
			t.Errorf("DriveErrorTip(%q) = %q, want substring %q", tt.in, got, tt.wantContains)
		}
	}
}
