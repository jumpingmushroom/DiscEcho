package state_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestStore_Settings_RoundTrip(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	if _, err := s.GetSetting(ctx, "library_path"); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}

	if err := s.SetSetting(ctx, "library_path", "/srv/media"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSetting(ctx, "library_path")
	if err != nil {
		t.Fatal(err)
	}
	if got != "/srv/media" {
		t.Errorf("got %q", got)
	}

	if err := s.SetSetting(ctx, "library_path", "/library"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetSetting(ctx, "library_path")
	if got != "/library" {
		t.Errorf("upsert failed: %q", got)
	}
}

func TestStore_Settings_GetAll(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	_ = s.SetSetting(ctx, "a", "1")
	_ = s.SetSetting(ctx, "b", "2")
	_ = s.SetSetting(ctx, "c", "3")

	all, err := s.GetAllSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Migration 008 seeds operation.mode + rip.eject_on_finish, so
	// GetAllSettings always returns at least those two plus whatever
	// this test wrote.
	if all["a"] != "1" || all["b"] != "2" || all["c"] != "3" {
		t.Errorf("got %+v", all)
	}
}
