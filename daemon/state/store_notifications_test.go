package state_test

import (
	"context"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestStore_Notification_CRUD(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	n := &state.Notification{
		Name: "homelab", URL: "ntfys://discecho",
		Tags: "music,test", Triggers: "done,failed", Enabled: true,
	}
	if err := s.CreateNotification(ctx, n); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetNotification(ctx, n.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.URL != "ntfys://discecho" || !got.Enabled {
		t.Errorf("get mismatch: %+v", got)
	}

	got.Enabled = false
	if err := s.UpdateNotification(ctx, got); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetNotification(ctx, n.ID)
	if got2.Enabled {
		t.Errorf("disable failed")
	}

	if err := s.DeleteNotification(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetNotification(ctx, n.ID); err == nil {
		t.Errorf("want ErrNotFound after delete")
	}
}

func TestStore_Notification_ListForTrigger_PostFilters(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	for _, n := range []state.Notification{
		{Name: "n1", URL: "u1", Triggers: "done,failed", Enabled: true},
		{Name: "n2", URL: "u2", Triggers: "warn", Enabled: true},
		{Name: "n3", URL: "u3", Triggers: "done", Enabled: false},  // disabled
		{Name: "n4", URL: "u4", Triggers: "donezo", Enabled: true}, // false positive
	} {
		nn := n
		if err := s.CreateNotification(ctx, &nn); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListNotificationsForTrigger(ctx, "done")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "n1" {
		t.Errorf("want [n1], got %+v", got)
	}
}

func TestStore_Notification_DefaultTriggers(t *testing.T) {
	s := openStore(t)
	n := &state.Notification{Name: "x", URL: "y"}
	if err := s.CreateNotification(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if n.Triggers != "done,failed" {
		t.Errorf("default triggers: want 'done,failed', got %q", n.Triggers)
	}
}
