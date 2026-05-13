package state_test

import (
	"context"
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
	if v != 3 {
		t.Errorf("schema_migrations max version: want 3, got %d", v)
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
	if n != 3 {
		t.Errorf("schema_migrations rows after second open: want 3, got %d", n)
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
// the pre-003 shape, then re-runs migration 003 to confirm it flips the
// row to MKV + main_feature.
func TestMigration003_FlipsDVDMovieDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.sqlite")

	db, err := state.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Wipe schema_migrations entry 003 so we can re-run it after we
	// insert a row that looks like the pre-003 seed shape.
	if _, err := db.Conn().ExecContext(ctx,
		`DELETE FROM schema_migrations WHERE version = 3`); err != nil {
		t.Fatal(err)
	}
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
	_ = db.Close()

	db2, err := state.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db2.Close() }()

	row := db2.Conn().QueryRowContext(ctx,
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
