package identify

import (
	"context"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// MusicBrainzClient looks up release candidates by disc ID.
// Implementations: mbClient (production HTTP), fakes (tests).
type MusicBrainzClient interface {
	Lookup(ctx context.Context, discID string) ([]state.Candidate, error)
}
