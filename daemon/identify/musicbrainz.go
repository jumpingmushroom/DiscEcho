package identify

import (
	"context"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// MusicBrainzClient looks up release candidates by disc ID and fetches
// extended release records by MBID. Implementations: mbClient
// (production HTTP), fakes (tests).
type MusicBrainzClient interface {
	Lookup(ctx context.Context, discID string) ([]state.Candidate, error)
	// ReleaseDetails fetches /ws/2/release/{mbid}?inc=recordings+labels
	// and returns the label, catalog number, and track list for the
	// pane's audio CD layout. Persisted into disc.metadata_json at
	// /api/discs/{id}/start.
	ReleaseDetails(ctx context.Context, mbid string) (AudioCDMetadata, error)
	// SearchByName runs a free-text search against the MB release
	// index and returns up to 25 candidates ranked by MB's relevance
	// score (0-100). Used by /api/discs/{id}/identify on AUDIO_CD discs
	// when the discID Lookup returned no matches and the user falls
	// back to a manual query. Empty results return (nil, nil).
	SearchByName(ctx context.Context, query string) ([]state.Candidate, error)
}

// AudioCDMetadata is the extended audio-CD payload the pane needs.
// Persisted into disc.metadata_json at /api/discs/{id}/start.
type AudioCDMetadata struct {
	Label         string       `json:"label,omitempty"`
	CatalogNumber string       `json:"catalog_number,omitempty"`
	Tracks        []AudioTrack `json:"tracks,omitempty"`
	CoverURL      string       `json:"cover_url,omitempty"`
	// ReleaseGroupMBID lets the UI fall back to the release-group's
	// cover-art when the specific release has none. Cover Art Archive
	// serves art at both /release/{id}/front and /release-group/{id}/front,
	// and the release-group level is populated far more often (a single
	// album cover used across reissues, regional pressings, etc.).
	ReleaseGroupMBID string `json:"release_group_mbid,omitempty"`
}

// AudioTrack is one entry in AudioCDMetadata.Tracks. Named to avoid a
// collision with toc.Track in the same package.
type AudioTrack struct {
	Number          int    `json:"number"`
	Title           string `json:"title"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
}
