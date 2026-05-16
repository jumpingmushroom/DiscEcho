package identify_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
	// Multiple-release responses get confidence=0 so the
	// AwaitingDecision card forces a manual pick instead of
	// auto-confirming one of an ambiguous set. The MB `score` field
	// isn't meaningful for the discid resource — it's a search concept.
	if cands[0].Confidence != 0 {
		t.Errorf("confidence: want 0 (multi-release), got %d", cands[0].Confidence)
	}
	if cands[0].Source != "MusicBrainz" {
		t.Errorf("source: want MusicBrainz, got %q", cands[0].Source)
	}
	if cands[0].MBID != "kb-1959" {
		t.Errorf("MBID: %q", cands[0].MBID)
	}
}

// TestMusicBrainz_Lookup_SingleReleaseIsConfident covers the
// auto-confirm path: when the disc-id lookup returns exactly one
// release, the candidate is marked confident (100) so the
// AwaitingDecision card can start the rip without a manual pick.
func TestMusicBrainz_Lookup_SingleReleaseIsConfident(t *testing.T) {
	body := []byte(`{"releases":[{"id":"mb-1","title":"Solo","date":"2020-01-01","artist-credit":[{"artist":{"name":"Solo Artist"}}]}]}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "x",
	})
	cands, err := c.Lookup(context.Background(), "solo-disc-id")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("want 1 candidate, got %d", len(cands))
	}
	if cands[0].Confidence != 100 {
		t.Errorf("confidence: want 100 (single release), got %d", cands[0].Confidence)
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

func TestMusicBrainz_ReleaseDetails(t *testing.T) {
	body, _ := os.ReadFile("testdata/mb-release-tracks.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/release/1a0ba71b-fb23-3931-a426") {
			t.Errorf("path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "test/1.0",
	})
	d, err := c.ReleaseDetails(context.Background(), "1a0ba71b-fb23-3931-a426-3e4ab35f2a7c")
	if err != nil {
		t.Fatal(err)
	}
	if d.Label != "Columbia" {
		t.Errorf("label: %q", d.Label)
	}
	if d.CatalogNumber != "CL 1355" {
		t.Errorf("catalog: %q", d.CatalogNumber)
	}
	if len(d.Tracks) != 5 {
		t.Fatalf("want 5 tracks, got %d", len(d.Tracks))
	}
	if d.Tracks[0].Number != 1 || d.Tracks[0].Title != "So What" || d.Tracks[0].DurationSeconds != 562 {
		t.Errorf("track[0]: %+v", d.Tracks[0])
	}
	if d.ReleaseGroupMBID != "7c3218d7-75e0-4e8c-971f-8a3a0a3a3a3a" {
		t.Errorf("release_group_mbid: %q", d.ReleaseGroupMBID)
	}
}

func TestMusicBrainz_ReleaseDetails_EmptyMBID(t *testing.T) {
	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: "https://example.invalid", UserAgent: "test/1.0",
	})
	d, err := c.ReleaseDetails(context.Background(), "")
	if err != nil {
		t.Errorf("empty mbid should not error: %v", err)
	}
	if d.Label != "" || len(d.Tracks) != 0 {
		t.Errorf("want empty AudioCDMetadata for empty mbid: %+v", d)
	}
}

func TestMusicBrainz_SearchByName(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/ws/2/release/") {
			t.Errorf("path: %s", r.URL.Path)
		}
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		// Two-result response: top hit at score 100, second at 73.
		_, _ = w.Write([]byte(`{
			"releases": [
				{"id":"r1","title":"Fear and Bullets","date":"1997","score":100,
				 "artist-credit":[{"artist":{"name":"Trust Obey"}}]},
				{"id":"r2","title":"Fear and Bullets","date":"1999","score":73,
				 "disambiguation":"Remastered",
				 "artist-credit":[{"artist":{"name":"Trust Obey"}}]}
			]
		}`))
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "test/1.0",
	})
	got, err := c.SearchByName(context.Background(), "Trust Obey Fear and Bullets")
	if err != nil {
		t.Fatal(err)
	}
	if capturedQuery == "" {
		t.Error("query parameter missing from request")
	}
	if len(got) != 2 {
		t.Fatalf("want 2 candidates, got %d", len(got))
	}
	if got[0].Confidence != 100 || got[0].MBID != "r1" || got[0].Artist != "Trust Obey" || got[0].Year != 1997 {
		t.Errorf("first candidate: %+v", got[0])
	}
	if got[1].Confidence != 73 || got[1].Title != "Fear and Bullets (Remastered)" {
		t.Errorf("second candidate: %+v", got[1])
	}
}

func TestMusicBrainz_SearchByName_EmptyResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"releases":[]}`))
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "test/1.0",
	})
	got, err := c.SearchByName(context.Background(), "no such album")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("want nil for empty results, got %+v", got)
	}
}

func TestMusicBrainz_SearchByName_EmptyQuery(t *testing.T) {
	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: "https://example.invalid", UserAgent: "test/1.0",
	})
	got, err := c.SearchByName(context.Background(), "   ")
	if err != nil {
		t.Errorf("empty query should not error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for empty query, got %+v", got)
	}
}

func TestMusicBrainz_SearchByName_EscapesLuceneSpecials(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("query")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"releases":[]}`))
	}))
	defer srv.Close()

	c := identify.NewMusicBrainzClient(identify.MusicBrainzConfig{
		BaseURL: srv.URL, UserAgent: "test/1.0",
	})
	_, err := c.SearchByName(context.Background(), `Bauhaus: Mask (1981)`)
	if err != nil {
		t.Fatal(err)
	}
	// The Lucene specials `:`, `(`, `)` should be backslash-escaped so
	// MB doesn't interpret `Bauhaus:` as a field selector.
	wantSubstrs := []string{`\:`, `\(`, `\)`}
	for _, s := range wantSubstrs {
		if !strings.Contains(capturedQuery, s) {
			t.Errorf("captured query %q missing escape %q", capturedQuery, s)
		}
	}
}
