package settings_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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
	if s.LibraryRoot != "/library" {
		t.Errorf("LibraryRoot = %q, want /library", s.LibraryRoot)
	}
	if want := "/library/movies"; s.LibraryMovies != want {
		t.Errorf("LibraryMovies = %q, want %q", s.LibraryMovies, want)
	}
	if want := "/library/tv"; s.LibraryTV != want {
		t.Errorf("LibraryTV = %q, want %q", s.LibraryTV, want)
	}
	if want := "/library/music"; s.LibraryMusic != want {
		t.Errorf("LibraryMusic = %q, want %q", s.LibraryMusic, want)
	}
	if want := "/library/games"; s.LibraryGames != want {
		t.Errorf("LibraryGames = %q, want %q", s.LibraryGames, want)
	}
	if want := "/library/data"; s.LibraryData != want {
		t.Errorf("LibraryData = %q, want %q", s.LibraryData, want)
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
	if s.Token != "" {
		t.Errorf("Token should be empty by default, got %q", s.Token)
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

func TestLoad_Token_DefaultEmpty(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})

	s1, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load (first): %v", err)
	}
	if s1.Token != "" {
		t.Errorf("first Token = %q, want empty", s1.Token)
	}

	// Reboot on the same data dir — Token must stay empty (no
	// accidental caching, no token re-emerging from disk state).
	s2, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load (second): %v", err)
	}
	if s2.Token != "" {
		t.Errorf("second Token = %q, want empty across reboots", s2.Token)
	}
}

func TestLoad_Token_DoesNotWriteFile(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()

	// Default mode: no file should ever be created.
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("Load (default): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "token")); !os.IsNotExist(err) {
		t.Errorf("token file should not exist in default mode; stat err=%v", err)
	}

	// Bearer mode: the env-provided token must also not touch disk.
	env2 := envFn(map[string]string{
		"DISCECHO_DATA":  dataDir,
		"DISCECHO_TOKEN": "env-token",
	})
	if _, err := settings.Load(env2, store, "test"); err != nil {
		t.Fatalf("Load (env token): %v", err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "token")); !os.IsNotExist(err) {
		t.Errorf("token file should not exist when DISCECHO_TOKEN is set; stat err=%v", err)
	}
}

func TestLoad_Token_IgnoresExistingFile(t *testing.T) {
	// Pre-existing on-disk token from a prior install must NOT be read.
	store := openStore(t)
	dataDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dataDir, "token"), []byte("legacy-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if s.Token != "" {
		t.Errorf("Token = %q, want empty (legacy file must be ignored)", s.Token)
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
	byName := map[string]*state.Profile{}
	for i := range dvds {
		byName[dvds[i].Name] = &dvds[i]
	}
	if byName["DVD-Movie"] == nil || byName["DVD-Series"] == nil {
		t.Fatalf("missing seed names: %v", byName)
	}

	mv := byName["DVD-Movie"]
	if mv.Format != "MKV" {
		t.Errorf("DVD-Movie format: want MKV, got %s", mv.Format)
	}
	if mv.Container != "MKV" {
		t.Errorf("DVD-Movie container: want MKV, got %s", mv.Container)
	}
	if mode, _ := mv.Options["dvd_selection_mode"].(string); mode != "main_feature" {
		t.Errorf("DVD-Movie dvd_selection_mode: want main_feature, got %q", mode)
	}
	// quality_rf round-trips through options_json (TEXT) → float64.
	if rf, _ := mv.Options["quality_rf"].(float64); rf != 18 {
		t.Errorf("DVD-Movie quality_rf: want 18, got %v", mv.Options["quality_rf"])
	}
	if p, _ := mv.Options["encoder_preset"].(string); p != "slow" {
		t.Errorf("DVD-Movie encoder_preset: want slow, got %q", p)
	}
	if !strings.HasSuffix(mv.OutputPathTemplate, ".mkv") {
		t.Errorf("DVD-Movie output template should end in .mkv: %s", mv.OutputPathTemplate)
	}

	sr := byName["DVD-Series"]
	if sr.Format != "MKV" {
		t.Errorf("DVD-Series format: want MKV, got %s", sr.Format)
	}
	if mode, _ := sr.Options["dvd_selection_mode"].(string); mode != "per_title" {
		t.Errorf("DVD-Series dvd_selection_mode: want per_title, got %q", mode)
	}
	if rf, _ := sr.Options["quality_rf"].(float64); rf != 18 {
		t.Errorf("DVD-Series quality_rf: want 18, got %v", sr.Options["quality_rf"])
	}
	if p, _ := sr.Options["encoder_preset"].(string); p != "slow" {
		t.Errorf("DVD-Series encoder_preset: want slow, got %q", p)
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

func TestSeedBDMVProfile(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	bds, err := store.ListProfilesByDiscType(context.Background(), state.DiscTypeBDMV)
	if err != nil {
		t.Fatal(err)
	}
	if len(bds) != 1 || bds[0].Name != "BD-1080p" {
		t.Errorf("BDMV profiles = %+v, want [BD-1080p]", bds)
	}
	p := bds[0]
	if p.Engine != "MakeMKV+HandBrake" {
		t.Errorf("BD-1080p engine = %q", p.Engine)
	}
	if p.Format != "MKV" {
		t.Errorf("BD-1080p format = %q", p.Format)
	}
	if p.Preset != "x265 RF 19 10-bit" {
		t.Errorf("BD-1080p preset = %q", p.Preset)
	}
	if p.StepCount != 7 {
		t.Errorf("BD-1080p step_count = %d, want 7", p.StepCount)
	}
	if !p.Enabled {
		t.Errorf("BD-1080p should be enabled")
	}
}

func TestSeedUHDProfile(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	uhds, err := store.ListProfilesByDiscType(context.Background(), state.DiscTypeUHD)
	if err != nil {
		t.Fatal(err)
	}
	if len(uhds) != 1 || uhds[0].Name != "UHD-Remux" {
		t.Errorf("UHD profiles = %+v, want [UHD-Remux]", uhds)
	}
	p := uhds[0]
	if p.Engine != "MakeMKV" {
		t.Errorf("UHD-Remux engine = %q", p.Engine)
	}
	if p.Format != "MKV" {
		t.Errorf("UHD-Remux format = %q", p.Format)
	}
	if p.Preset != "passthrough" {
		t.Errorf("UHD-Remux preset = %q", p.Preset)
	}
	if p.StepCount != 6 {
		t.Errorf("UHD-Remux step_count = %d, want 6", p.StepCount)
	}
	if !p.Enabled {
		t.Errorf("UHD-Remux should be enabled")
	}
}

func TestSeedVideoProfiles_Idempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	bds, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypeBDMV)
	if len(bds) != 1 {
		t.Errorf("BDMV after 2 loads = %d, want 1", len(bds))
	}
	uhds, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypeUHD)
	if len(uhds) != 1 {
		t.Errorf("UHD after 2 loads = %d, want 1", len(uhds))
	}
}

func TestMakeMKVBetaKey_Bootstrap(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	makemkvDir := filepath.Join(dataDir, "MakeMKV")

	env := envFn(map[string]string{
		"DISCECHO_DATA":             dataDir,
		"DISCECHO_MAKEMKV_BETA_KEY": "T-FAKEKEY-1234567890",
	})
	cfg, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MakeMKVBetaKey != "T-FAKEKEY-1234567890" {
		t.Errorf("MakeMKVBetaKey = %q", cfg.MakeMKVBetaKey)
	}
	if cfg.MakeMKVDataDir != makemkvDir {
		t.Errorf("MakeMKVDataDir = %q, want %q", cfg.MakeMKVDataDir, makemkvDir)
	}
	settingsConf := filepath.Join(makemkvDir, "settings.conf")
	b, err := os.ReadFile(settingsConf)
	if err != nil {
		t.Fatalf("read %s: %v", settingsConf, err)
	}
	if !strings.Contains(string(b), `app_Key = "T-FAKEKEY-1234567890"`) {
		t.Errorf("settings.conf content = %q", string(b))
	}
}

func TestMakeMKVBetaKey_NotSet_NoFileWritten(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	makemkvDir := filepath.Join(dataDir, "MakeMKV")
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(makemkvDir, "settings.conf")); !os.IsNotExist(err) {
		t.Errorf("settings.conf should not exist when beta key empty, err = %v", err)
	}
}

func TestSeedPSXProfile(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	ps, err := store.ListProfilesByDiscType(context.Background(), state.DiscTypePSX)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || ps[0].Name != "PSX-CHD" {
		t.Errorf("PSX profiles = %+v, want [PSX-CHD]", ps)
	}
	p := ps[0]
	if p.Engine != "redumper+chdman" {
		t.Errorf("PSX engine = %q", p.Engine)
	}
	if p.Format != "CHD" {
		t.Errorf("PSX format = %q", p.Format)
	}
	if p.StepCount != 7 {
		t.Errorf("PSX step_count = %d, want 7", p.StepCount)
	}
}

func TestSeedPS2Profile(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	ps, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypePS2)
	if len(ps) != 1 || ps[0].Name != "PS2-CHD" {
		t.Errorf("PS2 profiles = %+v, want [PS2-CHD]", ps)
	}
}

func TestSeedGameProfiles_Idempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	psx, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypePSX)
	ps2, _ := store.ListProfilesByDiscType(context.Background(), state.DiscTypePS2)
	if len(psx) != 1 {
		t.Errorf("PSX after 2 loads = %d, want 1", len(psx))
	}
	if len(ps2) != 1 {
		t.Errorf("PS2 after 2 loads = %d, want 1", len(ps2))
	}
}

func TestRedumperEnvVars_Defaults(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	cfg, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedumperBin != "redumper" {
		t.Errorf("RedumperBin = %q", cfg.RedumperBin)
	}
	if cfg.CHDManBin != "chdman" {
		t.Errorf("CHDManBin = %q", cfg.CHDManBin)
	}
	want := filepath.Join(dataDir, "redump")
	if cfg.RedumpDataDir != want {
		t.Errorf("RedumpDataDir = %q, want %q", cfg.RedumpDataDir, want)
	}
}

func TestNewMakeMKVEnvVars_Defaults(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	cfg, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MakeMKVBin != "makemkvcon" {
		t.Errorf("MakeMKVBin = %q, want makemkvcon", cfg.MakeMKVBin)
	}
	if want := filepath.Join(dataDir, "MakeMKV"); cfg.MakeMKVDataDir != want {
		t.Errorf("MakeMKVDataDir = %q, want %q", cfg.MakeMKVDataDir, want)
	}
	if cfg.BDInfoBin != "bd_info" {
		t.Errorf("BDInfoBin = %q, want bd_info", cfg.BDInfoBin)
	}
	if cfg.MakeMKVBetaKey != "" {
		t.Errorf("MakeMKVBetaKey should be empty by default, got %q", cfg.MakeMKVBetaKey)
	}
}

func TestSeedSaturnProfile_CreatesAndIsIdempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	ctx := context.Background()
	got, err := store.ListProfilesByDiscType(ctx, state.DiscTypeSAT)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Saturn-CHD" {
		t.Fatalf("expected exactly one Saturn-CHD; got %d: %#v", len(got), got)
	}
}

func TestSeedDCProfile_CreatesAndIsIdempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	ctx := context.Background()
	got, err := store.ListProfilesByDiscType(ctx, state.DiscTypeDC)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "DC-CHD" {
		t.Fatalf("expected exactly one DC-CHD; got %d: %#v", len(got), got)
	}
}

func TestSeedXboxProfile_CreatesAndIsIdempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	ctx := context.Background()
	got, err := store.ListProfilesByDiscType(ctx, state.DiscTypeXBOX)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "XBOX-ISO" {
		t.Fatalf("expected exactly one XBOX-ISO; got %d: %#v", len(got), got)
	}
}

func TestSeedDataProfile_CreatesAndIsIdempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	ctx := context.Background()
	got, err := store.ListProfilesByDiscType(ctx, state.DiscTypeData)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Data-ISO" {
		t.Fatalf("expected exactly one Data-ISO; got %d: %#v", len(got), got)
	}
}

func TestLoad_LibraryRoots_StoredKVBeatsEnv(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	// First boot: derive from DISCECHO_LIBRARY.
	env := envFn(map[string]string{
		"DISCECHO_DATA":    dataDir,
		"DISCECHO_LIBRARY": "/library",
	})
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatal(err)
	}
	// User edits movies path via the settings API (simulated).
	if err := store.SetSetting(context.Background(), "library.movies", "/srv/films"); err != nil {
		t.Fatal(err)
	}
	// Second boot: a per-root env override is set, but the stored KV
	// must win.
	env2 := envFn(map[string]string{
		"DISCECHO_DATA":           dataDir,
		"DISCECHO_LIBRARY":        "/library",
		"DISCECHO_LIBRARY_MOVIES": "/env/movies-override",
	})
	s, err := settings.Load(env2, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if s.LibraryMovies != "/srv/films" {
		t.Errorf("LibraryMovies = %q, want stored /srv/films (KV beats env)", s.LibraryMovies)
	}
	// Sibling roots still pick up env-derived defaults.
	if s.LibraryTV != "/library/tv" {
		t.Errorf("LibraryTV = %q, want /library/tv", s.LibraryTV)
	}
}

func TestLoad_LibraryRoots_EnvOverrideFreshInstall(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":          dataDir,
		"DISCECHO_LIBRARY":       "/library",
		"DISCECHO_LIBRARY_MUSIC": "/mnt/audio",
	})
	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	if s.LibraryMusic != "/mnt/audio" {
		t.Errorf("LibraryMusic = %q, want /mnt/audio", s.LibraryMusic)
	}
}

func TestLoad_LibraryRoots_LegacyPathFanout(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	// Existing v0 deployment: only library.path is stored.
	if err := store.SetSetting(context.Background(), "library.path", "/legacy"); err != nil {
		t.Fatal(err)
	}
	env := envFn(map[string]string{"DISCECHO_DATA": dataDir})
	s, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatal(err)
	}
	for got, want := range map[string]string{
		s.LibraryMovies: "/legacy/movies",
		s.LibraryTV:     "/legacy/tv",
		s.LibraryMusic:  "/legacy/music",
		s.LibraryGames:  "/legacy/games",
		s.LibraryData:   "/legacy/data",
	} {
		if got != want {
			t.Errorf("derived root = %q, want %q", got, want)
		}
	}
}

func TestSeedRetentionDefault_CreatesAndIsIdempotent(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA": dataDir,
	})
	// First Load seeds the default.
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	ctx := context.Background()
	v, err := store.GetSetting(ctx, "retention.forever")
	if err != nil {
		t.Fatalf("GetSetting: %v", err)
	}
	if v != "true" {
		t.Fatalf("expected retention.forever=true after first Load, got %q", v)
	}
	// Manually override to verify second Load does not overwrite.
	if err := store.SetSetting(ctx, "retention.forever", "false"); err != nil {
		t.Fatal(err)
	}
	if _, err := settings.Load(env, store, "test"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	v2, _ := store.GetSetting(ctx, "retention.forever")
	if v2 != "false" {
		t.Fatalf("second Load must not overwrite existing value; got %q", v2)
	}
}

func TestLoad_IGDB(t *testing.T) {
	store := openStore(t)
	dataDir := t.TempDir()
	env := envFn(map[string]string{
		"DISCECHO_DATA":               dataDir,
		"DISCECHO_IGDB_CLIENT_ID":     "abc123",
		"DISCECHO_IGDB_CLIENT_SECRET": "xyz789",
	})
	cfg, err := settings.Load(env, store, "test")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.IGDBClientID != "abc123" {
		t.Errorf("IGDBClientID = %q, want abc123", cfg.IGDBClientID)
	}
	if cfg.IGDBClientSecret != "xyz789" {
		t.Errorf("IGDBClientSecret = %q, want xyz789", cfg.IGDBClientSecret)
	}
}
