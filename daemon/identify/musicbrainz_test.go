package identify_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
)

func TestMusicBrainz_Lookup_ReturnsCandidates(t *testing.T) {
	body, err := os.ReadFile("testdata/musicbrainz-kindofblue.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("fmt") != "json" {
			t.Errorf("expected fmt=json, got %q", r.URL.Query().Get("fmt"))
		}
		if r.UserAgent() == "" {
			t.Errorf("expected non-empty User-Agent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL:     srv.URL,
		UserAgent:   "DiscEcho-test/0.0",
		HTTPClient:  &http.Client{Timeout: 5 * time.Second},
		MinInterval: 0,
	})

	cands, err := c.Lookup(context.Background(), "abc123-disc-id")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(cands))
	}
	if cands[0].Title != "Kind of Blue" || cands[0].Year != 1959 {
		t.Errorf("cand[0] mismatch: %+v", cands[0])
	}
	if cands[0].Artist != "Miles Davis" {
		t.Errorf("artist mismatch: %q", cands[0].Artist)
	}
	if cands[0].Confidence != 94 {
		t.Errorf("confidence: want 94, got %d", cands[0].Confidence)
	}
	if cands[0].Source != "MusicBrainz" {
		t.Errorf("source: want MusicBrainz, got %q", cands[0].Source)
	}
	if cands[0].MBID != "kb-1959" {
		t.Errorf("MBID: %q", cands[0].MBID)
	}
}

func TestMusicBrainz_Lookup_NotFound_ReturnsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"Not Found"}`))
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "x",
	})
	cands, err := c.Lookup(context.Background(), "no-such-id")
	if err != nil {
		t.Errorf("404 should return empty list, not error: %v", err)
	}
	if len(cands) != 0 {
		t.Errorf("want empty, got %d", len(cands))
	}
}

func TestMusicBrainz_Lookup_ServerError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "x",
	})
	_, err := c.Lookup(context.Background(), "x")
	if err == nil {
		t.Errorf("want error on 500")
	}
}

func TestMusicBrainz_Lookup_ContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "x",
	})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := c.Lookup(ctx, "x")
	if err == nil || (!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)) {
		t.Errorf("want context error, got %v", err)
	}
}

func TestMusicBrainz_RateLimit_DelaysSecondCall(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"releases":[]}`))
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "x",
		MinInterval: 100 * time.Millisecond,
	})

	start := time.Now()
	_, _ = c.Lookup(context.Background(), "a")
	_, _ = c.Lookup(context.Background(), "b")
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("rate limit not enforced: elapsed %v", elapsed)
	}
	if calls != 2 {
		t.Errorf("want 2 calls, got %d", calls)
	}
}
