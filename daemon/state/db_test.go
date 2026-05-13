package state_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jumpingmushroom/DiscEcho/daemon/state"
)

func TestOpen_AppliesMigrationsOnFreshDB(t *testing.T) {
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	row := db.Conn().QueryRowContext(context.Background(),
		"SELECT MAX(version) FROM schema_migrations")
	var v int
	if err := row.Scan(&v); err != nil {
		t.Fatalf("scan version: %v", err)
	}
	if v != 5 {
		t.Errorf("schema_migrations max version: want 5, got %d", v)
	}

	for _, tbl := range []string{
		"drives", "discs", "profiles", "jobs", "job_steps",
		"log_lines", "notifications", "settings", "schema_migrations",
	} {
		var name string
		row := db.Conn().QueryRowContext(context.Background(),
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl)
		if err := row.Scan(&name); err != nil {
			t.Errorf("table %s missing: %v", tbl, err)
		}
	}
}

func TestOpen_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	db1, err := state.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = db1.Close()

	db2, err := state.Open(path)
	if err != nil {
		t.Fatalf("re-Open: %v", err)
	}
	defer func() { _ = db2.Close() }()

	row := db2.Conn().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM schema_migrations")
	var n int
	if err := row.Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("schema_migrations rows after second open: want 5, got %d", n)
	}
}

func TestOpen_EnforcesForeignKeys(t *testing.T) {
	dir := t.TempDir()
	db, err := state.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_, err = db.Conn().ExecContext(context.Background(),
		`INSERT INTO jobs (id, disc_id, profile_id, state, created_at)
		 VALUES ('j1', 'disc-nope', 'prof-nope', 'queued', '2026-01-01T00:00:00Z')`)
	if err == nil {
		t.Errorf("FK violation should have failed")
	}
}

// TestMigration003_FlipsDVDMovieDefaults seeds a DVD-Movie row matching
// the pre-003 shape, then executes the migration 003 SQL directly to
// confirm its UPDATE flips the row to MKV + main_feature. We don't
// re-Open the DB because schema-altering migrations (004+) aren't
// idempotent and the migration runner uses MAX(version) — replaying
// 003 by wiping its schema_migrations row doesn't work once newer
// schema-mutating migrations exist.
func TestMigration003_FlipsDVDMovieDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	db, err := state.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	if _, err := db.Conn().ExecContext(ctx, `
		INSERT INTO profiles (id, disc_type, name, engine, format, preset,
		                      container, video_codec, quality_preset,
		                      drive_policy, auto_eject, options_json,
		                      output_path_template, enabled, step_count,
		                      created_at, updated_at)
		VALUES ('dvd-mov-test', 'DVD', 'DVD-Movie', 'HandBrake', 'MP4',
		        'x264 RF 20', 'MP4', 'x264', 'x264 RF 20', 'any', 1,
		        '{}',
		        '{{.Title}} ({{.Year}})/{{.Title}} ({{.Year}}).mp4',
		        1, 7, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}

	// Replay migration 003's body directly. Idempotent for our purposes:
	// the WHERE clauses target the pre-003 seed shape, which is exactly
	// what the test row matches.
	body, err := migrationBody("003_dvd_default_mkv.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Conn().ExecContext(ctx, body); err != nil {
		t.Fatalf("re-exec migration 003: %v", err)
	}

	row := db.Conn().QueryRowContext(ctx,
		`SELECT format, container, output_path_template, options_json
		   FROM profiles WHERE id = 'dvd-mov-test'`)
	var format, container, tmpl, opts string
	if err := row.Scan(&format, &container, &tmpl, &opts); err != nil {
		t.Fatal(err)
	}
	if format != "MKV" {
		t.Errorf("format: want MKV, got %s", format)
	}
	if container != "MKV" {
		t.Errorf("container: want MKV, got %s", container)
	}
	if !strings.HasSuffix(tmpl, ".mkv") {
		t.Errorf("template suffix: %s", tmpl)
	}
	if !strings.Contains(opts, `"dvd_selection_mode":"main_feature"`) {
		t.Errorf("options_json missing dvd_selection_mode: %s", opts)
	}
}

// migrationBody loads a migration file by name from the embed FS. Used
// only by tests that need to re-apply a specific migration's SQL.
func migrationBody(name string) (string, error) {
	// The embed FS lives in db.go; expose its bytes here via a small
	// helper that re-reads the file from disk under daemon/state/.
	b, err := os.ReadFile(filepath.Join("migrations", name))
	if err != nil {
		return "", err
	}
	return string(b), nil
}
