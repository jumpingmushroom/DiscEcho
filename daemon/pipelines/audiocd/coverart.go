package audiocd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// errCoverArtNotFound is returned when the CAA chain finds no cover for
// the disc. Callers (the audio-CD pipeline) log a single WARN and
// continue.
var errCoverArtNotFound = errors.New("audiocd: no cover art found")

// coverArtHTTPClient is the shared HTTP client used for cover-art and
// release-group lookups. 10-second per-request timeout keeps a stuck
// CAA mirror from extending the rip step indefinitely.
var coverArtHTTPClient = &http.Client{Timeout: 10 * time.Second}

// downloadFrontCover walks the MusicBrainz Cover Art Archive fallback
// chain for the given release MBID and writes the front cover bytes
// (as `cover.jpg`) into tmpdir, returning the absolute path. Order:
//
//  1. https://coverartarchive.org/release/<mbid>/front-500
//  2. resolve the release-group MBID via MusicBrainz JSON API
//  3. https://coverartarchive.org/release-group/<rg-mbid>/front-500
//
// Returns errCoverArtNotFound when every step misses, or the underlying
// error on any other failure. The 500px max-edge size is the right
// balance — typical embed weight is 40-100 KB, large enough for the
// player apps that consume it.
//
// userAgent is the value sent in the User-Agent header — MusicBrainz
// rate-limits anonymous clients aggressively, so use the daemon's
// configured UA (`DiscEcho/<version>`) where possible.
func downloadFrontCover(ctx context.Context, mbReleaseID, mbBaseURL, userAgent, tmpdir string) (string, error) {
	if mbReleaseID == "" {
		return "", errCoverArtNotFound
	}
	dst := filepath.Join(tmpdir, "cover.jpg")

	// 1. release-level front cover.
	if err := fetchCoverInto(ctx, coverArtReleaseURL(mbReleaseID), userAgent, dst); err == nil {
		return dst, nil
	} else if !errors.Is(err, errCoverArtNotFound) {
		return "", err
	}

	// 2. resolve release-group MBID via MusicBrainz.
	rgID, err := releaseGroupMBID(ctx, mbBaseURL, mbReleaseID, userAgent)
	if err != nil {
		return "", err
	}
	if rgID == "" {
		return "", errCoverArtNotFound
	}

	// 3. release-group front cover.
	if err := fetchCoverInto(ctx, coverArtReleaseGroupURL(rgID), userAgent, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// coverArtReleaseURL + coverArtReleaseGroupURL are vars (not funcs) so
// tests can swap the base hostname to point at an httptest server.
var coverArtReleaseURL = func(mbID string) string {
	return "https://coverartarchive.org/release/" + mbID + "/front-500"
}

var coverArtReleaseGroupURL = func(rgID string) string {
	return "https://coverartarchive.org/release-group/" + rgID + "/front-500"
}

// fetchCoverInto GETs the given URL with the supplied User-Agent header
// (CAA redirects to archive.org S3 — the client auto-follows). 404 maps
// to errCoverArtNotFound so the caller can fall through to the next
// step in the chain.
func fetchCoverInto(ctx context.Context, url, userAgent, dst string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("cover art request: %w", err)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	req.Header.Set("Accept", "image/*")

	resp, err := coverArtHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("cover art fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return errCoverArtNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cover art %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("cover art create %s: %w", dst, err)
	}
	defer func() { _ = f.Close() }()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("cover art write %s: %w", dst, err)
	}
	return nil
}

// releaseGroupMBID queries MusicBrainz for the release identified by
// mbReleaseID and returns the parent release-group MBID, or "" if the
// release has none. Errors propagate unwrapped to the caller; a 404 on
// the release itself is treated as "no group" (returns "", nil) so the
// caller surfaces errCoverArtNotFound to the user.
func releaseGroupMBID(ctx context.Context, mbBaseURL, mbReleaseID, userAgent string) (string, error) {
	if mbBaseURL == "" {
		mbBaseURL = "https://musicbrainz.org"
	}
	url := mbBaseURL + "/ws/2/release/" + mbReleaseID + "?inc=release-groups&fmt=json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("musicbrainz request: %w", err)
	}
	if userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := coverArtHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("musicbrainz fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("musicbrainz: HTTP %d", resp.StatusCode)
	}

	var body struct {
		ReleaseGroup struct {
			ID string `json:"id"`
		} `json:"release-group"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("musicbrainz decode: %w", err)
	}
	return body.ReleaseGroup.ID, nil
}
