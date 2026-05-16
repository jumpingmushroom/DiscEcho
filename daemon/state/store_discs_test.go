package state_test

import (
	"context"
	"errors"
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

func TestStore_UpdateDiscMetadataBlob_WritesAndReadsBack(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeDVD, Title: "Test"}
	if err := s.CreateDisc(ctx, d); err != nil {
		t.Fatal(err)
	}

	blob := `{"plot":"hello","cast":["a","b"]}`
	if err := s.UpdateDiscMetadataBlob(ctx, d.ID, blob); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetDisc(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MetadataJSON != blob {
		t.Errorf("metadata_json: want %q, got %q", blob, got.MetadataJSON)
	}
}

func TestStore_UpdateDiscMetadataBlob_NotFound(t *testing.T) {
	s := openStore(t)
	if err := s.UpdateDiscMetadataBlob(context.Background(), "missing", `{}`); err == nil {
		t.Errorf("want error, got nil")
	}
}

func TestStore_CreateDisc_DefaultsMetadataJSONToEmptyObject(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	d := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeDVD}
	if err := s.CreateDisc(ctx, d); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDisc(ctx, d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.MetadataJSON != "{}" {
		t.Errorf("default metadata_json: want %q, got %q", "{}", got.MetadataJSON)
	}
}

func TestStore_GetDiscByDriveTOC_ReturnsRowForMatch(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	d := &state.Disc{
		DriveID: drv.ID,
		Type:    state.DiscTypeAudioCD,
		Title:   "Fear and Bullets",
		TOCHash: "abc123",
	}
	if err := s.CreateDisc(ctx, d); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetDiscByDriveTOC(ctx, drv.ID, "abc123")
	if err != nil {
		t.Fatalf("get by toc: %v", err)
	}
	if got.ID != d.ID {
		t.Errorf("ID: want %q, got %q", d.ID, got.ID)
	}
	if got.Title != "Fear and Bullets" {
		t.Errorf("title: got %q", got.Title)
	}
}

func TestStore_GetDiscByDriveTOC_RejectsEmptyInputs(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	if _, err := s.GetDiscByDriveTOC(ctx, "", "abc"); err != state.ErrNotFound {
		t.Errorf("empty driveID: want ErrNotFound, got %v", err)
	}
	if _, err := s.GetDiscByDriveTOC(ctx, "drv-1", ""); err != state.ErrNotFound {
		t.Errorf("empty tocHash: want ErrNotFound, got %v", err)
	}
}

func TestStore_GetDiscByDriveTOC_NotFoundOnMiss(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	_, err := s.GetDiscByDriveTOC(ctx, drv.ID, "no-such-hash")
	if err != state.ErrNotFound {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestStore_DiscCandidates_RoundTripIGDBID(t *testing.T) {
	s := openStore(t)
	drv := newDrive(t, s, "/dev/sr0")
	d := newDisc(t, s, drv)
	cands := []state.Candidate{
		{Source: "IGDB", Title: "Sly 3: Honor Among Thieves", Year: 2005, Confidence: 25, IGDBID: 12345},
	}
	if err := s.UpdateDiscCandidates(context.Background(), d.ID, cands); err != nil {
		t.Fatalf("UpdateDiscCandidates: %v", err)
	}
	got, err := s.GetDisc(context.Background(), d.ID)
	if err != nil {
		t.Fatalf("GetDisc: %v", err)
	}
	if len(got.Candidates) != 1 || got.Candidates[0].IGDBID != 12345 {
		t.Errorf("IGDBID round-trip: got %+v", got.Candidates)
	}
}

func TestStore_GetDiscByDriveAndMetadataID(t *testing.T) {
	s := openStore(t)
	drv := newDrive(t, s, "/dev/sr0")

	d1 := &state.Disc{Type: state.DiscTypePS2, DriveID: drv.ID, MetadataID: "SCES_534.09", Title: "Sly 3"}
	if err := s.CreateDisc(context.Background(), d1); err != nil {
		t.Fatal(err)
	}

	t.Run("recent disc with matching metadata_id is returned", func(t *testing.T) {
		got, err := s.GetDiscByDriveAndMetadataID(context.Background(), drv.ID, "SCES_534.09", time.Minute)
		if err != nil {
			t.Fatalf("got err %v", err)
		}
		if got.ID != d1.ID {
			t.Errorf("got disc id %q, want %q", got.ID, d1.ID)
		}
	})

	t.Run("non-matching metadata_id returns ErrNotFound", func(t *testing.T) {
		_, err := s.GetDiscByDriveAndMetadataID(context.Background(), drv.ID, "SCES_000.00", time.Minute)
		if !errors.Is(err, state.ErrNotFound) {
			t.Errorf("got err %v, want ErrNotFound", err)
		}
	})

	t.Run("empty arg short-circuits", func(t *testing.T) {
		_, err := s.GetDiscByDriveAndMetadataID(context.Background(), "", "x", time.Minute)
		if !errors.Is(err, state.ErrNotFound) {
			t.Errorf("got err %v, want ErrNotFound", err)
		}
		_, err = s.GetDiscByDriveAndMetadataID(context.Background(), drv.ID, "", time.Minute)
		if !errors.Is(err, state.ErrNotFound) {
			t.Errorf("got err %v, want ErrNotFound", err)
		}
	})

	t.Run("disc older than `within` is filtered out", func(t *testing.T) {
		// Insert a disc with a created_at well in the past via CreateDisc
		// after manually setting CreatedAt; CreateDisc respects a non-zero
		// CreatedAt, so we can backdate it to simulate an old row.
		oldDisc := &state.Disc{
			Type:       state.DiscTypePS2,
			DriveID:    drv.ID,
			MetadataID: "SCES_111.11",
			Title:      "Old",
			CreatedAt:  time.Now().Add(-10 * time.Minute),
		}
		if err := s.CreateDisc(context.Background(), oldDisc); err != nil {
			t.Fatal(err)
		}
		_, err := s.GetDiscByDriveAndMetadataID(context.Background(), drv.ID, "SCES_111.11", time.Minute)
		if !errors.Is(err, state.ErrNotFound) {
			t.Errorf("got err %v, want ErrNotFound (outside dedup window)", err)
		}
	})
}

// Migration 009 also enforces a partial unique index on
// (drive_id, toc_hash) when the toc_hash is non-empty. Two empty-toc
// rows on the same drive are still allowed (so unidentifiable discs
// can coexist), but a duplicate non-empty toc_hash is rejected at the
// schema level.
func TestStore_DiscUniqueIndex_EnforcesOnePerDriveTOC(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	drv := newDrive(t, s, "/dev/sr0")

	first := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, TOCHash: "shared"}
	if err := s.CreateDisc(ctx, first); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	second := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeAudioCD, TOCHash: "shared"}
	if err := s.CreateDisc(ctx, second); err == nil {
		t.Fatal("second insert with same (drive, toc) should have failed")
	}

	// Empty-toc rows are allowed to coexist.
	a := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeData}
	b := &state.Disc{DriveID: drv.ID, Type: state.DiscTypeData}
	if err := s.CreateDisc(ctx, a); err != nil {
		t.Fatalf("empty-toc first: %v", err)
	}
	if err := s.CreateDisc(ctx, b); err != nil {
		t.Fatalf("empty-toc second: %v", err)
	}
}
