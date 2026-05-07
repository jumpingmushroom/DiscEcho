package identify_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestTMDB_SearchMovie(t *testing.T) {
	body, err := os.ReadFile("testdata/tmdb-arrival-movie.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/search/movie") {
			t.Errorf("path: got %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("api_key") != "test-key" {
			t.Errorf("api_key: got %q", q.Get("api_key"))
		}
		if q.Get("query") != "Arrival" {
			t.Errorf("query: got %q", q.Get("query"))
		}
		if q.Get("language") != "en-US" {
			t.Errorf("language: got %q", q.Get("language"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := identify.NewTMDBClient(identify.TMDBConfig{
		APIKey:   "test-key",
		BaseURL:  srv.URL,
		Language: "en-US",
	})

	cands, err := c.SearchMovie(context.Background(), "Arrival")
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(cands))
	}
	if cands[0].Title != "Arrival" || cands[0].Year != 2016 {
		t.Errorf("cand[0] mismatch: %+v", cands[0])
	}
	if cands[0].MediaType != "movie" {
		t.Errorf("media_type: got %q", cands[0].MediaType)
	}
	if cands[0].TMDBID != 329865 {
		t.Errorf("tmdb_id: got %d", cands[0].TMDBID)
	}
	if cands[0].Source != "TMDB" {
		t.Errorf("source: got %q", cands[0].Source)
	}
	// Confidence: popularity 38.5 / 10 = 3.85 → rounded to 4
	if cands[0].Confidence < 3 || cands[0].Confidence > 5 {
		t.Errorf("confidence: got %d", cands[0].Confidence)
	}
}

func TestTMDB_SearchTV(t *testing.T) {
	body, err := os.ReadFile("testdata/tmdb-friends-tv.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/search/tv") {
			t.Errorf("path: got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := identify.NewTMDBClient(identify.TMDBConfig{APIKey: "x", BaseURL: srv.URL})
	cands, err := c.SearchTV(context.Background(), "Friends")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1, got %d", len(cands))
	}
	if cands[0].Title != "Friends" || cands[0].Year != 1994 {
		t.Errorf("got %+v", cands[0])
	}
	if cands[0].MediaType != "tv" {
		t.Errorf("media_type: got %q", cands[0].MediaType)
	}
}

func TestTMDB_SearchBoth_MergesAndSortsAndCaps(t *testing.T) {
	movieBody, _ := os.ReadFile("testdata/tmdb-arrival-movie.json")
	tvBody, _ := os.ReadFile("testdata/tmdb-friends-tv.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/search/movie") {
			_, _ = w.Write(movieBody)
		} else {
			_, _ = w.Write(tvBody)
		}
	}))
	defer srv.Close()

	c := identify.NewTMDBClient(identify.TMDBConfig{APIKey: "x", BaseURL: srv.URL})
	cands, err := c.SearchBoth(context.Background(), "x")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) > 5 {
		t.Errorf("should cap at 5, got %d", len(cands))
	}
	// Friends has popularity 110 → confidence ~11 vs Arrival 3-4. TV first.
	if cands[0].MediaType != "tv" {
		t.Errorf("highest confidence first: got %s", cands[0].MediaType)
	}
}

func TestTMDB_NoAPIKey_ReturnsEmpty(t *testing.T) {
	c := identify.NewTMDBClient(identify.TMDBConfig{})
	cands, err := c.SearchMovie(context.Background(), "Arrival")
	if err != nil {
		t.Errorf("no key should return empty, not error: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("got %d candidates with no api key", len(cands))
	}
}

func TestTMDB_404_ReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	c := identify.NewTMDBClient(identify.TMDBConfig{APIKey: "x", BaseURL: srv.URL})
	cands, err := c.SearchMovie(context.Background(), "x")
	if err != nil {
		t.Errorf("404 should return empty, got %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("got %d", len(cands))
	}
}

func TestTMDB_5xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := identify.NewTMDBClient(identify.TMDBConfig{APIKey: "x", BaseURL: srv.URL})
	_, err := c.SearchMovie(context.Background(), "x")
	if err == nil {
		t.Errorf("want error on 500")
	}
}

func TestTMDB_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c := identify.NewTMDBClient(identify.TMDBConfig{APIKey: "x", BaseURL: srv.URL})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.SearchMovie(ctx, "x")
	if err == nil {
		t.Errorf("want context error")
	}
}

// keep url package alive for goimports
var _ = url.Parse
