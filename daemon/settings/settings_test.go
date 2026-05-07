package settings_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/settings"
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

// envFn returns a getenv that reads from m. Empty/missing keys yield "".
func envFn(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoad_Defaults(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})

	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Addr != ":8088" {
		t.Errorf("Addr = %q, want :8088", s.Addr)
	}
	if s.LibraryPath != "/library" {
		t.Errorf("LibraryPath = %q, want /library", s.LibraryPath)
	}
	if s.DataPath != dataDir {
		t.Errorf("DataPath = %q, want %q", s.DataPath, dataDir)
	}
	if s.WhipperBin != "whipper" {
		t.Errorf("WhipperBin = %q", s.WhipperBin)
	}
	if s.AppriseBin != "apprise" {
		t.Errorf("AppriseBin = %q", s.AppriseBin)
	}
	if s.EjectBin != "eject" {
		t.Errorf("EjectBin = %q", s.EjectBin)
	}
	if s.CDInfoBin != "cd-info" {
		t.Errorf("CDInfoBin = %q", s.CDInfoBin)
	}
	if s.CDParanoiaBin != "cdparanoia" {
		t.Errorf("CDParanoiaBin = %q", s.CDParanoiaBin)
	}
	if s.MusicBrainzBaseURL != "https://musicbrainz.org" {
		t.Errorf("MusicBrainzBaseURL = %q", s.MusicBrainzBaseURL)
	}
	if want := "DiscEcho/test ( https://github.com/jumpingmushroom/DiscEcho )"; s.MusicBrainzUserAgent != want {
		t.Errorf("MusicBrainzUserAgent = %q, want %q", s.MusicBrainzUserAgent, want)
	}
	if s.AutoConfirmSeconds != 8 {
		t.Errorf("AutoConfirmSeconds = %d, want 8", s.AutoConfirmSeconds)
	}
	if s.Token == "" {
		t.Error("Token should be non-empty (generated)")
	}
}

func TestLoad_AutoConfirmSeconds_Override(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":                 dataDir,
		"DISCECHO_AUTO_CONFIRM_SECONDS": "15",
	})

	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.AutoConfirmSeconds != 15 {
		t.Errorf("AutoConfirmSeconds = %d, want 15", s.AutoConfirmSeconds)
	}
}

func TestLoad_AutoConfirmSeconds_InvalidFallsBackToDefault(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":                 dataDir,
		"DISCECHO_AUTO_CONFIRM_SECONDS": "not-a-number",
	})

	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.AutoConfirmSeconds != 8 {
		t.Errorf("AutoConfirmSeconds = %d, want default 8", s.AutoConfirmSeconds)
	}
}

func TestLoad_Token_FromEnv(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":  dataDir,
		"DISCECHO_TOKEN": "env-token-xyz",
	})

	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Token != "env-token-xyz" {
		t.Errorf("Token = %q, want env-token-xyz", s.Token)
	}
	// Env-provided token must NOT touch the on-disk file.
	if _, err := os.Stat(filepath.Join(dataDir, "token")); !os.IsNotExist(err) {
		t.Errorf("token file should not exist when env token provided; stat err=%v", err)
	}
}

func TestLoad_Token_FromFile(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "token"), []byte("file-token-abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})

	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Token != "file-token-abc" {
		t.Errorf("Token = %q, want file-token-abc", s.Token)
	}
}

func TestLoad_Token_GeneratedAndPersisted(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})

	s1, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s1.Token == "" {
		t.Fatal("expected generated token")
	}
	tokFile := filepath.Join(dataDir, "token")
	b, err := os.ReadFile(tokFile)
	if err != nil {
		t.Fatalf("token file: %v", err)
	}
	if got := string(b); got == "" || got[len(got)-1] != '\n' {
		t.Errorf("token file should end with newline, got %q", got)
	}
	// Hex of 32 random bytes => 64 hex chars.
	if len(s1.Token) != 64 {
		t.Errorf("generated token len = %d, want 64", len(s1.Token))
	}
	// Re-Load should pick up the persisted file, yielding the same token.
	s2, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}
	if s2.Token != s1.Token {
		t.Errorf("token not stable across loads: %q vs %q", s1.Token, s2.Token)
	}
}

func TestLoad_ProfileSeeded_OnlyOnce(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})

	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	ctx := context.Background()
	profs, err := store.ListProfilesByDiscType(ctx, state.DiscTypeAudioCD)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs) != 1 {
		t.Fatalf("got %d profiles, want 1", len(profs))
	}
	if profs[0].Name != "CD-FLAC" {
		t.Errorf("Name = %q, want CD-FLAC", profs[0].Name)
	}
	if profs[0].Engine != "whipper" || profs[0].Format != "FLAC" {
		t.Errorf("unexpected profile: %+v", profs[0])
	}

	// Second load: must not duplicate.
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	profs2, err := store.ListProfilesByDiscType(ctx, state.DiscTypeAudioCD)
	if err != nil {
		t.Fatal(err)
	}
	if len(profs2) != 1 {
		t.Errorf("after second Load got %d profiles, want 1", len(profs2))
	}
}

func TestLoad_NotificationsSeeded_FromCommaList(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":         dataDir,
		"DISCECHO_APPRISE_URLS": "tgram://a/b , discord://x/y,  ",
	})

	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	ctx := context.Background()
	notifs, err := store.ListNotifications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifs) != 2 {
		t.Fatalf("got %d notifications, want 2", len(notifs))
	}
	urls := map[string]bool{}
	for _, n := range notifs {
		urls[n.URL] = true
		if n.Triggers != "done,failed" {
			t.Errorf("Triggers = %q, want done,failed", n.Triggers)
		}
		if !n.Enabled {
			t.Errorf("notification %q not enabled", n.Name)
		}
	}
	if !urls["tgram://a/b"] || !urls["discord://x/y"] {
		t.Errorf("missing expected URLs: %v", urls)
	}

	// Re-Load with the same URLs: must not append duplicates because
	// notifications already exist.
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	notifs2, err := store.ListNotifications(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(notifs2) != 2 {
		t.Errorf("after second Load got %d notifications, want 2", len(notifs2))
	}
}

func TestLoad_NotificationsSeeded_EmptyURLs_NoOp(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})

	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	notifs, err := store.ListNotifications(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifs) != 0 {
		t.Errorf("got %d notifications, want 0", len(notifs))
	}
}

func TestResolveToken_DisabledReturnsEmpty(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":          dataDir,
		"DISCECHO_AUTH_DISABLED": "true",
	})

	cfg, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token != "" {
		t.Errorf("token should be empty when auth disabled, got %q", cfg.Token)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "token")); err == nil {
		t.Errorf("token file should not be created when auth disabled")
	}
}

func TestLoad_DVDProfilesSeeded(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{"DISCECHO_DATA": dataDir})

	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}

	dvds, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypeDVD)
	if len(dvds) != 2 {
		t.Fatalf("want 2 DVD profiles, got %d", len(dvds))
	}
	names := map[string]bool{}
	for _, p := range dvds {
		names[p.Name] = true
	}
	if !names["DVD-Movie"] || !names["DVD-Series"] {
		t.Errorf("missing names: %v", names)
	}

	// Idempotent
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	dvds2, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypeDVD)
	if len(dvds2) != 2 {
		t.Errorf("after re-Load: %d", len(dvds2))
	}
}

func TestLoad_TMDBEnvVars(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":          dataDir,
		"DISCECHO_TMDB_KEY":      "abc-key",
		"DISCECHO_TMDB_LANG":     "fr-FR",
		"DISCECHO_SUBS_LANG":     "fra",
		"DISCECHO_HANDBRAKE_BIN": "/opt/handbrake",
		"DISCECHO_ISOINFO_BIN":   "/usr/local/bin/isoinfo",
	})
	cfg, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TMDBKey != "abc-key" || cfg.TMDBLang != "fr-FR" ||
		cfg.SubsLang != "fra" || cfg.HandBrakeBin != "/opt/handbrake" ||
		cfg.IsoInfoBin != "/usr/local/bin/isoinfo" {
		t.Errorf("got %+v", cfg)
	}
}
