package state_test

import (
	"context"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func newProfile(t *testing.T, s *state.Store, name string, dt state.DiscType) *state.Profile {
	t.Helper()
	p := &state.Profile{
		DiscType: dt,
		Name:     name,
		Engine:   "whipper",
		Format:   "FLAC",
		Preset:   "AccurateRip",
		Options: map[string]any{
			"cuesheet": true,
			"retries":  3,
		},
		OutputPathTemplate: "{{.Artist}}/{{.Album}}/{{.Title}}.flac",
		Enabled:            true,
		StepCount:          6,
	}
	if err := s.CreateProfile(context.Background(), p); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	return p
}

func TestStore_Profile_CreateAndGet_RoundTripsOptions(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	p := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	if p.ID == "" || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Fatalf("metadata not assigned: %+v", p)
	}

	got, err := s.GetProfile(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "CD-FLAC" || got.DiscType != state.DiscTypeAudioCD {
		t.Errorf("unexpected: %+v", got)
	}
	if got.Options["cuesheet"] != true {
		t.Errorf("options round-trip: cuesheet missing")
	}
	if v, ok := got.Options["retries"].(float64); !ok || v != 3 {
		t.Errorf("options round-trip: retries got %v", got.Options["retries"])
	}
}

func TestStore_Profile_NameUnique(t *testing.T) {
	s := openStore(t)
	newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	dup := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Name: "CD-FLAC",
		Engine: "x", Format: "y", Preset: "z",
		OutputPathTemplate: "{{.X}}", Enabled: true, StepCount: 1,
	}
	if err := s.CreateProfile(context.Background(), dup); err == nil {
		t.Errorf("expected unique-violation error")
	}
}

func TestStore_Profile_ListByDiscType(t *testing.T) {
	s := openStore(t)
	newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)
	newProfile(t, s, "DVD-Movie", state.DiscTypeDVD)
	newProfile(t, s, "DVD-Series", state.DiscTypeDVD)

	dvds, err := s.ListProfilesByDiscType(context.Background(), state.DiscTypeDVD)
	if err != nil {
		t.Fatal(err)
	}
	if len(dvds) != 2 {
		t.Errorf("want 2 DVD profiles, got %d", len(dvds))
	}

	cds, err := s.ListProfilesByDiscType(context.Background(), state.DiscTypeAudioCD)
	if err != nil {
		t.Fatal(err)
	}
	if len(cds) != 1 {
		t.Errorf("want 1 CD profile, got %d", len(cds))
	}
}

func TestStore_Profile_Update(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	p := newProfile(t, s, "CD-FLAC", state.DiscTypeAudioCD)

	original := p.UpdatedAt
	time.Sleep(2 * time.Millisecond)

	p.Preset = "lossless"
	if err := s.UpdateProfile(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetProfile(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Preset != "lossless" {
		t.Errorf("preset not updated: %q", got.Preset)
	}
	if !got.UpdatedAt.After(original) {
		t.Errorf("UpdatedAt not refreshed")
	}
}

func TestStore_Profile_DeleteNotFound(t *testing.T) {
	s := openStore(t)
	if err := s.DeleteProfile(context.Background(), "nope"); err == nil {
		t.Errorf("want ErrNotFound")
	}
}
