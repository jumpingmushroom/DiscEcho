// Package state holds DiscEcho's persistent and in-memory state model:
// the resource types (Drive, Disc, Job, Profile, etc.) plus the SQLite
// store and event broadcaster that back them.
package state

import "time"

// DiscType is the canonical disc-type discriminator. The string values
// land verbatim in SQLite and the wire JSON.
type DiscType string

const (
	DiscTypeAudioCD DiscType = "AUDIO_CD"
	DiscTypeDVD     DiscType = "DVD"
	DiscTypeBDMV    DiscType = "BDMV"
	DiscTypeUHD     DiscType = "UHD"
	DiscTypePSX     DiscType = "PSX"
	DiscTypePS2     DiscType = "PS2"
	DiscTypeXBOX    DiscType = "XBOX"
	DiscTypeSAT     DiscType = "SAT"
	DiscTypeDC      DiscType = "DC"
	DiscTypeVCD     DiscType = "VCD"
	DiscTypeData    DiscType = "DATA"
)

// DriveState transitions:
//
//	idle → identifying → ripping → ejecting → idle
//	any → error (manual recovery via /api/drives/:id rescan)
type DriveState string

const (
	DriveStateIdle        DriveState = "idle"
	DriveStateIdentifying DriveState = "identifying"
	DriveStateRipping     DriveState = "ripping"
	DriveStateEjecting    DriveState = "ejecting"
	DriveStateError       DriveState = "error"
)

// JobState transitions:
//
//	queued → identifying → running → done | failed | cancelled
//	running → paused (M1: never; pause is 501)
//	any except {done,failed,cancelled} → interrupted (on daemon crash)
type JobState string

const (
	JobStateQueued      JobState = "queued"
	JobStateIdentifying JobState = "identifying"
	JobStateRunning     JobState = "running"
	JobStatePaused      JobState = "paused"
	JobStateDone        JobState = "done"
	JobStateFailed      JobState = "failed"
	JobStateCancelled   JobState = "cancelled"
	JobStateInterrupted JobState = "interrupted"
)

// StepID names a position in the canonical 8-step pipeline. Order is
// fixed; profiles set Skip=true on irrelevant steps.
type StepID string

const (
	StepDetect    StepID = "detect"
	StepIdentify  StepID = "identify"
	StepRip       StepID = "rip"
	StepTranscode StepID = "transcode"
	StepCompress  StepID = "compress"
	StepMove      StepID = "move"
	StepNotify    StepID = "notify"
	StepEject     StepID = "eject"
)

// CanonicalSteps returns the eight-step order. Used by job_steps row
// insertion and UI rendering.
func CanonicalSteps() []StepID {
	return []StepID{
		StepDetect, StepIdentify, StepRip, StepTranscode,
		StepCompress, StepMove, StepNotify, StepEject,
	}
}

// JobStepState transitions:
//
//	pending → running → done | failed | skipped
type JobStepState string

const (
	JobStepStatePending JobStepState = "pending"
	JobStepStateRunning JobStepState = "running"
	JobStepStateDone    JobStepState = "done"
	JobStepStateSkipped JobStepState = "skipped"
	JobStepStateFailed  JobStepState = "failed"
)

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// Drive is a row of the `drives` table.
type Drive struct {
	ID            string     `json:"id"`
	Model         string     `json:"model"`
	Bus           string     `json:"bus"`
	DevPath       string     `json:"dev_path"`
	State         DriveState `json:"state"`
	LastSeenAt    time.Time  `json:"last_seen_at"`
	Notes         string     `json:"notes,omitempty"`
	CurrentDiscID string     `json:"current_disc_id,omitempty"` // computed, not stored
}

// Candidate is a single MB (or other source) match for a disc.
type Candidate struct {
	Source         string `json:"source"`
	Title          string `json:"title"`
	Artist         string `json:"artist,omitempty"`
	Year           int    `json:"year,omitempty"`
	Region         string `json:"region,omitempty"` // game-disc region (USA / Europe / Japan / ...)
	Confidence     int    `json:"confidence"`
	MBID           string `json:"mbid,omitempty"`
	TMDBID         int    `json:"tmdb_id,omitempty"`
	MediaType      string `json:"media_type,omitempty"`      // 'movie' | 'tv' | '' (audio CD)
	RuntimeSeconds int    `json:"runtime_seconds,omitempty"` // populated by per-pick TMDB /movie/{id} fetch when the user picks
}

// Disc is a row of the `discs` table.
type Disc struct {
	ID               string      `json:"id"`
	DriveID          string      `json:"drive_id,omitempty"`
	Type             DiscType    `json:"type"`
	Title            string      `json:"title,omitempty"`
	Year             int         `json:"year,omitempty"`
	RuntimeSeconds   int         `json:"runtime_seconds,omitempty"`
	SizeBytesRaw     int64       `json:"size_bytes_raw,omitempty"`
	TOCHash          string      `json:"toc_hash,omitempty"`
	MetadataProvider string      `json:"metadata_provider,omitempty"`
	MetadataID       string      `json:"metadata_id,omitempty"`
	MetadataJSON     string      `json:"metadata_json,omitempty"` // extended per-disc-type display data
	Candidates       []Candidate `json:"candidates"`
	CreatedAt        time.Time   `json:"created_at"`
}

// JobStep is a row of the `job_steps` table.
type JobStep struct {
	Step         StepID         `json:"step"`
	State        JobStepState   `json:"state"`
	AttemptCount int            `json:"attempt_count"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	Notes        map[string]any `json:"notes,omitempty"`
}

// Job is a row of the `jobs` table.
type Job struct {
	ID             string     `json:"id"`
	DiscID         string     `json:"disc_id"`
	DriveID        string     `json:"drive_id,omitempty"`
	ProfileID      string     `json:"profile_id"`
	State          JobState   `json:"state"`
	ActiveStep     StepID     `json:"active_step,omitempty"`
	Progress       float64    `json:"progress"`
	Speed          string     `json:"speed,omitempty"`
	ETASeconds     int        `json:"eta_seconds,omitempty"`
	ElapsedSeconds int        `json:"elapsed_seconds,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	Steps          []JobStep  `json:"steps,omitempty"`
}

// Profile is a row of the `profiles` table. Container/VideoCodec/
// QualityPreset/HDRPipeline/DrivePolicy/AutoEject are the typed fields
// that drive the mockup-shaped editor; Format and Preset stay for one
// release as a fallback so older API clients continue to work.
type Profile struct {
	ID                 string         `json:"id"`
	DiscType           DiscType       `json:"disc_type"`
	Name               string         `json:"name"`
	Engine             string         `json:"engine"`
	Format             string         `json:"format,omitempty"`
	Preset             string         `json:"preset,omitempty"`
	Container          string         `json:"container"`
	VideoCodec         string         `json:"video_codec"`
	QualityPreset      string         `json:"quality_preset"`
	HDRPipeline        string         `json:"hdr_pipeline"`
	DrivePolicy        string         `json:"drive_policy"`
	AutoEject          bool           `json:"auto_eject"`
	Options            map[string]any `json:"options"`
	OutputPathTemplate string         `json:"output_path_template"`
	Enabled            bool           `json:"enabled"`
	StepCount          int            `json:"step_count"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
}

// Notification is a row of the `notifications` table.
type Notification struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Tags      string    `json:"tags"`     // comma-separated
	Triggers  string    `json:"triggers"` // comma-separated subset of done|failed|warn
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LogLine is a row of the `log_lines` table.
type LogLine struct {
	JobID   string    `json:"job_id"`
	T       time.Time `json:"t"`
	Level   LogLevel  `json:"level"`
	Message string    `json:"message"`
}
