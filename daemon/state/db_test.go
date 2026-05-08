package state_test

import (
	"context"
	"path/filepath"
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
	if v != 2 {
		t.Errorf("schema_migrations max version: want 2, got %d", v)
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
	if n != 2 {
		t.Errorf("schema_migrations rows after second open: want 2, got %d", n)
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
