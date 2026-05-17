package audiocd

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// TestDownloadFrontCover_ReleaseLevelHit walks the CAA chain and stops
// at step 1 when the release-level cover exists.
func TestDownloadFrontCover_ReleaseLevelHit(t *testing.T) {
	tmpdir := t.TempDir()
	// Swap the package-level HTTP client to point at the test server.
	defer swapCoverArtClient(http.DefaultClient)()

	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[r.URL.Path]++
		if strings.HasSuffix(r.URL.Path, "/front-500") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff\xe0fakejpeg"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	defer swapCoverArtClient(srv.Client())()
	defer swapReleaseURLBase(srv.URL)()
	defer swapReleaseGroupURLBase(srv.URL)()

	got, err := downloadFrontCover(context.Background(), "release-mb-id", "", "DiscEcho-test/1", tmpdir)
	if err != nil {
		t.Fatalf("downloadFrontCover: %v", err)
	}
	if got != filepath.Join(tmpdir, "cover.jpg") {
		t.Errorf("dst path: got %q", got)
	}
	data, err := os.ReadFile(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("cover.jpg is empty")
	}
	// Only the release-level path should have been hit — no fall-through.
	if hits["/release/release-mb-id/front-500"] != 1 {
		t.Errorf("release-level hit count: got %v", hits)
	}
	if hits["/release-group/release-mb-id/front-500"] != 0 {
		t.Errorf("release-group fall-through should not have fired: %v", hits)
	}
}

// TestDownloadFrontCover_FallsBackToReleaseGroup exercises the MB
// release-group lookup when the release-level cover 404s.
func TestDownloadFrontCover_FallsBackToReleaseGroup(t *testing.T) {
	tmpdir := t.TempDir()

	var caaHits atomic.Int32
	var rgFetched atomic.Bool

	caa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		caaHits.Add(1)
		switch {
		case strings.HasPrefix(r.URL.Path, "/release/"):
			http.NotFound(w, r)
		case strings.HasPrefix(r.URL.Path, "/release-group/"):
			rgFetched.Store(true)
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write([]byte("\xff\xd8\xff\xe0fakejpeg-rg"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer caa.Close()

	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Just enough JSON for the decoder to pull the release-group MBID out.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"release-group":{"id":"rg-mb-id"}}`))
	}))
	defer mb.Close()

	defer swapCoverArtClient(caa.Client())()
	defer swapReleaseURLBase(caa.URL)()
	defer swapReleaseGroupURLBase(caa.URL)()

	got, err := downloadFrontCover(context.Background(),
		"release-mb-id", mb.URL, "DiscEcho-test/1", tmpdir)
	if err != nil {
		t.Fatalf("downloadFrontCover: %v", err)
	}
	if !rgFetched.Load() {
		t.Errorf("release-group fallback path was not exercised")
	}
	data, _ := os.ReadFile(got)
	if !strings.Contains(string(data), "fakejpeg-rg") {
		t.Errorf("cover bytes don't match the release-group fixture: %q", string(data))
	}
}

// TestDownloadFrontCover_NotFoundEverywhere maps to errCoverArtNotFound
// so the caller can WARN + continue without false-flagging a network error.
func TestDownloadFrontCover_NotFoundEverywhere(t *testing.T) {
	tmpdir := t.TempDir()

	caa := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
	defer caa.Close()
	mb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"release-group":{"id":""}}`))
	}))
	defer mb.Close()

	defer swapCoverArtClient(caa.Client())()
	defer swapReleaseURLBase(caa.URL)()
	defer swapReleaseGroupURLBase(caa.URL)()

	_, err := downloadFrontCover(context.Background(),
		"release-mb-id", mb.URL, "DiscEcho-test/1", tmpdir)
	if !errors.Is(err, errCoverArtNotFound) {
		t.Errorf("want errCoverArtNotFound, got %v", err)
	}
}

func TestDownloadFrontCover_EmptyMBIDIsImmediateNotFound(t *testing.T) {
	_, err := downloadFrontCover(context.Background(), "", "", "", t.TempDir())
	if !errors.Is(err, errCoverArtNotFound) {
		t.Errorf("want errCoverArtNotFound, got %v", err)
	}
}

// --- test-only seams ---

// The cover-art helper hard-codes the CAA hostnames. The tests rerouting
// to httptest.Server URLs need a way to override them — we swap the
// package-level URL builders for the duration of the test and restore
// on cleanup.
//
// These are deliberately in this test file, not the package, so they
// never affect production behavior.

func swapCoverArtClient(c *http.Client) func() {
	old := coverArtHTTPClient
	coverArtHTTPClient = c
	return func() { coverArtHTTPClient = old }
}

func swapReleaseURLBase(base string) func() {
	old := coverArtReleaseURL
	coverArtReleaseURLOverride := func(mbID string) string {
		return base + "/release/" + mbID + "/front-500"
	}
	coverArtReleaseURL = coverArtReleaseURLOverride
	return func() { coverArtReleaseURL = old }
}

func swapReleaseGroupURLBase(base string) func() {
	old := coverArtReleaseGroupURL
	coverArtReleaseGroupURLOverride := func(rgID string) string {
		return base + "/release-group/" + rgID + "/front-500"
	}
	coverArtReleaseGroupURL = coverArtReleaseGroupURLOverride
	return func() { coverArtReleaseGroupURL = old }
}
