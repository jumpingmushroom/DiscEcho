package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// IGDBConfig configures an IGDBClient. ClientID + ClientSecret are
// Twitch app credentials (https://dev.twitch.tv/console/apps); leaving
// either blank disables the client (Configured() returns false).
type IGDBConfig struct {
	ClientID     string
	ClientSecret string
	BaseURL      string        // default "https://api.igdb.com/v4"
	TokenURL     string        // default "https://id.twitch.tv/oauth2/token"
	HTTPClient   *http.Client  // default {Timeout: 20s}
	MinInterval  time.Duration // 0 disables throttling; production uses 250ms (4 req/s ceiling)
}

// IGDBClient is the public surface for game-disc manual search.
type IGDBClient interface {
	SearchGames(ctx context.Context, query string, discType state.DiscType) ([]state.Candidate, error)
	GameDetails(ctx context.Context, igdbID int) (*IGDBGameDetails, error)
	Configured() bool
}

// IGDBGameDetails is the per-pick metadata used to populate
// disc.metadata_json. CoverURL is rewritten to https + t_cover_big size
// for dashboard rendering.
type IGDBGameDetails struct {
	ID        int      `json:"id"`
	Name      string   `json:"name"`
	Year      int      `json:"year"`
	CoverURL  string   `json:"cover_url"`
	Summary   string   `json:"summary"`
	Platforms []string `json:"platforms"`
}

// igdbPlatformID maps DiscEcho disc types to IGDB platform IDs.
var igdbPlatformID = map[state.DiscType]int{
	state.DiscTypePSX:  7,
	state.DiscTypePS2:  8,
	state.DiscTypeXBOX: 11,
	state.DiscTypeDC:   23,
	state.DiscTypeSAT:  32,
}

// NewIGDBClient constructs an IGDBClient. Empty credentials produce a
// Configured()==false client that errors on every call; callers should
// short-circuit via Configured() and return 503 to the caller.
func NewIGDBClient(c IGDBConfig) IGDBClient {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.igdb.com/v4"
	}
	if c.TokenURL == "" {
		c.TokenURL = "https://id.twitch.tv/oauth2/token"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &igdbClient{cfg: c}
}

type igdbClient struct {
	cfg     IGDBConfig
	mu      sync.Mutex
	lastReq time.Time

	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
}

func (c *igdbClient) Configured() bool {
	return c.cfg.ClientID != "" && c.cfg.ClientSecret != ""
}

func (c *igdbClient) SearchGames(ctx context.Context, query string, discType state.DiscType) ([]state.Candidate, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("igdb: client_id or client_secret unset")
	}
	platformID, ok := igdbPlatformID[discType]
	if !ok {
		return nil, fmt.Errorf("igdb: no platform mapping for disc type %s", discType)
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	body := fmt.Sprintf(
		`search %q; fields name,first_release_date,cover.url; where platforms = (%d); limit 10;`,
		query, platformID,
	)

	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	tok, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.doAPI(ctx, "/games", body, tok)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("igdb: status %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}

	var raw []struct {
		ID               int    `json:"id"`
		Name             string `json:"name"`
		FirstReleaseDate int64  `json:"first_release_date"`
		Cover            struct {
			URL string `json:"url"`
		} `json:"cover"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("igdb: decode: %w", err)
	}

	out := make([]state.Candidate, 0, len(raw))
	for _, g := range raw {
		year := 0
		if g.FirstReleaseDate > 0 {
			year = time.Unix(g.FirstReleaseDate, 0).UTC().Year()
		}
		out = append(out, state.Candidate{
			Source:     "IGDB",
			Title:      g.Name,
			Year:       year,
			Confidence: 25, // never auto-rip IGDB picks (below batch threshold)
			IGDBID:     g.ID,
		})
	}
	return out, nil
}

func (c *igdbClient) GameDetails(ctx context.Context, igdbID int) (*IGDBGameDetails, error) {
	if !c.Configured() {
		return nil, fmt.Errorf("igdb: client_id or client_secret unset")
	}
	body := fmt.Sprintf(
		`fields name,first_release_date,cover.url,summary,platforms.name; where id = %d;`,
		igdbID,
	)
	if err := c.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	tok, err := c.getToken(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := c.doAPI(ctx, "/games", body, tok)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("igdb: status %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}

	var raw []struct {
		ID               int    `json:"id"`
		Name             string `json:"name"`
		FirstReleaseDate int64  `json:"first_release_date"`
		Summary          string `json:"summary"`
		Cover            struct {
			URL string `json:"url"`
		} `json:"cover"`
		Platforms []struct {
			Name string `json:"name"`
		} `json:"platforms"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("igdb: decode: %w", err)
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("igdb: no game with id %d", igdbID)
	}
	g := raw[0]
	year := 0
	if g.FirstReleaseDate > 0 {
		year = time.Unix(g.FirstReleaseDate, 0).UTC().Year()
	}
	platforms := make([]string, 0, len(g.Platforms))
	for _, p := range g.Platforms {
		platforms = append(platforms, p.Name)
	}
	return &IGDBGameDetails{
		ID:        g.ID,
		Name:      g.Name,
		Year:      year,
		CoverURL:  rewriteCoverURL(g.Cover.URL, "t_cover_big"),
		Summary:   g.Summary,
		Platforms: platforms,
	}, nil
}

// doAPI sends a POST to {BaseURL}{path} with the Apicalypse body. The
// IGDB v4 API uses text/plain for query bodies, not application/json.
func (c *igdbClient) doAPI(ctx context.Context, path, body, tok string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.BaseURL, "/")+path,
		strings.NewReader(body),
	)
	if err != nil {
		return nil, fmt.Errorf("igdb: build request: %w", err)
	}
	req.Header.Set("Client-ID", c.cfg.ClientID)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "text/plain")
	return c.cfg.HTTPClient.Do(req)
}

// getToken returns a valid access token, refreshing if expired or within
// the 5-minute refresh-before-expiry window. Concurrent callers share
// the token via the mutex.
func (c *igdbClient) getToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" && time.Until(c.tokenExpiry) > 5*time.Minute {
		return c.token, nil
	}
	form := url.Values{}
	form.Set("client_id", c.cfg.ClientID)
	form.Set("client_secret", c.cfg.ClientSecret)
	form.Set("grant_type", "client_credentials")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.cfg.TokenURL+"?"+form.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("igdb: build token request: %w", err)
	}
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("igdb: token status %d: %s", resp.StatusCode, strings.TrimSpace(string(buf)))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", fmt.Errorf("igdb: decode token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("igdb: empty access_token")
	}
	c.token = tr.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	return c.token, nil
}

func (c *igdbClient) waitForRateLimit(ctx context.Context) error {
	if c.cfg.MinInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	wait := time.Until(c.lastReq.Add(c.cfg.MinInterval))
	c.lastReq = time.Now()
	c.mu.Unlock()
	if wait <= 0 {
		return nil
	}
	select {
	case <-time.After(wait):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// rewriteCoverURL converts IGDB protocol-relative thumbnail URLs to
// absolute https URLs with the requested size token. IGDB returns
// "//images.igdb.com/igdb/image/upload/t_thumb/abc.jpg"; we want
// "https://images.igdb.com/igdb/image/upload/t_cover_big/abc.jpg".
func rewriteCoverURL(raw, sizeToken string) string {
	if raw == "" {
		return ""
	}
	abs := raw
	if strings.HasPrefix(abs, "//") {
		abs = "https:" + abs
	}
	for _, token := range []string{"t_thumb", "t_cover_small", "t_screenshot_med", "t_logo_med"} {
		abs = strings.Replace(abs, "/"+token+"/", "/"+sizeToken+"/", 1)
	}
	return abs
}
