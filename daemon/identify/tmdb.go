package identify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// TMDBConfig configures a TMDB client.
type TMDBConfig struct {
	APIKey     string       // empty → all Search calls return ([], nil) without HTTP
	BaseURL    string       // default https://api.themoviedb.org/3
	Language   string       // default en-US
	HTTPClient *http.Client // default {Timeout: 30s}
}

// TMDBClient looks up movie / tv release candidates by free-text query.
type TMDBClient interface {
	SearchMovie(ctx context.Context, query string) ([]state.Candidate, error)
	SearchTV(ctx context.Context, query string) ([]state.Candidate, error)
	SearchBoth(ctx context.Context, query string) ([]state.Candidate, error)
	// MovieRuntime fetches `/movie/{id}` and returns the runtime in
	// seconds. Returns (0, nil) when the API is not configured or
	// TMDB doesn't know the runtime; only network / decode errors
	// produce non-nil error.
	MovieRuntime(ctx context.Context, tmdbID int) (int, error)
}

const tmdbCandidateCap = 5

// NewTMDBClient constructs a TMDBClient.
func NewTMDBClient(c TMDBConfig) TMDBClient {
	if c.BaseURL == "" {
		c.BaseURL = "https://api.themoviedb.org/3"
	}
	if c.Language == "" {
		c.Language = "en-US"
	}
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &tmdbClient{cfg: c}
}

type tmdbClient struct{ cfg TMDBConfig }

// MovieRuntime fetches `/movie/{id}` to read the canonical runtime
// in minutes, returns it in seconds. Search endpoints don't include
// runtime, so this is called on a per-pick basis when the user
// starts a rip.
func (c *tmdbClient) MovieRuntime(ctx context.Context, tmdbID int) (int, error) {
	if c.cfg.APIKey == "" || tmdbID <= 0 {
		return 0, nil
	}
	endpoint := fmt.Sprintf("/movie/%d", tmdbID)
	u, err := url.Parse(strings.TrimRight(c.cfg.BaseURL, "/") + endpoint)
	if err != nil {
		return 0, fmt.Errorf("build url: %w", err)
	}
	q := u.Query()
	q.Set("api_key", c.cfg.APIKey)
	q.Set("language", c.cfg.Language)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return 0, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return 0, fmt.Errorf("tmdb movie/%d: status %d: %s", tmdbID, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var detail struct {
		Runtime int `json:"runtime"` // minutes; may be 0 or null for unknown
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return 0, fmt.Errorf("decode movie response: %w", err)
	}
	return detail.Runtime * 60, nil
}

func (c *tmdbClient) SearchMovie(ctx context.Context, query string) ([]state.Candidate, error) {
	return c.search(ctx, "/search/movie", "movie", query, parseTMDBMovie)
}

func (c *tmdbClient) SearchTV(ctx context.Context, query string) ([]state.Candidate, error) {
	return c.search(ctx, "/search/tv", "tv", query, parseTMDBTV)
}

// SearchBoth runs movie + tv searches in parallel, merges, sorts by
// confidence DESC, caps at 5.
func (c *tmdbClient) SearchBoth(ctx context.Context, query string) ([]state.Candidate, error) {
	if c.cfg.APIKey == "" {
		return nil, nil
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		out      []state.Candidate
		movieErr error
		tvErr    error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		cands, err := c.SearchMovie(ctx, query)
		mu.Lock()
		out = append(out, cands...)
		movieErr = err
		mu.Unlock()
	}()
	go func() {
		defer wg.Done()
		cands, err := c.SearchTV(ctx, query)
		mu.Lock()
		out = append(out, cands...)
		tvErr = err
		mu.Unlock()
	}()
	wg.Wait()

	if movieErr != nil && tvErr != nil {
		return nil, fmt.Errorf("tmdb both: movie=%w; tv=%v", movieErr, tvErr)
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Confidence > out[j].Confidence
	})
	if len(out) > tmdbCandidateCap {
		out = out[:tmdbCandidateCap]
	}
	return out, nil
}

type tmdbSearchResponse struct {
	Results []json.RawMessage `json:"results"`
}

type tmdbMovie struct {
	ID          int     `json:"id"`
	Title       string  `json:"title"`
	ReleaseDate string  `json:"release_date"`
	Popularity  float64 `json:"popularity"`
}

type tmdbTV struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	FirstAirDate string  `json:"first_air_date"`
	Popularity   float64 `json:"popularity"`
}

func parseTMDBMovie(raw json.RawMessage) (state.Candidate, error) {
	var m tmdbMovie
	if err := json.Unmarshal(raw, &m); err != nil {
		return state.Candidate{}, err
	}
	return state.Candidate{
		Source:     "TMDB",
		Title:      m.Title,
		Year:       parseYear(m.ReleaseDate),
		Confidence: int(math.Round(math.Min(m.Popularity/10, 100))),
		TMDBID:     m.ID,
		MediaType:  "movie",
	}, nil
}

func parseTMDBTV(raw json.RawMessage) (state.Candidate, error) {
	var t tmdbTV
	if err := json.Unmarshal(raw, &t); err != nil {
		return state.Candidate{}, err
	}
	return state.Candidate{
		Source:     "TMDB",
		Title:      t.Name,
		Year:       parseYear(t.FirstAirDate),
		Confidence: int(math.Round(math.Min(t.Popularity/10, 100))),
		TMDBID:     t.ID,
		MediaType:  "tv",
	}, nil
}

// search runs one TMDB endpoint and parses with the given parser. 404
// → empty + nil; other non-2xx → error.
func (c *tmdbClient) search(
	ctx context.Context,
	endpoint, mediaType, query string,
	parser func(json.RawMessage) (state.Candidate, error),
) ([]state.Candidate, error) {
	if c.cfg.APIKey == "" {
		return nil, nil
	}
	u, err := url.Parse(strings.TrimRight(c.cfg.BaseURL, "/") + endpoint)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}
	q := u.Query()
	q.Set("api_key", c.cfg.APIKey)
	q.Set("query", query)
	q.Set("language", c.cfg.Language)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
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
		return nil, fmt.Errorf("tmdb %s: status %d: %s", mediaType, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw tmdbSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	out := make([]state.Candidate, 0, len(raw.Results))
	for _, r := range raw.Results {
		c, err := parser(r)
		if err != nil {
			continue
		}
		out = append(out, c)
	}
	if len(out) > tmdbCandidateCap {
		out = out[:tmdbCandidateCap]
	}
	return out, nil
}
