package api

import (
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
}
