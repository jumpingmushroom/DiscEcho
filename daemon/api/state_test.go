package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// apitestServer constructs an in-memory store + broadcaster + empty
// pipelines registry and returns a Handlers wired up for endpoint tests.
// The Orchestrator field is left nil; tests that exercise endpoints
// touching it must construct their own (see discs_test.go).
func apitestServer(t *testing.T) *api.Handlers {
	t.Helper()
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := state.NewStore(db)
	bc := state.NewBroadcaster()
	t.Cleanup(bc.Close)
	return &api.Handlers{
		Store:       store,
		Broadcaster: bc,
		Pipelines:   pipelines.NewRegistry(),
		Apprise:     &fakeApprise{},
	}
}

// seedDrive inserts a drive and returns it.
func seedDrive(t *testing.T, h *api.Handlers) *state.Drive {
	t.Helper()
	d := &state.Drive{
		DevPath: "/dev/sr0", Model: "ASUS", Bus: "usb",
		State: state.DriveStateIdle, LastSeenAt: time.Now(),
	}
	if err := h.Store.UpsertDrive(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

// seedProfile inserts a profile and returns it.
func seedProfile(t *testing.T, h *api.Handlers) *state.Profile {
	t.Helper()
	p := &state.Profile{
		DiscType: state.DiscTypeAudioCD, Name: "default",
		Engine: "whipper", Format: "FLAC", Preset: "best",
		OutputPathTemplate: "{{.Title}}", Enabled: true, StepCount: 6,
	}
	if err := h.Store.CreateProfile(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

// seedDisc inserts a disc on the given drive and returns it.
func seedDisc(t *testing.T, h *api.Handlers, driveID string) *state.Disc {
	t.Helper()
	d := &state.Disc{
		DriveID: driveID, Type: state.DiscTypeAudioCD,
		Title: "An Album",
		Candidates: []state.Candidate{
			{Source: "musicbrainz", Title: "An Album", MBID: "mbid-1", Confidence: 95},
		},
	}
	if err := h.Store.CreateDisc(context.Background(), d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestGetState_ReturnsAllSections(t *testing.T) {
	h := apitestServer(t)
	drv := seedDrive(t, h)
	prof := seedProfile(t, h)
	seedDisc(t, h, drv.ID)
	if err := h.Store.SetSetting(context.Background(), "k", "v"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	w := httptest.NewRecorder()
	h.GetState(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: %d", w.Code)
	}
	var body struct {
		Drives   []state.Drive     `json:"drives"`
		Jobs     []state.Job       `json:"jobs"`
		Profiles []state.Profile   `json:"profiles"`
		Settings map[string]string `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Drives) != 1 {
		t.Errorf("drives: %d", len(body.Drives))
	}
	if len(body.Profiles) != 1 || body.Profiles[0].ID != prof.ID {
		t.Errorf("profiles: %+v", body.Profiles)
	}
	if body.Settings["k"] != "v" {
		t.Errorf("settings: %+v", body.Settings)
	}
}
