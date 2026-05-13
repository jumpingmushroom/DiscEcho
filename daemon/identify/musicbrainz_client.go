package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// MusicBrainzConfig configures a MusicBrainz client.
type MusicBrainzConfig struct {
	BaseURL     string        // default https://musicbrainz.org
	UserAgent   string        // required by MB TOS
	HTTPClient  *http.Client  // default {Timeout: 30s}
	MinInterval time.Duration // 0 disables; production should set 1 * time.Second
}

// NewMusicBrainzClient constructs a MusicBrainzClient. Lookup is
// goroutine-safe; the rate limiter serializes calls per client
// instance.
func NewMusicBrainzClient(c MusicBrainzConfig) MusicBrainzClient {
	if c.BaseURL == "" {
		c.BaseURL = "https://musicbrainz.org"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &mbClient{cfg: c}
}

type mbClient struct {
	cfg     MusicBrainzConfig
	mu      sync.Mutex
	lastReq time.Time
}

// Lookup queries /ws/2/discid/{id}. 404 returns ([]Candidate{}, nil).
// Other non-2xx return an error. Network failures and context
// cancellation propagate as-is.
func (c *mbClient) Lookup(ctx context.Context, discID string) ([]state.Candidate, error) {
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, err
	}

	u := strings.TrimRight(c.cfg.BaseURL, "/") +
		"/ws/2/discid/" + discID +
		"?fmt=json&inc=artist-credits+releases"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("musicbrainz: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw mbDiscIDResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return raw.toCandidates(), nil
}

// ReleaseDetails fetches /ws/2/release/{mbid}?inc=recordings+labels
// and returns label/catalog/track list. Track durations from MB are
// in milliseconds; converted to seconds here.
func (c *mbClient) ReleaseDetails(ctx context.Context, mbid string) (AudioCDMetadata, error) {
	if mbid == "" {
		return AudioCDMetadata{}, nil
	}
	if err := c.waitForRateLimit(ctx); err != nil {
		return AudioCDMetadata{}, err
	}

	u := strings.TrimRight(c.cfg.BaseURL, "/") +
		"/ws/2/release/" + mbid +
		"?fmt=json&inc=recordings+labels"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return AudioCDMetadata{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return AudioCDMetadata{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return AudioCDMetadata{}, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return AudioCDMetadata{}, fmt.Errorf("musicbrainz release: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw mbReleaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return AudioCDMetadata{}, fmt.Errorf("decode release: %w", err)
	}

	out := AudioCDMetadata{}
	if len(raw.LabelInfo) > 0 {
		out.Label = raw.LabelInfo[0].Label.Name
		out.CatalogNumber = raw.LabelInfo[0].CatalogNumber
	}
	if len(raw.Media) > 0 {
		for _, t := range raw.Media[0].Tracks {
			out.Tracks = append(out.Tracks, AudioTrack{
				Number:          t.Position,
				Title:           t.Title,
				DurationSeconds: t.Length / 1000,
			})
		}
	}
	return out, nil
}

// mbReleaseResponse is the slice of MusicBrainz /release/{mbid} we
// care about. Length values are in milliseconds; we convert to seconds.
type mbReleaseResponse struct {
	LabelInfo []struct {
		Label struct {
			Name string `json:"name"`
		} `json:"label"`
		CatalogNumber string `json:"catalog-number"`
	} `json:"label-info"`
	Media []struct {
		Tracks []struct {
			Position int    `json:"position"`
			Title    string `json:"title"`
			Length   int    `json:"length"`
		} `json:"tracks"`
	} `json:"media"`
}

func (c *mbClient) waitForRateLimit(ctx context.Context) error {
	if c.cfg.MinInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	wait := time.Until(c.lastReq.Add(c.cfg.MinInterval))
	c.mu.Unlock()

	if wait > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}

	c.mu.Lock()
	c.lastReq = time.Now()
	c.mu.Unlock()
	return nil
}

type mbDiscIDResponse struct {
	Releases []mbRelease `json:"releases"`
}

type mbRelease struct {
	ID             string           `json:"id"`
	Title          string           `json:"title"`
	Date           string           `json:"date"`
	Score          int              `json:"score"`
	Disambiguation string           `json:"disambiguation"`
	ArtistCredit   []mbArtistCredit `json:"artist-credit"`
}

type mbArtistCredit struct {
	Artist mbArtist `json:"artist"`
}

type mbArtist struct {
	Name string `json:"name"`
}

func (r mbDiscIDResponse) toCandidates() []state.Candidate {
	out := make([]state.Candidate, 0, len(r.Releases))
	for _, rel := range r.Releases {
		c := state.Candidate{
			Source:     "MusicBrainz",
			Title:      rel.Title,
			MBID:       rel.ID,
			Confidence: rel.Score,
		}
		if rel.Disambiguation != "" {
			c.Title = rel.Title + " (" + rel.Disambiguation + ")"
		}
		if y := parseYear(rel.Date); y > 0 {
			c.Year = y
		}
		if len(rel.ArtistCredit) > 0 {
			c.Artist = rel.ArtistCredit[0].Artist.Name
		}
		out = append(out, c)
	}
	return out
}

func parseYear(s string) int {
	if len(s) < 4 {
		return 0
	}
	y, err := strconv.Atoi(s[:4])
	if err != nil {
		return 0
	}
	return y
}
