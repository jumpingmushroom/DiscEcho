package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by Get* methods when no row matches.
var ErrNotFound = errors.New("state: not found")

// Store is the typed CRUD layer over the DB. All tables route through here.
type Store struct {
	db *DB
}

func NewStore(db *DB) *Store { return &Store{db: db} }

// DB exposes the underlying *DB for tests and the broadcaster bootstrap.
// Production callers should use the typed methods on Store.
func (s *Store) DB() *DB { return s.db }

// NewID returns a fresh UUIDv4 string (lower-case, 36 chars).
func NewID() string { return uuid.NewString() }

// timestamp formats t in the canonical RFC3339Nano UTC string.
func timestamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// parseTime parses "" as a zero time, otherwise RFC3339Nano.
func parseTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, s)
}

// parseTimePtr returns nil for "", otherwise a pointer to the parsed
// time. Used for nullable timestamp columns where the wire JSON expects
// `null` rather than the zero time.
func parseTimePtr(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// timestampPtr returns "" for nil, otherwise the formatted timestamp.
func timestampPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return timestamp(*t)
}

// ---- shared scanners ------------------------------------------------------

type rowScanner interface {
	Scan(dest ...any) error
}

// ---- JSON helpers ---------------------------------------------------------

func marshalCandidates(cs []Candidate) (string, error) {
	if cs == nil {
		cs = []Candidate{}
	}
	b, err := json.Marshal(cs)
	return string(b), err
}

func unmarshalCandidates(s string) ([]Candidate, error) {
	if s == "" {
		return nil, nil
	}
	var cs []Candidate
	if err := json.Unmarshal([]byte(s), &cs); err != nil {
		return nil, err
	}
	return cs, nil
}

func marshalOptions(opts map[string]any) (string, error) {
	if opts == nil {
		opts = map[string]any{}
	}
	b, err := json.Marshal(opts)
	return string(b), err
}

func unmarshalOptions(s string) (map[string]any, error) {
	if s == "" {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ---- DRIVES ----------------------------------------------------------------

// GetDrive fetches a drive by ID.
func (s *Store) GetDrive(ctx context.Context, id string) (*Drive, error) {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, model, bus, dev_path, state, last_seen_at, notes
		FROM drives WHERE id = ?`, id)
	return scanDrive(row)
}

// ListDrives returns all drives ordered by dev_path.
func (s *Store) ListDrives(ctx context.Context) ([]Drive, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT id, model, bus, dev_path, state, last_seen_at, notes
		FROM drives ORDER BY dev_path`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Drive
	for rows.Next() {
		d, err := scanDrive(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// UpsertDrive inserts a new drive, or updates the existing one keyed by
// dev_path. d.ID is filled in on insert.
func (s *Store) UpsertDrive(ctx context.Context, d *Drive) error {
	if d.ID == "" {
		d.ID = NewID()
	}
	_, err := s.db.Conn().ExecContext(ctx, `
		INSERT INTO drives (id, model, bus, dev_path, state, last_seen_at, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dev_path) DO UPDATE SET
		  model = excluded.model,
		  bus = excluded.bus,
		  state = excluded.state,
		  last_seen_at = excluded.last_seen_at,
		  notes = excluded.notes`,
		d.ID, d.Model, d.Bus, d.DevPath, string(d.State),
		timestamp(d.LastSeenAt), d.Notes)
	return err
}

// ResetIdentifyingDrives clears any drive left in the `identifying`
// state, transitioning it back to `idle`. Returns the number of rows
// updated.
//
// A daemon crash, container restart, or classify-ctx timeout whose
// cleanup write was itself made with the timed-out ctx (and silently
// failed) can leave a drive stuck in `identifying`. ClaimDriveForIdentify
// only transitions from `idle` or `error`, so a stuck row means every
// subsequent uevent hits "already identifying" and the daemon is deaf
// until the row is corrected. Call this at startup so the next uevent
// can claim the drive cleanly.
func (s *Store) ResetIdentifyingDrives(ctx context.Context) (int, error) {
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE drives SET state = ?, last_seen_at = ? WHERE state = ?`,
		string(DriveStateIdle), timestamp(time.Now()), string(DriveStateIdentifying))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// UpdateDriveState sets the drive's state and refreshes last_seen_at.
func (s *Store) UpdateDriveState(ctx context.Context, id string, state DriveState) error {
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE drives SET state = ?, last_seen_at = ? WHERE id = ?`,
		string(state), timestamp(time.Now()), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ClaimDriveForIdentify atomically transitions the drive into the
// `identifying` state only if it's currently `idle` or `error`. Returns
// (true, nil) when the claim succeeds — the caller owns the identify
// slot. Returns (false, nil) when another caller is already identifying
// or the drive is busy with a later state (ripping, transcoding, …);
// the caller must drop the uevent.
//
// This closes the race the v0.1.4 active-job guard left open: between
// the first `disc inserted` uevent (which kicks off identify but
// doesn't have a job yet) and the second uevent a few seconds later,
// `HasActiveJobOnDrive` returns false for both, so both proceed to
// identify and both create separate Disc rows. Hollywood DVDs emit
// multiple media-change uevents per insertion as the drive settles,
// which made the duplicate Disc rows reliably reproducible.
func (s *Store) ClaimDriveForIdentify(ctx context.Context, id string) (bool, error) {
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE drives SET state = ?, last_seen_at = ?
		 WHERE id = ? AND state IN ('idle','error')`,
		string(DriveStateIdentifying), timestamp(time.Now()), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func scanDrive(r rowScanner) (*Drive, error) {
	var d Drive
	var state, lastSeenStr string
	if err := r.Scan(&d.ID, &d.Model, &d.Bus, &d.DevPath, &state, &lastSeenStr, &d.Notes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d.State = DriveState(state)
	t, err := parseTime(lastSeenStr)
	if err != nil {
		return nil, fmt.Errorf("parse last_seen_at: %w", err)
	}
	d.LastSeenAt = t
	return &d, nil
}

// ---- DISCS ----------------------------------------------------------------

// CreateDisc inserts a new disc row. d.ID and d.CreatedAt are filled in
// if zero. Discs are immutable after creation in M1.1; updates land in
// future milestones via separate dedicated methods.
func (s *Store) CreateDisc(ctx context.Context, d *Disc) error {
	if d.ID == "" {
		d.ID = NewID()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now()
	}
	candJSON, err := marshalCandidates(d.Candidates)
	if err != nil {
		return fmt.Errorf("marshal candidates: %w", err)
	}
	metaBlob := d.MetadataJSON
	if metaBlob == "" {
		metaBlob = "{}"
	}
	_, err = s.db.Conn().ExecContext(ctx, `
		INSERT INTO discs (id, drive_id, type, title, year, runtime_seconds,
		                   size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		                   candidates_json, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, nullString(d.DriveID), string(d.Type), d.Title, d.Year, d.RuntimeSeconds,
		d.SizeBytesRaw, d.TOCHash, d.MetadataProvider, d.MetadataID,
		candJSON, metaBlob, timestamp(d.CreatedAt))
	return err
}

// GetDiscByDriveTOC returns the most-recently-created disc on driveID
// with the given non-empty toc_hash, or ErrNotFound when none exists.
// Callers (notably discflow) use this on detect to reuse an existing
// disc row instead of inserting a new one on every retry — see
// migration 009 for the backfill that collapsed pre-existing dupes.
// An empty driveID or tocHash short-circuits to ErrNotFound because
// neither identifies a single physical disc.
func (s *Store) GetDiscByDriveTOC(ctx context.Context, driveID, tocHash string) (*Disc, error) {
	if driveID == "" || tocHash == "" {
		return nil, ErrNotFound
	}
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, COALESCE(drive_id, ''), type, title, year, runtime_seconds,
		       size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		       candidates_json, metadata_json, created_at
		FROM discs WHERE drive_id = ? AND toc_hash = ?
		ORDER BY created_at DESC LIMIT 1`, driveID, tocHash)
	return scanDisc(row)
}

// GetDisc fetches a disc by ID, including its candidates.
func (s *Store) GetDisc(ctx context.Context, id string) (*Disc, error) {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, COALESCE(drive_id, ''), type, title, year, runtime_seconds,
		       size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		       candidates_json, metadata_json, created_at
		FROM discs WHERE id = ?`, id)
	return scanDisc(row)
}

// ListRecentDiscs returns the N most-recently-created discs across all
// drives, newest first. Used by /api/state and the SSE bootstrap so the
// UI can resolve disc titles for the active and recent jobs without an
// extra round-trip per job.
func (s *Store) ListRecentDiscs(ctx context.Context, limit int) ([]Disc, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT id, COALESCE(drive_id, ''), type, title, year, runtime_seconds,
		       size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		       candidates_json, metadata_json, created_at
		FROM discs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Disc
	for rows.Next() {
		d, err := scanDisc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// ListDiscsForDrive returns discs that were inserted in the given drive,
// most recent first.
func (s *Store) ListDiscsForDrive(ctx context.Context, driveID string) ([]Disc, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT id, COALESCE(drive_id, ''), type, title, year, runtime_seconds,
		       size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		       candidates_json, metadata_json, created_at
		FROM discs WHERE drive_id = ? ORDER BY created_at DESC`, driveID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Disc
	for rows.Next() {
		d, err := scanDisc(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// UpdateDiscMetadata persists the title/year/provider/metadata_id
// fields for a disc. Used when the user picks a candidate via
// /discs/{id}/start so the chosen identity reaches the orchestrator
// (which re-reads the row before dispatching the job).
func (s *Store) UpdateDiscMetadata(ctx context.Context, id, title string, year int, provider, metadataID string) error {
	res, err := s.db.Conn().ExecContext(ctx, `
		UPDATE discs SET title = ?, year = ?, metadata_provider = ?, metadata_id = ?
		WHERE id = ?`, title, year, provider, metadataID, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateDiscMetadataBlob writes the per-disc-type extended metadata
// JSON onto an existing disc row. Used at /api/discs/{id}/start after
// the user picks a candidate so the pane has rich data from the first
// paint without round-tripping back to TMDB / MusicBrainz at view
// time. blob is the raw JSON string (UTF-8). An empty string is
// stored as "{}" so the column's NOT NULL constraint is satisfied
// and the webui can safely parse the result.
func (s *Store) UpdateDiscMetadataBlob(ctx context.Context, id string, blob string) error {
	if blob == "" {
		blob = "{}"
	}
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE discs SET metadata_json = ? WHERE id = ?`, blob, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateDiscRuntime persists the runtime (in seconds) for a disc.
// Called from /discs/{id}/start after the daemon fetches TMDB's
// `/movie/{id}` for the chosen candidate. The DVD pipeline reads
// this column as a sanity check against the scanned title duration.
func (s *Store) UpdateDiscRuntime(ctx context.Context, id string, runtimeSec int) error {
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE discs SET runtime_seconds = ? WHERE id = ?`, runtimeSec, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DiscHasAnyJob reports whether any job (in any state) references the
// disc. Used by the Skip / delete affordance to refuse removing discs
// that already have job history — a delete would leave those job rows
// pointing at a non-existent disc_id.
func (s *Store) DiscHasAnyJob(ctx context.Context, discID string) (bool, error) {
	if discID == "" {
		return false, nil
	}
	var n int
	err := s.db.Conn().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM jobs WHERE disc_id = ?`, discID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DiscHasActiveJob reports whether the disc currently has a job in any
// non-terminal state (queued / identifying / running / paused). Used by
// POST /api/discs/{id}/start to refuse a duplicate start when a previous
// click already created a job — the orchestrator serialises per-drive
// but the API itself has no idempotency, so a fast double-click (or
// auto-confirm + manual click racing) otherwise enqueues two jobs for
// the same disc.
func (s *Store) DiscHasActiveJob(ctx context.Context, discID string) (bool, error) {
	if discID == "" {
		return false, nil
	}
	var n int
	err := s.db.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE disc_id = ?
		  AND state IN ('queued','identifying','running','paused')`,
		discID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// DeleteDisc removes a disc row by id. Returns ErrNotFound when no row
// matches. Does NOT cascade-delete jobs — callers must check via
// DiscHasAnyJob first.
func (s *Store) DeleteDisc(ctx context.Context, id string) error {
	res, err := s.db.Conn().ExecContext(ctx,
		`DELETE FROM discs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateDiscCandidates replaces the candidates JSON for an existing disc.
// Used by the identify endpoint when the user supplies a manual TMDB
// query and we want to persist the new candidate list.
func (s *Store) UpdateDiscCandidates(ctx context.Context, id string, cands []Candidate) error {
	body, err := marshalCandidates(cands)
	if err != nil {
		return fmt.Errorf("marshal candidates: %w", err)
	}
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE discs SET candidates_json = ? WHERE id = ?`, body, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// nullString returns a sql.NullString that's NULL on the empty string,
// used for nullable FK columns. SQLite ON DELETE SET NULL needs real
// NULLs to work; an empty-string value would never match.
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func scanDisc(r rowScanner) (*Disc, error) {
	var d Disc
	var dtype, candJSON, metaJSON, createdStr string
	if err := r.Scan(
		&d.ID, &d.DriveID, &dtype, &d.Title, &d.Year, &d.RuntimeSeconds,
		&d.SizeBytesRaw, &d.TOCHash, &d.MetadataProvider, &d.MetadataID,
		&candJSON, &metaJSON, &createdStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	d.Type = DiscType(dtype)
	cs, err := unmarshalCandidates(candJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal candidates: %w", err)
	}
	d.Candidates = cs
	d.MetadataJSON = metaJSON
	t, err := parseTime(createdStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	d.CreatedAt = t
	return &d, nil
}

// ---- PROFILES -------------------------------------------------------------

// CreateProfile inserts a new profile. p.ID, p.CreatedAt, p.UpdatedAt
// are filled in if zero.
func (s *Store) CreateProfile(ctx context.Context, p *Profile) error {
	if p.ID == "" {
		p.ID = NewID()
	}
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	optsJSON, err := marshalOptions(p.Options)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}
	if p.DrivePolicy == "" {
		p.DrivePolicy = "any"
	}
	_, err = s.db.Conn().ExecContext(ctx, `
		INSERT INTO profiles (id, disc_type, name, engine, format, preset,
		                      container, video_codec, quality_preset,
		                      hdr_pipeline, drive_policy,
		                      options_json, output_path_template, enabled,
		                      step_count, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, string(p.DiscType), p.Name, p.Engine, p.Format, p.Preset,
		p.Container, p.VideoCodec, p.QualityPreset,
		p.HDRPipeline, p.DrivePolicy,
		optsJSON, p.OutputPathTemplate, boolToInt(p.Enabled),
		p.StepCount, timestamp(p.CreatedAt), timestamp(p.UpdatedAt))
	return err
}

// GetProfile fetches a profile by ID.
func (s *Store) GetProfile(ctx context.Context, id string) (*Profile, error) {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, disc_type, name, engine, format, preset,
		       container, video_codec, quality_preset,
		       hdr_pipeline, drive_policy,
		       options_json, output_path_template, enabled,
		       step_count, created_at, updated_at
		FROM profiles WHERE id = ?`, id)
	return scanProfile(row)
}

// ListProfiles returns all profiles.
func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	return s.queryProfiles(ctx, `
		SELECT id, disc_type, name, engine, format, preset,
		       container, video_codec, quality_preset,
		       hdr_pipeline, drive_policy,
		       options_json, output_path_template, enabled,
		       step_count, created_at, updated_at
		FROM profiles ORDER BY name`)
}

// ListProfilesByDiscType filters profiles to those matching the given
// disc type. Used by the orchestrator when picking a default for a
// freshly identified disc.
func (s *Store) ListProfilesByDiscType(ctx context.Context, dt DiscType) ([]Profile, error) {
	return s.queryProfiles(ctx, `
		SELECT id, disc_type, name, engine, format, preset,
		       container, video_codec, quality_preset,
		       hdr_pipeline, drive_policy,
		       options_json, output_path_template, enabled,
		       step_count, created_at, updated_at
		FROM profiles WHERE disc_type = ? ORDER BY name`, string(dt))
}

// UpdateProfile rewrites every mutable column. The ID and CreatedAt
// stay; UpdatedAt is refreshed.
func (s *Store) UpdateProfile(ctx context.Context, p *Profile) error {
	p.UpdatedAt = time.Now()
	optsJSON, err := marshalOptions(p.Options)
	if err != nil {
		return fmt.Errorf("marshal options: %w", err)
	}
	if p.DrivePolicy == "" {
		p.DrivePolicy = "any"
	}
	res, err := s.db.Conn().ExecContext(ctx, `
		UPDATE profiles SET disc_type = ?, name = ?, engine = ?, format = ?,
		                    preset = ?, container = ?, video_codec = ?,
		                    quality_preset = ?, hdr_pipeline = ?,
		                    drive_policy = ?,
		                    options_json = ?, output_path_template = ?,
		                    enabled = ?, step_count = ?, updated_at = ?
		WHERE id = ?`,
		string(p.DiscType), p.Name, p.Engine, p.Format, p.Preset,
		p.Container, p.VideoCodec, p.QualityPreset, p.HDRPipeline,
		p.DrivePolicy,
		optsJSON, p.OutputPathTemplate, boolToInt(p.Enabled),
		p.StepCount, timestamp(p.UpdatedAt), p.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteProfile removes a profile. ON DELETE RESTRICT on jobs.profile_id
// will cause this to error if any job references the profile.
func (s *Store) DeleteProfile(ctx context.Context, id string) error {
	res, err := s.db.Conn().ExecContext(ctx, `DELETE FROM profiles WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) queryProfiles(ctx context.Context, q string, args ...any) ([]Profile, error) {
	rows, err := s.db.Conn().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Profile
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func scanProfile(r rowScanner) (*Profile, error) {
	var p Profile
	var dtype, optsJSON, createdStr, updatedStr string
	var enabled int
	if err := r.Scan(
		&p.ID, &dtype, &p.Name, &p.Engine, &p.Format, &p.Preset,
		&p.Container, &p.VideoCodec, &p.QualityPreset,
		&p.HDRPipeline, &p.DrivePolicy,
		&optsJSON, &p.OutputPathTemplate, &enabled,
		&p.StepCount, &createdStr, &updatedStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.DiscType = DiscType(dtype)
	p.Enabled = enabled != 0
	opts, err := unmarshalOptions(optsJSON)
	if err != nil {
		return nil, fmt.Errorf("unmarshal options: %w", err)
	}
	p.Options = opts
	if p.CreatedAt, err = parseTime(createdStr); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if p.UpdatedAt, err = parseTime(updatedStr); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &p, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---- JOBS -----------------------------------------------------------------

// CreateJob inserts a new job AND eagerly materializes its eight
// job_steps rows. Steps marked Skipped in j.Steps land as
// JobStepStateSkipped; the rest start at JobStepStatePending. The whole
// insert is one tx so callers never observe a half-created job. j.ID
// and j.CreatedAt fill in if zero.
func (s *Store) CreateJob(ctx context.Context, j *Job) error {
	if j.ID == "" {
		j.ID = NewID()
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now()
	}
	if j.State == "" {
		j.State = JobStateQueued
	}

	skipSet := map[StepID]bool{}
	for _, st := range j.Steps {
		if st.State == JobStepStateSkipped {
			skipSet[st.Step] = true
		}
	}

	tx, err := s.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO jobs (id, disc_id, drive_id, profile_id, state, active_step,
		                  progress, speed, eta_seconds, elapsed_seconds, output_bytes,
		                  started_at, finished_at, error_message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		j.ID, j.DiscID, nullString(j.DriveID), j.ProfileID,
		string(j.State), string(j.ActiveStep),
		j.Progress, j.Speed, j.ETASeconds, j.ElapsedSeconds, j.OutputBytes,
		timestampPtr(j.StartedAt), timestampPtr(j.FinishedAt),
		j.ErrorMessage, timestamp(j.CreatedAt),
	); err != nil {
		return err
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO job_steps (job_id, step, state, attempt_count,
		                       started_at, finished_at, notes_json)
		VALUES (?, ?, ?, 0, '', '', '{}')`)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	steps := make([]JobStep, 0, len(CanonicalSteps()))
	for _, sid := range CanonicalSteps() {
		stState := JobStepStatePending
		if skipSet[sid] {
			stState = JobStepStateSkipped
		}
		if _, err := stmt.ExecContext(ctx, j.ID, string(sid), string(stState)); err != nil {
			return err
		}
		steps = append(steps, JobStep{Step: sid, State: stState})
	}
	j.Steps = steps

	return tx.Commit()
}

// GetJob fetches a job and its steps. Returns ErrNotFound if missing.
func (s *Store) GetJob(ctx context.Context, id string) (*Job, error) {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, disc_id, COALESCE(drive_id, ''), profile_id, state, active_step,
		       progress, speed, eta_seconds, elapsed_seconds, output_bytes,
		       started_at, finished_at, error_message, created_at
		FROM jobs WHERE id = ?`, id)
	j, err := scanJob(row)
	if err != nil {
		return nil, err
	}
	steps, err := s.ListJobSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	j.Steps = steps
	return j, nil
}

// JobFilter narrows ListJobs.
type JobFilter struct {
	State   JobState // empty = no filter
	DriveID string   // empty = no filter
	Limit   int      // 0 → 50, capped at 200
	Offset  int      // 0 = beginning
}

// ListJobs returns jobs matching f, ordered by created_at DESC. Steps
// are NOT loaded; callers wanting steps use GetJob per-id.
func (s *Store) ListJobs(ctx context.Context, f JobFilter) ([]Job, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	q := `SELECT id, disc_id, COALESCE(drive_id, ''), profile_id, state, active_step,
	             progress, speed, eta_seconds, elapsed_seconds, output_bytes,
	             started_at, finished_at, error_message, created_at
	      FROM jobs`
	var args []any
	var conds []string
	if f.State != "" {
		conds = append(conds, "state = ?")
		args = append(args, string(f.State))
	}
	if f.DriveID != "" {
		conds = append(conds, "drive_id = ?")
		args = append(args, f.DriveID)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	rows, err := s.db.Conn().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// ListActiveAndRecentJobs returns currently-active jobs plus the N
// most-recent finished jobs, used for /api/state's first-paint payload.
// Active = state NOT IN ('done','failed','cancelled').
func (s *Store) ListActiveAndRecentJobs(ctx context.Context, recentLimit int) ([]Job, error) {
	if recentLimit < 0 {
		recentLimit = 0
	}

	activeRows, err := s.db.Conn().QueryContext(ctx, `
		SELECT id, disc_id, COALESCE(drive_id, ''), profile_id, state, active_step,
		       progress, speed, eta_seconds, elapsed_seconds, output_bytes,
		       started_at, finished_at, error_message, created_at
		FROM jobs
		WHERE state NOT IN ('done','failed','cancelled')
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = activeRows.Close() }()
	var out []Job
	for activeRows.Next() {
		j, err := scanJob(activeRows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	if err := activeRows.Err(); err != nil {
		return nil, err
	}

	if recentLimit > 0 {
		recentRows, err := s.db.Conn().QueryContext(ctx, `
			SELECT id, disc_id, COALESCE(drive_id, ''), profile_id, state, active_step,
			       progress, speed, eta_seconds, elapsed_seconds, output_bytes,
			       started_at, finished_at, error_message, created_at
			FROM jobs
			WHERE state IN ('done','failed','cancelled')
			ORDER BY COALESCE(NULLIF(finished_at,''), created_at) DESC
			LIMIT ?`, recentLimit)
		if err != nil {
			return nil, err
		}
		defer func() { _ = recentRows.Close() }()
		for recentRows.Next() {
			j, err := scanJob(recentRows)
			if err != nil {
				return nil, err
			}
			out = append(out, *j)
		}
		if err := recentRows.Err(); err != nil {
			return nil, err
		}
	}
	// Hydrate steps for every job so the desktop pipeline stepper and
	// the mobile job rows can render the correct dot colors without an
	// extra round-trip. Without this, /api/state and the SSE snapshot
	// always return step_count=0 and the stepper renders empty.
	for i := range out {
		steps, err := s.ListJobSteps(ctx, out[i].ID)
		if err != nil {
			return nil, fmt.Errorf("hydrate steps for %s: %w", out[i].ID, err)
		}
		out[i].Steps = steps
	}
	return out, nil
}

// HasActiveJobOnDrive reports whether the given drive currently has a
// job in queued / running / identifying state. Used by the udev event
// handler to drop mid-rip media-change events that would otherwise
// collide with the active job's exclusive hold on /dev/sr0.
func (s *Store) HasActiveJobOnDrive(ctx context.Context, driveID string) (bool, error) {
	if driveID == "" {
		return false, nil
	}
	var n int
	err := s.db.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE drive_id = ? AND state IN ('queued','running','identifying')`,
		driveID).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// SetActiveStep writes jobs.active_step without touching progress /
// speed / eta. Called by the persistent sink at OnStepStart so the
// dashboard's pipeline-stepper highlight tracks the running step even
// before any progress event fires.
func (s *Store) SetActiveStep(ctx context.Context, jobID string, step StepID) error {
	_, err := s.db.Conn().ExecContext(ctx,
		`UPDATE jobs SET active_step = ? WHERE id = ?`,
		string(step), jobID)
	return err
}

// HasRecentJobOnDrive reports whether any job on the drive finished
// within the last `cooldown`. Defence-in-depth against the race where
// a spurious mid-rip media-change uevent fires at the *exact* instant
// the current job transitions to done — HasActiveJobOnDrive returns
// false, the guard lets the re-classify through, and the kernel disc
// disturbance trashes whatever the orchestrator was about to do next.
func (s *Store) HasRecentJobOnDrive(ctx context.Context, driveID string, cooldown time.Duration) (bool, error) {
	if driveID == "" || cooldown <= 0 {
		return false, nil
	}
	cutoff := timestamp(time.Now().Add(-cooldown))
	var n int
	err := s.db.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE drive_id = ?
		  AND state IN ('done','failed','cancelled','interrupted')
		  AND finished_at >= ?`,
		driveID, cutoff).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// UpdateJobState transitions a job, optionally bumping started_at /
// finished_at on the relevant transitions. error_message is set on
// JobStateFailed transitions; pass "" otherwise.
func (s *Store) UpdateJobState(ctx context.Context, id string, st JobState, errMsg string) error {
	now := timestamp(time.Now())
	q := `UPDATE jobs SET state = ?`
	args := []any{string(st)}
	switch st {
	case JobStateRunning:
		q += `, started_at = COALESCE(NULLIF(started_at, ''), ?)`
		args = append(args, now)
	case JobStateDone, JobStateFailed, JobStateCancelled, JobStateInterrupted:
		q += `, finished_at = ?`
		args = append(args, now)
	}
	if st == JobStateFailed {
		q += `, error_message = ?`
		args = append(args, errMsg)
	}
	q += ` WHERE id = ?`
	args = append(args, id)
	res, err := s.db.Conn().ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateJobProgress writes the volatile progress fields. Cheap; called
// by the orchestrator at most once per second per job.
func (s *Store) UpdateJobProgress(ctx context.Context, id string, activeStep StepID,
	pct float64, speed string, etaSeconds, elapsedSeconds int) error {
	res, err := s.db.Conn().ExecContext(ctx, `
		UPDATE jobs SET active_step = ?, progress = ?, speed = ?,
		                eta_seconds = ?, elapsed_seconds = ?
		WHERE id = ?`,
		string(activeStep), pct, speed, etaSeconds, elapsedSeconds, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// RecordOutputBytes writes the move step's final output size onto a
// job row. Called from each pipeline after the move step succeeds so
// the LIBRARY SIZE widget can sum across all done jobs.
func (s *Store) RecordOutputBytes(ctx context.Context, jobID string, bytes int64) error {
	res, err := s.db.Conn().ExecContext(ctx,
		`UPDATE jobs SET output_bytes = ? WHERE id = ?`, bytes, jobID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MarkInterruptedJobs flips every job in {queued, identifying, running}
// to interrupted. Used at daemon startup so crashed-mid-rip jobs are
// visible in the UI for resolution. Returns the count flipped.
func (s *Store) MarkInterruptedJobs(ctx context.Context) (int, error) {
	now := timestamp(time.Now())
	res, err := s.db.Conn().ExecContext(ctx, `
		UPDATE jobs
		SET state = 'interrupted', finished_at = ?
		WHERE state IN ('queued','identifying','running')`, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanJob(r rowScanner) (*Job, error) {
	var j Job
	var st, activeStep, startedStr, finishedStr, createdStr string
	if err := r.Scan(
		&j.ID, &j.DiscID, &j.DriveID, &j.ProfileID, &st, &activeStep,
		&j.Progress, &j.Speed, &j.ETASeconds, &j.ElapsedSeconds, &j.OutputBytes,
		&startedStr, &finishedStr, &j.ErrorMessage, &createdStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	j.State = JobState(st)
	j.ActiveStep = StepID(activeStep)
	var err error
	if j.StartedAt, err = parseTimePtr(startedStr); err != nil {
		return nil, fmt.Errorf("parse started_at: %w", err)
	}
	if j.FinishedAt, err = parseTimePtr(finishedStr); err != nil {
		return nil, fmt.Errorf("parse finished_at: %w", err)
	}
	if j.CreatedAt, err = parseTime(createdStr); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return &j, nil
}

// ---- JOB STEPS (read-side; mutators in next commit) -----------------------

// ListJobSteps returns every job_step row for a job, in insertion order
// (= canonical step sequence, since CreateJob inserts them that way).
func (s *Store) ListJobSteps(ctx context.Context, jobID string) ([]JobStep, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT step, state, attempt_count, started_at, finished_at, notes_json
		FROM job_steps WHERE job_id = ?
		ORDER BY id`, jobID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []JobStep
	for rows.Next() {
		st, err := scanJobStep(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

func scanJobStep(r rowScanner) (*JobStep, error) {
	var st JobStep
	var step, stateStr, startedStr, finishedStr, notesJSON string
	if err := r.Scan(&step, &stateStr, &st.AttemptCount, &startedStr, &finishedStr, &notesJSON); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	st.Step = StepID(step)
	st.State = JobStepState(stateStr)
	var err error
	if st.StartedAt, err = parseTimePtr(startedStr); err != nil {
		return nil, fmt.Errorf("parse started_at: %w", err)
	}
	if st.FinishedAt, err = parseTimePtr(finishedStr); err != nil {
		return nil, fmt.Errorf("parse finished_at: %w", err)
	}
	if notesJSON != "" && notesJSON != "{}" {
		var notes map[string]any
		if err := json.Unmarshal([]byte(notesJSON), &notes); err != nil {
			return nil, fmt.Errorf("unmarshal notes_json: %w", err)
		}
		st.Notes = notes
	}
	return &st, nil
}

// UpdateJobStepState transitions a step's state. Sets started_at on
// transitions to running; sets finished_at on transitions to
// {done, skipped, failed}. Bumps attempt_count on every transition to
// running (so "running again after failure" reads as a retry).
func (s *Store) UpdateJobStepState(ctx context.Context, jobID string, step StepID, st JobStepState) error {
	now := timestamp(time.Now())
	q := `UPDATE job_steps SET state = ?`
	args := []any{string(st)}
	switch st {
	case JobStepStateRunning:
		q += `, started_at = COALESCE(NULLIF(started_at, ''), ?), attempt_count = attempt_count + 1`
		args = append(args, now)
	case JobStepStateDone, JobStepStateSkipped, JobStepStateFailed:
		q += `, finished_at = ?`
		args = append(args, now)
	}
	q += ` WHERE job_id = ? AND step = ?`
	args = append(args, jobID, string(step))
	res, err := s.db.Conn().ExecContext(ctx, q, args...)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// AppendJobStepNotes merges the given map into the step's notes_json.
// Concurrent appenders may race; M1.1's pipeline is single-writer per
// step so this is acceptable.
func (s *Store) AppendJobStepNotes(ctx context.Context, jobID string, step StepID, extra map[string]any) error {
	if len(extra) == 0 {
		return nil
	}
	tx, err := s.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var current string
	row := tx.QueryRowContext(ctx,
		`SELECT notes_json FROM job_steps WHERE job_id = ? AND step = ?`,
		jobID, string(step))
	if err := row.Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}

	merged := map[string]any{}
	if current != "" {
		if err := json.Unmarshal([]byte(current), &merged); err != nil {
			return fmt.Errorf("unmarshal notes_json: %w", err)
		}
	}
	for k, v := range extra {
		merged[k] = v
	}
	encoded, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal notes_json: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE job_steps SET notes_json = ? WHERE job_id = ? AND step = ?`,
		string(encoded), jobID, string(step)); err != nil {
		return err
	}
	return tx.Commit()
}

// ---- LOG LINES ------------------------------------------------------------

// AppendLogLine inserts one row.
func (s *Store) AppendLogLine(ctx context.Context, l LogLine) error {
	_, err := s.db.Conn().ExecContext(ctx, `
		INSERT INTO log_lines (job_id, t, step, level, message)
		VALUES (?, ?, ?, ?, ?)`,
		l.JobID, timestamp(l.T), string(l.Step), string(l.Level), l.Message)
	return err
}

// TailLogLines returns the most recent n log lines for a job, oldest
// first (the same order they'd appear in a follow-the-tail UI). n<=0
// defaults to 200.
func (s *Store) TailLogLines(ctx context.Context, jobID string, n int) ([]LogLine, error) {
	if n <= 0 {
		n = 200
	}
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT job_id, t, step, level, message FROM log_lines
		WHERE job_id = ? ORDER BY id DESC LIMIT ?`, jobID, n)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var reversed []LogLine
	for rows.Next() {
		var l LogLine
		var tStr, step, level string
		if err := rows.Scan(&l.JobID, &tStr, &step, &level, &l.Message); err != nil {
			return nil, err
		}
		l.Step = StepID(step)
		l.Level = LogLevel(level)
		t, err := parseTime(tStr)
		if err != nil {
			return nil, fmt.Errorf("parse t: %w", err)
		}
		l.T = t
		reversed = append(reversed, l)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	return reversed, nil
}

// LogFilter narrows ListLogLines. Empty Step matches every line. Limit
// is clamped to (0, 2000]; Offset is clamped to >= 0.
type LogFilter struct {
	Step   StepID
	Limit  int
	Offset int
}

// ListLogLines returns log lines for a job in insertion order
// (oldest → newest), optionally filtered by step, paginated by
// limit/offset. Also returns the total matching count (ignoring the
// limit/offset window) so the webui can size its paging.
func (s *Store) ListLogLines(ctx context.Context, jobID string, f LogFilter) ([]LogLine, int, error) {
	if f.Limit <= 0 {
		f.Limit = 500
	}
	if f.Limit > 2000 {
		f.Limit = 2000
	}
	if f.Offset < 0 {
		f.Offset = 0
	}

	args := []any{jobID}
	where := "WHERE job_id = ?"
	if f.Step != "" {
		where += " AND step = ?"
		args = append(args, string(f.Step))
	}

	var total int
	if err := s.db.Conn().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM log_lines `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, f.Limit, f.Offset)
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT job_id, t, step, level, message FROM log_lines
		`+where+` ORDER BY id ASC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()
	var out []LogLine
	for rows.Next() {
		var l LogLine
		var tStr, step, level string
		if err := rows.Scan(&l.JobID, &tStr, &step, &level, &l.Message); err != nil {
			return nil, 0, err
		}
		l.Step = StepID(step)
		l.Level = LogLevel(level)
		t, err := parseTime(tStr)
		if err != nil {
			return nil, 0, fmt.Errorf("parse t: %w", err)
		}
		l.T = t
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// triggersInclude reports whether `triggers` (a comma-separated list)
// contains the given trigger token.
func triggersInclude(triggers, trigger string) bool {
	for _, t := range strings.Split(triggers, ",") {
		if strings.TrimSpace(t) == trigger {
			return true
		}
	}
	return false
}

// ---- NOTIFICATIONS --------------------------------------------------------

// CreateNotification inserts a new notification.
func (s *Store) CreateNotification(ctx context.Context, n *Notification) error {
	if n.ID == "" {
		n.ID = NewID()
	}
	now := time.Now()
	if n.CreatedAt.IsZero() {
		n.CreatedAt = now
	}
	if n.UpdatedAt.IsZero() {
		n.UpdatedAt = now
	}
	if n.Triggers == "" {
		n.Triggers = "done,failed"
	}
	_, err := s.db.Conn().ExecContext(ctx, `
		INSERT INTO notifications (id, name, url, tags, triggers, enabled,
		                           created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.ID, n.Name, n.URL, n.Tags, n.Triggers, boolToInt(n.Enabled),
		timestamp(n.CreatedAt), timestamp(n.UpdatedAt))
	return err
}

// GetNotification fetches a notification by ID.
func (s *Store) GetNotification(ctx context.Context, id string) (*Notification, error) {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, name, url, tags, triggers, enabled, created_at, updated_at
		FROM notifications WHERE id = ?`, id)
	return scanNotification(row)
}

// ListNotifications returns every notification ordered by name.
func (s *Store) ListNotifications(ctx context.Context) ([]Notification, error) {
	return s.queryNotifications(ctx, `
		SELECT id, name, url, tags, triggers, enabled, created_at, updated_at
		FROM notifications ORDER BY name`)
}

// ListNotificationsForTrigger returns enabled notifications whose
// triggers list contains the given token. SQL pre-filter is by LIKE
// (cheap) then post-checked in Go to avoid false positives like
// "donezo" matching "done".
func (s *Store) ListNotificationsForTrigger(ctx context.Context, trigger string) ([]Notification, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT id, name, url, tags, triggers, enabled, created_at, updated_at
		FROM notifications
		WHERE enabled = 1 AND triggers LIKE '%' || ? || '%'`, trigger)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		if triggersInclude(n.Triggers, trigger) {
			out = append(out, *n)
		}
	}
	return out, rows.Err()
}

// UpdateNotification rewrites every mutable column. ID + CreatedAt stay.
func (s *Store) UpdateNotification(ctx context.Context, n *Notification) error {
	n.UpdatedAt = time.Now()
	res, err := s.db.Conn().ExecContext(ctx, `
		UPDATE notifications SET name = ?, url = ?, tags = ?, triggers = ?,
		                         enabled = ?, updated_at = ?
		WHERE id = ?`,
		n.Name, n.URL, n.Tags, n.Triggers, boolToInt(n.Enabled),
		timestamp(n.UpdatedAt), n.ID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteNotification removes a notification.
func (s *Store) DeleteNotification(ctx context.Context, id string) error {
	res, err := s.db.Conn().ExecContext(ctx, `DELETE FROM notifications WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) queryNotifications(ctx context.Context, q string, args ...any) ([]Notification, error) {
	rows, err := s.db.Conn().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Notification
	for rows.Next() {
		n, err := scanNotification(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *n)
	}
	return out, rows.Err()
}

func scanNotification(r rowScanner) (*Notification, error) {
	var n Notification
	var enabled int
	var createdStr, updatedStr string
	if err := r.Scan(&n.ID, &n.Name, &n.URL, &n.Tags, &n.Triggers, &enabled, &createdStr, &updatedStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	n.Enabled = enabled != 0
	var err error
	if n.CreatedAt, err = parseTime(createdStr); err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	if n.UpdatedAt, err = parseTime(updatedStr); err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	return &n, nil
}

// ---- SETTINGS -------------------------------------------------------------

// GetSetting returns the value for key. ErrNotFound if missing.
func (s *Store) GetSetting(ctx context.Context, key string) (string, error) {
	row := s.db.Conn().QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key)
	var v string
	if err := row.Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}

// GetAllSettings returns every key-value pair as a map.
func (s *Store) GetAllSettings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// GetBool returns the value for key parsed as bool. Returns (false, nil)
// if the key is missing or unparseable — callers treat absent settings as false.
func (s *Store) GetBool(ctx context.Context, key string) (bool, error) {
	v, err := s.GetSetting(ctx, key)
	if err != nil || v == "" {
		return false, nil
	}
	b, perr := strconv.ParseBool(v)
	if perr != nil {
		return false, nil
	}
	return b, nil
}

// GetInt returns the value for key parsed as int. Returns (0, nil)
// if the key is missing or unparseable.
func (s *Store) GetInt(ctx context.Context, key string) (int, error) {
	v, err := s.GetSetting(ctx, key)
	if err != nil || v == "" {
		return 0, nil
	}
	n, perr := strconv.Atoi(v)
	if perr != nil {
		return 0, nil
	}
	return n, nil
}

// SetSetting upserts (key, value).
func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.Conn().ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		key, value, timestamp(time.Now()))
	return err
}

// ---- HISTORY (query layer over jobs + discs) ------------------------------

// HistoryFilter narrows ListHistory.
type HistoryFilter struct {
	Type   DiscType
	From   time.Time
	To     time.Time
	Limit  int
	Offset int
}

// HistoryRow is one finished job + the disc it ripped.
type HistoryRow struct {
	Job  Job  `json:"job"`
	Disc Disc `json:"disc"`
}

// ListHistory returns finished jobs (state in done/failed/cancelled)
// joined with their disc, ordered by finished_at DESC.
func (s *Store) ListHistory(ctx context.Context, f HistoryFilter) ([]HistoryRow, error) {
	if f.Limit <= 0 {
		f.Limit = 50
	}
	if f.Limit > 200 {
		f.Limit = 200
	}

	q := `
		SELECT
		  j.id, j.disc_id, COALESCE(j.drive_id, ''), j.profile_id, j.state, j.active_step,
		  j.progress, j.speed, j.eta_seconds, j.elapsed_seconds, j.output_bytes,
		  j.started_at, j.finished_at, j.error_message, j.created_at,
		  d.id, COALESCE(d.drive_id, ''), d.type, d.title, d.year, d.runtime_seconds,
		  d.size_bytes_raw, d.toc_hash, d.metadata_provider, d.metadata_id,
		  d.candidates_json, d.metadata_json, d.created_at
		FROM jobs j
		JOIN discs d ON j.disc_id = d.id
		WHERE j.state IN ('done','failed','cancelled')
	`
	args := []any{}
	if f.Type != "" {
		q += " AND d.type = ?"
		args = append(args, string(f.Type))
	}
	if !f.From.IsZero() {
		q += " AND j.finished_at >= ?"
		args = append(args, timestamp(f.From))
	}
	if !f.To.IsZero() {
		q += " AND j.finished_at <= ?"
		args = append(args, timestamp(f.To))
	}
	q += " ORDER BY j.finished_at DESC LIMIT ? OFFSET ?"
	args = append(args, f.Limit, f.Offset)

	rows, err := s.db.Conn().QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var out []HistoryRow
	for rows.Next() {
		var (
			j                                              Job
			d                                              Disc
			jState, jActive, jStarted, jFinished, jCreated string
			dType, dCands, dMeta, dCreated                 string
		)
		if err := rows.Scan(
			&j.ID, &j.DiscID, &j.DriveID, &j.ProfileID, &jState, &jActive,
			&j.Progress, &j.Speed, &j.ETASeconds, &j.ElapsedSeconds, &j.OutputBytes,
			&jStarted, &jFinished, &j.ErrorMessage, &jCreated,
			&d.ID, &d.DriveID, &dType, &d.Title, &d.Year, &d.RuntimeSeconds,
			&d.SizeBytesRaw, &d.TOCHash, &d.MetadataProvider, &d.MetadataID,
			&dCands, &dMeta, &dCreated,
		); err != nil {
			return nil, err
		}
		j.State = JobState(jState)
		j.ActiveStep = StepID(jActive)
		var err error
		if j.StartedAt, err = parseTimePtr(jStarted); err != nil {
			return nil, fmt.Errorf("parse j.started_at: %w", err)
		}
		if j.FinishedAt, err = parseTimePtr(jFinished); err != nil {
			return nil, fmt.Errorf("parse j.finished_at: %w", err)
		}
		if j.CreatedAt, err = parseTime(jCreated); err != nil {
			return nil, fmt.Errorf("parse j.created_at: %w", err)
		}
		d.Type = DiscType(dType)
		if d.Candidates, err = unmarshalCandidates(dCands); err != nil {
			return nil, fmt.Errorf("unmarshal candidates: %w", err)
		}
		d.MetadataJSON = dMeta
		if d.CreatedAt, err = parseTime(dCreated); err != nil {
			return nil, fmt.Errorf("parse d.created_at: %w", err)
		}
		out = append(out, HistoryRow{Job: j, Disc: d})
	}
	return out, rows.Err()
}

// CountHistory returns the count of rows matching the filter.
func (s *Store) CountHistory(ctx context.Context, f HistoryFilter) (int, error) {
	q := `SELECT COUNT(*) FROM jobs j WHERE j.state IN ('done','failed','cancelled')`
	args := []any{}
	if f.Type != "" {
		q = `SELECT COUNT(*) FROM jobs j JOIN discs d ON j.disc_id = d.id
		     WHERE j.state IN ('done','failed','cancelled') AND d.type = ?`
		args = append(args, string(f.Type))
	}
	if !f.From.IsZero() {
		q += " AND j.finished_at >= ?"
		args = append(args, timestamp(f.From))
	}
	if !f.To.IsZero() {
		q += " AND j.finished_at <= ?"
		args = append(args, timestamp(f.To))
	}
	row := s.db.Conn().QueryRowContext(ctx, q, args...)
	var n int
	if err := row.Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// PruneHistoryBefore deletes jobs in {done, failed, cancelled} whose
// finished_at is before cutoff. Returns the number of jobs deleted.
// FK cascades remove job_steps and log_lines automatically; orphan
// discs are pruned in the same transaction.
func (s *Store) PruneHistoryBefore(ctx context.Context, cutoff time.Time) (int, error) {
	tx, err := s.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		DELETE FROM jobs
		WHERE state IN ('done','failed','cancelled')
		  AND finished_at IS NOT NULL
		  AND finished_at < ?`, cutoff.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("delete jobs: %w", err)
	}
	deleted, _ := res.RowsAffected()

	// Drop discs that are no longer referenced by any job.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM discs
		WHERE NOT EXISTS (SELECT 1 FROM jobs WHERE jobs.disc_id = discs.id)`); err != nil {
		return 0, fmt.Errorf("delete orphan discs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return int(deleted), nil
}

// DeleteJobAndOrphans removes one job (and its FK-cascaded step + log
// rows), then drops the disc if no other job references it. Used by
// DELETE /api/jobs/:id for single-row history pruning; callers are
// responsible for refusing non-terminal states before invoking this.
func (s *Store) DeleteJobAndOrphans(ctx context.Context, jobID string) error {
	tx, err := s.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var discID string
	if err := tx.QueryRowContext(ctx,
		`SELECT disc_id FROM jobs WHERE id = ?`, jobID).Scan(&discID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("lookup disc_id: %w", err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM jobs WHERE id = ?`, jobID); err != nil {
		return fmt.Errorf("delete job: %w", err)
	}

	// Drop the disc only if no other job references it. Active rips on
	// the same disc keep the row alive.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM discs
		WHERE id = ?
		  AND NOT EXISTS (SELECT 1 FROM jobs WHERE jobs.disc_id = discs.id)`, discID); err != nil {
		return fmt.Errorf("delete orphan disc: %w", err)
	}

	return tx.Commit()
}

// ClearHistory deletes every finished job (done/failed/cancelled), the
// job_steps and log_lines they own (via FK cascade), and the disc rows
// left with no job at all once those jobs are gone. In-progress jobs
// (queued/identifying/running/paused) and their discs are untouched,
// as are discs that never had a job (e.g. one still awaiting a
// decision). Ripped files on disk are not touched. Returns the number
// of jobs deleted. Runs in a single transaction.
func (s *Store) ClearHistory(ctx context.Context) (int, error) {
	tx, err := s.db.Conn().BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Count the finished jobs up front — once the deletes (and their FK
	// cascades) run, RowsAffected can't see cascade-deleted rows.
	var deleted int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE state IN ('done','failed','cancelled')`).Scan(&deleted); err != nil {
		return 0, fmt.Errorf("count finished jobs: %w", err)
	}

	// Drop discs whose every job is finished — they had history and have
	// no active rip. The jobs.disc_id ON DELETE CASCADE removes those
	// discs' jobs, job_steps and log_lines. The first EXISTS clause
	// keeps discs that never had a job (one still awaiting a decision).
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM discs
		WHERE EXISTS (SELECT 1 FROM jobs j WHERE j.disc_id = discs.id)
		  AND NOT EXISTS (
		    SELECT 1 FROM jobs j WHERE j.disc_id = discs.id
		      AND j.state IN ('queued','identifying','running','paused'))`); err != nil {
		return 0, fmt.Errorf("delete history discs: %w", err)
	}

	// Mop up finished jobs on discs that were KEPT (a disc with both a
	// finished job and an active re-rip).
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM jobs
		WHERE state IN ('done','failed','cancelled')`); err != nil {
		return 0, fmt.Errorf("delete finished jobs: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit: %w", err)
	}
	return deleted, nil
}

// Stats computes the dashboard's top-widgets payload. Library
// total_bytes is left zero — that's filled in by the API layer via
// statfs against the library roots (the daemon's state package
// shouldn't reach for the OS filesystem). ActiveJobs.Delta1h and
// Spark24h are similarly zero-filled here and stitched in by the API
// layer's in-memory active-jobs sampler.
func (s *Store) Stats(ctx context.Context, now time.Time) (Stats, error) {
	var out Stats
	if err := s.statsActive(ctx, &out.ActiveJobs); err != nil {
		return out, err
	}
	if err := s.statsTodayRipped(ctx, now, &out.TodayRipped); err != nil {
		return out, err
	}
	if err := s.statsLibrary(ctx, now, &out.Library); err != nil {
		return out, err
	}
	if err := s.statsFailures(ctx, now, &out.Failures7d); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) statsActive(ctx context.Context, out *ActiveJobsStat) error {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE state NOT IN ('done','failed','cancelled','interrupted')`)
	if err := row.Scan(&out.Value); err != nil {
		return err
	}
	out.Spark24h = make([]int, 24)
	return nil
}

func (s *Store) statsTodayRipped(ctx context.Context, now time.Time, out *TodayRippedStat) error {
	startOfToday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Format(time.RFC3339)
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT COALESCE(SUM(output_bytes), 0), COUNT(*)
		FROM jobs WHERE state='done' AND finished_at >= ?`, startOfToday)
	if err := row.Scan(&out.Bytes, &out.Titles); err != nil {
		return err
	}
	spark, err := s.dailyByteSeries(ctx, now, 7)
	if err != nil {
		return err
	}
	out.Spark7dBytes = spark
	return nil
}

func (s *Store) statsLibrary(ctx context.Context, now time.Time, out *LibraryStat) error {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT COALESCE(SUM(output_bytes), 0)
		FROM jobs WHERE state='done'`)
	if err := row.Scan(&out.UsedBytes); err != nil {
		return err
	}
	// Build a cumulative-at-end-of-day series for the last 30 days.
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT finished_at, output_bytes
		FROM jobs WHERE state='done' ORDER BY finished_at ASC`)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type pt struct {
		t time.Time
		b int64
	}
	var all []pt
	for rows.Next() {
		var ts string
		var b int64
		if err := rows.Scan(&ts, &b); err != nil {
			return err
		}
		t, _ := time.Parse(time.RFC3339, ts)
		all = append(all, pt{t, b})
	}
	out.Spark30dUsed = make([]int64, 30)
	dayEnd := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, now.Location())
	for i := 0; i < 30; i++ {
		thisDayEnd := dayEnd.AddDate(0, 0, -(29 - i))
		var sum int64
		for _, p := range all {
			if !p.t.After(thisDayEnd) {
				sum += p.b
			}
		}
		out.Spark30dUsed[i] = sum
	}
	return nil
}

func (s *Store) statsFailures(ctx context.Context, now time.Time, out *Failures7dStat) error {
	cutCurr := now.AddDate(0, 0, -7).Format(time.RFC3339)
	cutPrev := now.AddDate(0, 0, -14).Format(time.RFC3339)

	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE state IN ('failed','cancelled','interrupted')
		  AND finished_at >= ?`, cutCurr)
	if err := row.Scan(&out.Value); err != nil {
		return err
	}
	row = s.db.Conn().QueryRowContext(ctx, `
		SELECT COUNT(*) FROM jobs
		WHERE state IN ('failed','cancelled','interrupted')
		  AND finished_at >= ? AND finished_at < ?`, cutPrev, cutCurr)
	if err := row.Scan(&out.Previous); err != nil {
		return err
	}

	out.Spark30d = make([]int, 30)
	cut30 := now.AddDate(0, 0, -30).Format(time.RFC3339)
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT date(finished_at), COUNT(*)
		FROM jobs WHERE state IN ('failed','cancelled','interrupted')
		  AND finished_at >= ?
		GROUP BY date(finished_at)`, cut30)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return err
		}
		idx := dayOffsetIndex(now, day, 30)
		if idx >= 0 && idx < 30 {
			out.Spark30d[idx] = n
		}
	}
	return nil
}

// dailyByteSeries returns the per-day SUM(output_bytes) over the last
// `days` calendar days as a slice of length `days` (oldest first).
// Missing days are zero-filled.
func (s *Store) dailyByteSeries(ctx context.Context, now time.Time, days int) ([]int64, error) {
	cut := now.AddDate(0, 0, -days).Format(time.RFC3339)
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT date(finished_at), SUM(output_bytes)
		FROM jobs WHERE state='done' AND finished_at >= ?
		GROUP BY date(finished_at)`, cut)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]int64, days)
	for rows.Next() {
		var day string
		var n int64
		if err := rows.Scan(&day, &n); err != nil {
			return nil, err
		}
		idx := dayOffsetIndex(now, day, days)
		if idx >= 0 && idx < days {
			out[idx] = n
		}
	}
	return out, nil
}

// dayOffsetIndex maps a 'YYYY-MM-DD' day string to its bucket index in
// a window of `days` days ending today. days-1 is today; 0 is the
// oldest day in the window. Returns -1 if the day is outside the
// window or unparseable.
func dayOffsetIndex(now time.Time, ymd string, days int) int {
	t, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		return -1
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	bucket := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, now.Location())
	diff := int(today.Sub(bucket).Hours() / 24)
	idx := (days - 1) - diff
	if idx < 0 || idx >= days {
		return -1
	}
	return idx
}

// Conn exposes the underlying *sql.DB. Used by the API layer's
// active-jobs sampler, which needs a single COUNT query without going
// through the full Stats aggregator.
func (s *Store) Conn() *sql.DB {
	return s.db.Conn()
}
