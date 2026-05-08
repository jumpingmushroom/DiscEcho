// Package settings loads environment configuration and seeds the
// database on first start: the CD-FLAC profile, the bearer token, and
// any APPRISE_URLS notifications.
package settings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Settings is the resolved runtime configuration.
type Settings struct {
	Addr                 string
	Token                string
	LibraryPath          string
	DataPath             string
	AutoConfirmSeconds   int
	WhipperBin           string
	AppriseBin           string
	EjectBin             string
	CDInfoBin            string
	CDParanoiaBin        string
	MusicBrainzBaseURL   string
	MusicBrainzUserAgent string
	TMDBKey              string
	TMDBLang             string
	SubsLang             string
	HandBrakeBin         string
	IsoInfoBin           string
	MakeMKVBin           string
	MakeMKVDataDir       string
	MakeMKVBetaKey       string
	BDInfoBin            string
	RedumperBin          string
	CHDManBin            string
	RedumpDataDir        string
	DDBin                string
}

// Load reads env vars, generates a token if needed, seeds default
// rows, and returns a *Settings. If getenv is nil, os.Getenv is used.
func Load(getenv func(string) string, store *state.Store, version string) (*Settings, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	s := &Settings{
		Addr:                 firstNonEmpty(getenv("DISCECHO_ADDR"), ":8088"),
		LibraryPath:          firstNonEmpty(getenv("DISCECHO_LIBRARY"), "/library"),
		DataPath:             firstNonEmpty(getenv("DISCECHO_DATA"), "/var/lib/discecho"),
		WhipperBin:           firstNonEmpty(getenv("DISCECHO_WHIPPER_BIN"), "whipper"),
		AppriseBin:           firstNonEmpty(getenv("DISCECHO_APPRISE_BIN"), "apprise"),
		EjectBin:             firstNonEmpty(getenv("DISCECHO_EJECT_BIN"), "eject"),
		CDInfoBin:            firstNonEmpty(getenv("DISCECHO_CDINFO_BIN"), "cd-info"),
		CDParanoiaBin:        firstNonEmpty(getenv("DISCECHO_CDPARANOIA_BIN"), "cdparanoia"),
		MusicBrainzBaseURL:   firstNonEmpty(getenv("DISCECHO_MB_BASE_URL"), "https://musicbrainz.org"),
		MusicBrainzUserAgent: fmt.Sprintf("DiscEcho/%s ( https://github.com/jumpingmushroom/DiscEcho )", version),
		TMDBKey:              getenv("DISCECHO_TMDB_KEY"),
		TMDBLang:             firstNonEmpty(getenv("DISCECHO_TMDB_LANG"), "en-US"),
		SubsLang:             firstNonEmpty(getenv("DISCECHO_SUBS_LANG"), "eng"),
		HandBrakeBin:         firstNonEmpty(getenv("DISCECHO_HANDBRAKE_BIN"), "HandBrakeCLI"),
		IsoInfoBin:           firstNonEmpty(getenv("DISCECHO_ISOINFO_BIN"), "isoinfo"),
		MakeMKVBin:           firstNonEmpty(getenv("DISCECHO_MAKEMKV_BIN"), "makemkvcon"),
		MakeMKVDataDir:       firstNonEmpty(getenv("DISCECHO_MAKEMKV_DATA"), filepath.Join(firstNonEmpty(getenv("DISCECHO_DATA"), "/var/lib/discecho"), "MakeMKV")),
		MakeMKVBetaKey:       getenv("DISCECHO_MAKEMKV_BETA_KEY"),
		BDInfoBin:            firstNonEmpty(getenv("DISCECHO_BDINFO_BIN"), "bd_info"),
		RedumperBin:          firstNonEmpty(getenv("DISCECHO_REDUMPER_BIN"), "redumper"),
		CHDManBin:            firstNonEmpty(getenv("DISCECHO_CHDMAN_BIN"), "chdman"),
		RedumpDataDir:        firstNonEmpty(getenv("DISCECHO_REDUMP_DIR"), filepath.Join(firstNonEmpty(getenv("DISCECHO_DATA"), "/var/lib/discecho"), "redump")),
		DDBin:                firstNonEmpty(getenv("DISCECHO_DD_BIN"), "dd"),
	}

	if v := getenv("DISCECHO_AUTO_CONFIRM_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			s.AutoConfirmSeconds = n
		}
	}
	if s.AutoConfirmSeconds == 0 {
		s.AutoConfirmSeconds = 8
	}

	tok, err := resolveToken(getenv("DISCECHO_TOKEN"), s.DataPath, getenv("DISCECHO_AUTH_DISABLED"))
	if err != nil {
		return nil, err
	}
	s.Token = tok

	if s.MakeMKVBetaKey != "" {
		if err := writeMakeMKVBetaKey(s.MakeMKVDataDir, s.MakeMKVBetaKey); err != nil {
			return nil, fmt.Errorf("makemkv beta key: %w", err)
		}
	}

	ctx := context.Background()
	if err := seedProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed profile: %w", err)
	}
	if err := seedDVDProfiles(ctx, store); err != nil {
		return nil, fmt.Errorf("seed DVD profiles: %w", err)
	}
	if err := seedBDMVProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed BDMV profile: %w", err)
	}
	if err := seedUHDProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed UHD profile: %w", err)
	}
	if err := seedPSXProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed PSX profile: %w", err)
	}
	if err := seedPS2Profile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed PS2 profile: %w", err)
	}
	if err := seedSaturnProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed Saturn profile: %w", err)
	}
	if err := seedDCProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed DC profile: %w", err)
	}
	if err := seedXboxProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed Xbox profile: %w", err)
	}
	if err := seedDataProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed Data profile: %w", err)
	}
	if err := seedNotifications(ctx, store, getenv("DISCECHO_APPRISE_URLS")); err != nil {
		return nil, fmt.Errorf("seed notifications: %w", err)
	}
	if err := seedRetentionDefault(ctx, store); err != nil {
		return nil, fmt.Errorf("seed retention default: %w", err)
	}
	return s, nil
}

func resolveToken(envVal, dataPath, disabled string) (string, error) {
	if disabled == "true" || disabled == "1" || disabled == "yes" {
		return "", nil // explicitly disabled — middleware passthrough
	}
	if envVal != "" {
		return envVal, nil
	}
	if err := os.MkdirAll(dataPath, 0o700); err != nil {
		return "", fmt.Errorf("mkdir data: %w", err)
	}
	tokenFile := filepath.Join(dataPath, "token")
	if b, err := os.ReadFile(tokenFile); err == nil {
		t := strings.TrimSpace(string(b))
		if t != "" {
			return t, nil
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(buf)
	if err := os.WriteFile(tokenFile, []byte(tok+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("write token: %w", err)
	}
	return tok, nil
}

const (
	cdFlacProfileName    = "CD-FLAC"
	dvdMovieProfileName  = "DVD-Movie"
	dvdSeriesProfileName = "DVD-Series"
	bdProfileName        = "BD-1080p"
	uhdProfileName       = "UHD-Remux"
	psxProfileName       = "PSX-CHD"
	ps2ProfileName       = "PS2-CHD"
	saturnProfileName    = "Saturn-CHD"
	dcProfileName        = "DC-CHD"
	xboxProfileName      = "XBOX-ISO"
	dataProfileName      = "Data-ISO"
)

func seedDVDProfiles(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeDVD)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, p := range existing {
		have[p.Name] = true
	}

	now := time.Now()
	if !have[dvdMovieProfileName] {
		p := &state.Profile{
			DiscType:           state.DiscTypeDVD,
			Name:               dvdMovieProfileName,
			Engine:             "HandBrake",
			Format:             "MP4",
			Preset:             "x264 RF 20",
			Options:            map[string]any{},
			OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mp4`,
			Enabled:            true,
			StepCount:          7,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := store.CreateProfile(ctx, p); err != nil {
			return err
		}
	}
	if !have[dvdSeriesProfileName] {
		p := &state.Profile{
			DiscType: state.DiscTypeDVD,
			Name:     dvdSeriesProfileName,
			Engine:   "HandBrake",
			Format:   "MKV",
			Preset:   "x264 RF 20 per-title",
			Options: map[string]any{
				"min_title_seconds": 300,
				"season":            1,
			},
			OutputPathTemplate: `{{.Show}}/Season {{printf "%02d" .Season}}/{{.Show}} - S{{printf "%02d" .Season}}E{{printf "%02d" .EpisodeNumber}}.mkv`,
			Enabled:            true,
			StepCount:          7,
			CreatedAt:          now,
			UpdatedAt:          now,
		}
		if err := store.CreateProfile(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

func seedProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeAudioCD)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == cdFlacProfileName {
			return nil
		}
	}
	now := time.Now()
	p := &state.Profile{
		DiscType:           state.DiscTypeAudioCD,
		Name:               cdFlacProfileName,
		Engine:             "whipper",
		Format:             "FLAC",
		Preset:             "AccurateRip · cuesheet",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Artist}}/{{.Album}} ({{.Year}})/{{printf "%02d" .TrackNumber}} - {{.Title}}.flac`,
		Enabled:            true,
		StepCount:          6,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	return store.CreateProfile(ctx, p)
}

// seedRetentionDefault sets retention.forever = "true" on first boot if not already set.
func seedRetentionDefault(ctx context.Context, store *state.Store) error {
	if v, err := store.GetSetting(ctx, "retention.forever"); err == nil && v != "" {
		return nil
	}
	return store.SetSetting(ctx, "retention.forever", "true")
}

func seedNotifications(ctx context.Context, store *state.Store, urls string) error {
	if urls == "" {
		return nil
	}
	existing, err := store.ListNotifications(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	for i, u := range strings.Split(urls, ",") {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		n := &state.Notification{
			Name:     fmt.Sprintf("env-%d", i+1),
			URL:      u,
			Triggers: "done,failed",
			Enabled:  true,
		}
		if err := store.CreateNotification(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

func writeMakeMKVBetaKey(dataDir, key string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dataDir, err)
	}
	confPath := filepath.Join(dataDir, "settings.conf")
	content := fmt.Sprintf("app_Key = %q\n", key)
	if err := os.WriteFile(confPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", confPath, err)
	}
	return nil
}

func seedBDMVProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeBDMV)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == bdProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypeBDMV,
		Name:               bdProfileName,
		Engine:             "MakeMKV+HandBrake",
		Format:             "MKV",
		Preset:             "x265 RF 19 10-bit",
		Options:            map[string]any{"min_title_seconds": 3600, "keep_all_tracks": false},
		OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mkv`,
		Enabled:            true,
		StepCount:          7,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedUHDProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeUHD)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == uhdProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypeUHD,
		Name:               uhdProfileName,
		Engine:             "MakeMKV",
		Format:             "MKV",
		Preset:             "passthrough",
		Options:            map[string]any{"min_title_seconds": 3600, "keep_all_tracks": true},
		OutputPathTemplate: `{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}) [UHD].mkv`,
		Enabled:            true,
		StepCount:          6,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedPSXProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypePSX)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == psxProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypePSX,
		Name:               psxProfileName,
		Engine:             "redumper+chdman",
		Format:             "CHD",
		Preset:             "default",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd`,
		Enabled:            true,
		StepCount:          7,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedPS2Profile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypePS2)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == ps2ProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypePS2,
		Name:               ps2ProfileName,
		Engine:             "redumper+chdman",
		Format:             "CHD",
		Preset:             "default",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd`,
		Enabled:            true,
		StepCount:          7,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedSaturnProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeSAT)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == saturnProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypeSAT,
		Name:               saturnProfileName,
		Engine:             "redumper+chdman",
		Format:             "CHD",
		Preset:             "default",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd`,
		Enabled:            true,
		StepCount:          6,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedDCProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeDC)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == dcProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypeDC,
		Name:               dcProfileName,
		Engine:             "redumper+chdman",
		Format:             "CHD",
		Preset:             "default",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).chd`,
		Enabled:            true,
		StepCount:          6,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedXboxProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeXBOX)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == xboxProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypeXBOX,
		Name:               xboxProfileName,
		Engine:             "redumper",
		Format:             "ISO",
		Preset:             "default",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}} ({{.Region}})/{{.Title}} ({{.Region}}).iso`,
		Enabled:            true,
		StepCount:          5,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func seedDataProfile(ctx context.Context, store *state.Store) error {
	existing, err := store.ListProfilesByDiscType(ctx, state.DiscTypeData)
	if err != nil {
		return err
	}
	for _, p := range existing {
		if p.Name == dataProfileName {
			return nil
		}
	}
	now := time.Now()
	return store.CreateProfile(ctx, &state.Profile{
		DiscType:           state.DiscTypeData,
		Name:               dataProfileName,
		Engine:             "dd",
		Format:             "ISO",
		Preset:             "default",
		Options:            map[string]any{},
		OutputPathTemplate: `{{.Title}}/{{.Title}}.iso`,
		Enabled:            true,
		StepCount:          5,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
