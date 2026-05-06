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
	}

	if v := getenv("DISCECHO_AUTO_CONFIRM_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			s.AutoConfirmSeconds = n
		}
	}
	if s.AutoConfirmSeconds == 0 {
		s.AutoConfirmSeconds = 8
	}

	tok, err := resolveToken(getenv("DISCECHO_TOKEN"), s.DataPath)
	if err != nil {
		return nil, err
	}
	s.Token = tok

	ctx := context.Background()
	if err := seedProfile(ctx, store); err != nil {
		return nil, fmt.Errorf("seed profile: %w", err)
	}
	if err := seedNotifications(ctx, store, getenv("DISCECHO_APPRISE_URLS")); err != nil {
		return nil, fmt.Errorf("seed notifications: %w", err)
	}
	return s, nil
}

func resolveToken(envVal, dataPath string) (string, error) {
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

const cdFlacProfileName = "CD-FLAC"

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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
