package api

import (
	"sync"

	"github.com/jumpingmushroom/DiscEcho/daemon/identify"
	"github.com/jumpingmushroom/DiscEcho/daemon/jobs"
	"github.com/jumpingmushroom/DiscEcho/daemon/pipelines"
	"github.com/jumpingmushroom/DiscEcho/daemon/settings"
	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

// Handlers bundles every dependency the API endpoints need. Constructed
// in main.go and passed to NewRouter. Methods on *Handlers implement
// individual endpoints; auth middleware sits in front of the protected
// subset.
type Handlers struct {
	Store         *state.Store
	Broadcaster   *state.Broadcaster
	Orchestrator  *jobs.Orchestrator
	Pipelines     *pipelines.Registry
	Classifier    identify.Classifier
	TMDB          identify.TMDBClient
	MusicBrainz   identify.MusicBrainzClient
	ActiveSampler *ActiveJobsSampler
	Token         string
	Apprise       Apprise // defined in notifications.go; nil-safe in handlers
	Settings      *settings.Settings

	// NVENCAvailable is set at boot by tools.ProbeNVENC. It controls
	// the "GPU transcoding" Settings row and is threaded into the
	// DVD-Video / BDMV pipeline Deps (in main.go).
	NVENCAvailable bool

	// startMu serializes the "no active job?" check against job
	// submission in StartDisc. Two POST /discs/{id}/start requests can
	// arrive within milliseconds for the same disc (the dashboard
	// mounts both the mobile and desktop component trees, each with
	// its own auto-confirm timer); without this lock both pass the
	// guard and enqueue a duplicate job. The daemon is single-process,
	// so an in-process mutex fully closes the race.
	startMu sync.Mutex
}
