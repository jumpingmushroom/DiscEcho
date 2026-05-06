package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func newDrive(t *testing.T, s *state.Store, devPath string) *state.Drive {
	t.Helper()
	d := &state.Drive{
		DevPath: devPath, Model: "Test", Bus: "USB",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := s.UpsertDrive(context.Background(), d); err != nil {
		t.Fatalf("upsert drive: %v", err)
	}
	return d
}

func TestStore_Disc_CreateAndGet_RoundTripsCandidates(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	d := &state.Disc{
		DriveID: drv.ID,
		Type:    state.DiscTypeAudioCD,
		Title:   "Kind of Blue",
		Year:    1959,
		TOCHash: "sha1abc",
		Candidates: []state.Candidate{
			{Source: "MusicBrainz", Title: "Kind of Blue", Year: 1959, Confidence: 94, MBID: "kb-1"},
			{Source: "MusicBrainz", Title: "Kind of Blue (Remaster)", Year: 1997, Confidence: 81, MBID: "kb-2"},
		},
	}
	if err := s.CreateDisc(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}
	if d.ID == "" {
		t.Fatal("ID not assigned")
	}
	if d.CreatedAt.IsZero() {
		t.Errorf("CreatedAt not assigned")
	}

	got, err := s.GetDisc(ctx, d.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Title != "Kind of Blue" || got.Type != state.DiscTypeAudioCD {
		t.Errorf("unexpected: %+v", got)
	}
	if len(got.Candidates) != 2 {
		t.Fatalf("candidates: want 2, got %d", len(got.Candidates))
	}
	if got.Candidates[0].MBID != "kb-1" || got.Candidates[1].Confidence != 81 {
		t.Errorf("candidate round-trip mismatch: %+v", got.Candidates)
	}
}

func TestStore_Disc_CreateWithoutDriveID(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	d := &state.Disc{Type: state.DiscTypeData, Title: "A backup"}
	if err := s.CreateDisc(ctx, d); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := s.GetDisc(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.DriveID != "" {
		t.Errorf("DriveID: want empty, got %q", got.DriveID)
	}
}

func TestStore_Disc_ListForDrive(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	for i := 0; i < 3; i++ {
		d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, Title: "x"}
		if err := s.CreateDisc(ctx, d); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond) // ensure ordering
	}

	got, err := s.ListDiscsForDrive(ctx, drv.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 discs, got %d", len(got))
	}
}

func TestStore_Disc_GetNotFound(t *testing.T) {
	s := openStore(t)
	_, err := s.GetDisc(context.Background(), "nope")
	if err == nil {
		t.Errorf("want ErrNotFound, got nil")
	}
}
