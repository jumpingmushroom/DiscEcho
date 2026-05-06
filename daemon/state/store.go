package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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
	_, err = s.db.Conn().ExecContext(ctx, `
		INSERT INTO discs (id, drive_id, type, title, year, runtime_seconds,
		                   size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		                   candidates_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, nullString(d.DriveID), string(d.Type), d.Title, d.Year, d.RuntimeSeconds,
		d.SizeBytesRaw, d.TOCHash, d.MetadataProvider, d.MetadataID,
		candJSON, timestamp(d.CreatedAt))
	return err
}

// GetDisc fetches a disc by ID, including its candidates.
func (s *Store) GetDisc(ctx context.Context, id string) (*Disc, error) {
	row := s.db.Conn().QueryRowContext(ctx, `
		SELECT id, COALESCE(drive_id, ''), type, title, year, runtime_seconds,
		       size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		       candidates_json, created_at
		FROM discs WHERE id = ?`, id)
	return scanDisc(row)
}

// ListDiscsForDrive returns discs that were inserted in the given drive,
// most recent first.
func (s *Store) ListDiscsForDrive(ctx context.Context, driveID string) ([]Disc, error) {
	rows, err := s.db.Conn().QueryContext(ctx, `
		SELECT id, COALESCE(drive_id, ''), type, title, year, runtime_seconds,
		       size_bytes_raw, toc_hash, metadata_provider, metadata_id,
		       candidates_json, created_at
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
	var dtype, candJSON, createdStr string
	if err := r.Scan(
		&d.ID, &d.DriveID, &dtype, &d.Title, &d.Year, &d.RuntimeSeconds,
		&d.SizeBytesRaw, &d.TOCHash, &d.MetadataProvider, &d.MetadataID,
		&candJSON, &createdStr,
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
	t, err := parseTime(createdStr)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	d.CreatedAt = t
	return &d, nil
}
