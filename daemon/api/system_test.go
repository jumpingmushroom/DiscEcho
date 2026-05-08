package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/api"
	"github.com/jumpingmushroom/DiscEcho/daemon/settings"
)

func TestGetSystemHost_OK(t *testing.T) {
	h := apitestServer(t)
	libDir := t.TempDir()
	h.Settings = &settings.Settings{
		LibraryRoot:   libDir,
		LibraryMovies: libDir,
		DataPath:      "/path/that/definitely/does/not/exist/abcxyz",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/system/host", nil)
	w := httptest.NewRecorder()
	h.GetSystemHost(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var info api.HostInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.CPUCount != runtime.NumCPU() {
		t.Errorf("cpu_count: got %d want %d", info.CPUCount, runtime.NumCPU())
	}
	// Library path exists (TempDir) → at least one disk row.
	// Data path doesn't exist → silently skipped, no 500.
	if len(info.Disks) == 0 {
		t.Error("expected at least the library disk entry")
	}
	for _, d := range info.Disks {
		if d.Path == "/path/that/definitely/does/not/exist/abcxyz" {
			t.Errorf("missing path should be skipped, got %+v", d)
		}
	}
}

func TestGetSystemHost_NilSettings_DefaultsAreSafe(t *testing.T) {
	h := apitestServer(t)
	h.Settings = nil

	req := httptest.NewRequest(http.MethodGet, "/api/system/host", nil)
	w := httptest.NewRecorder()
	h.GetSystemHost(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
}

func TestGetSystemIntegrations_TMDBUnconfigured(t *testing.T) {
	h := apitestServer(t)
	h.Settings = &settings.Settings{
		TMDBKey:              "",
		TMDBLang:             "en-US",
		MusicBrainzBaseURL:   "https://musicbrainz.org",
		MusicBrainzUserAgent: "DiscEcho/test",
		AppriseBin:           "/nonexistent/apprise-binary",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/system/integrations", nil)
	w := httptest.NewRecorder()
	h.GetSystemIntegrations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d body %s", w.Code, w.Body.String())
	}
	var info api.IntegrationsInfo
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.TMDB.Configured {
		t.Error("TMDB should be unconfigured when key is empty")
	}
	if info.TMDB.Language != "en-US" {
		t.Errorf("language: %q", info.TMDB.Language)
	}
	if info.MusicBrainz.UserAgent != "DiscEcho/test" {
		t.Errorf("user-agent: %q", info.MusicBrainz.UserAgent)
	}
	if info.Apprise.Bin != "/nonexistent/apprise-binary" {
		t.Errorf("apprise bin: %q", info.Apprise.Bin)
	}
	// Bogus binary → version omitted, no 500.
	if info.Apprise.Version != "" {
		t.Errorf("apprise version unexpectedly set: %q", info.Apprise.Version)
	}
}

func TestGetSystemIntegrations_LibraryRoots(t *testing.T) {
	h := apitestServer(t)
	h.Settings = &settings.Settings{
		LibraryRoot:   "/library",
		LibraryMovies: "/library/movies",
		LibraryTV:     "/library/tv",
		LibraryMusic:  "/srv/audio",
		LibraryGames:  "/library/games",
		LibraryData:   "/library/data",
		AppriseBin:    "apprise",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/system/integrations", nil)
	w := httptest.NewRecorder()
	h.GetSystemIntegrations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var info api.IntegrationsInfo
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	for k, want := range map[string]string{
		"movies": "/library/movies",
		"tv":     "/library/tv",
		"music":  "/srv/audio",
		"games":  "/library/games",
		"data":   "/library/data",
	} {
		if got := info.LibraryRoots[k]; got != want {
			t.Errorf("library_roots[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestGetSystemIntegrations_TMDBConfigured(t *testing.T) {
	h := apitestServer(t)
	h.Settings = &settings.Settings{
		TMDBKey:              "secret-key",
		TMDBLang:             "fr-FR",
		MusicBrainzBaseURL:   "https://musicbrainz.org",
		MusicBrainzUserAgent: "DiscEcho/test",
		AppriseBin:           "apprise",
	}

	req := httptest.NewRequest(http.MethodGet, "/api/system/integrations", nil)
	w := httptest.NewRecorder()
	h.GetSystemIntegrations(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d", w.Code)
	}
	var info api.IntegrationsInfo
	_ = json.Unmarshal(w.Body.Bytes(), &info)
	if !info.TMDB.Configured {
		t.Error("TMDB should be configured")
	}
	// Body must not contain the secret key.
	if strings.Contains(w.Body.String(), "secret-key") {
		t.Errorf("response leaks TMDB key: %s", w.Body.String())
	}
}
