package identify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestIGDBClient_TokenCached(t *testing.T) {
	var tokenCalls int32
	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenCalls, 1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-token-abc",
			"expires_in":   3600,
			"token_type":   "bearer",
		})
	}))
	defer tokenSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token-abc" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Client-ID"); got != "client-id-xyz" {
			t.Errorf("Client-ID = %q", got)
		}
		_, _ = w.Write([]byte(`[{"id":1,"name":"Sly 3"}]`))
	}))
	defer apiSrv.Close()

	c := identify.NewIGDBClient(identify.IGDBConfig{
		ClientID:     "client-id-xyz",
		ClientSecret: "secret-pqr",
		BaseURL:      apiSrv.URL,
		TokenURL:     tokenSrv.URL,
		MinInterval:  time.Millisecond,
	})

	for i := 0; i < 3; i++ {
		if _, err := c.SearchGames(context.Background(), "sly", state.DiscTypePS2); err != nil {
			t.Fatalf("SearchGames iter %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&tokenCalls); got != 1 {
		t.Errorf("token endpoint called %d times, want 1 (cached)", got)
	}
}

func TestIGDBClient_NotConfigured(t *testing.T) {
	c := identify.NewIGDBClient(identify.IGDBConfig{}) // empty client_id/secret
	if c.Configured() {
		t.Error("Configured() = true, want false for empty credentials")
	}
	_, err := c.SearchGames(context.Background(), "sly", state.DiscTypePS2)
	if err == nil {
		t.Error("SearchGames returned nil error with no credentials")
	}
}

func TestIGDBClient_SearchGames_CandidateMapping(t *testing.T) {
	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := mustReadAll(t, r.Body)
		// Apicalypse body should filter by PS2 platform (id 8).
		if !strings.Contains(body, "where platforms = (8)") {
			t.Errorf("body lacks PS2 platform filter: %s", body)
		}
		_, _ = w.Write([]byte(`[
			{"id":12345,"name":"Sly 3: Honor Among Thieves","first_release_date":1130630400,
			 "cover":{"url":"//images.igdb.com/igdb/image/upload/t_thumb/abc.jpg"}}
		]`))
	}))
	defer apiSrv.Close()

	tokenSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "tok", "expires_in": 3600})
	}))
	defer tokenSrv.Close()

	c := identify.NewIGDBClient(identify.IGDBConfig{
		ClientID: "x", ClientSecret: "y",
		BaseURL: apiSrv.URL, TokenURL: tokenSrv.URL,
		MinInterval: time.Millisecond,
	})

	cands, err := c.SearchGames(context.Background(), "sly 3", state.DiscTypePS2)
	if err != nil {
		t.Fatalf("SearchGames: %v", err)
	}
	if len(cands) != 1 {
		t.Fatalf("len(cands) = %d, want 1", len(cands))
	}
	got := cands[0]
	if got.Source != "IGDB" {
		t.Errorf("Source = %q", got.Source)
	}
	if got.Title != "Sly 3: Honor Among Thieves" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Year != 2005 {
		t.Errorf("Year = %d, want 2005 (1130630400 unix = 2005-10-30)", got.Year)
	}
	if got.Confidence != 25 {
		t.Errorf("Confidence = %d, want 25 (below batch auto-confirm threshold)", got.Confidence)
	}
	if got.IGDBID != 12345 {
		t.Errorf("IGDBID = %d", got.IGDBID)
	}
}

func mustReadAll(t *testing.T, r interface{ Read(p []byte) (int, error) }) string {
	t.Helper()
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			sb.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	return sb.String()
}
