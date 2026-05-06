package state

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

// DB wraps a *sql.DB and exposes only the operations the rest of the
// daemon needs. Closing it closes the underlying connection pool.
type DB struct {
	db *sql.DB
}

// Conn returns the underlying *sql.DB. Used by Store; not exported for
// external callers (Store is the canonical access path).
func (d *DB) Conn() *sql.DB { return d.db }

// Close closes the connection pool.
func (d *DB) Close() error { return d.db.Close() }

// Open opens (or creates) a SQLite database at path with WAL mode and
// foreign keys enabled, then runs any pending migrations.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", path)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite open: %w", err)
	}
	if err := conn.Ping(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("sqlite ping: %w", err)
	}
	d := &DB{db: conn}
	if err := d.migrate(context.Background()); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	type file struct {
		version int
		name    string
	}
	var files []file
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		idx := strings.IndexByte(e.Name(), '_')
		if idx < 1 {
			return fmt.Errorf("malformed migration filename: %s", e.Name())
		}
		v, err := strconv.Atoi(e.Name()[:idx])
		if err != nil {
			return fmt.Errorf("malformed migration version in %s: %w", e.Name(), err)
		}
		files = append(files, file{version: v, name: e.Name()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })

	// Bootstrap schema_migrations on a fresh DB. The first migration
	// creates the table; before that we treat MAX(version) as 0.
	current := 0
	row := d.db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'")
	var existing string
	if err := row.Scan(&existing); err == nil {
		row := d.db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations")
		if err := row.Scan(&current); err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}
	}

	for _, f := range files {
		if f.version <= current {
			continue
		}
		body, err := fs.ReadFile(migrationFS, "migrations/"+f.name)
		if err != nil {
			return fmt.Errorf("read %s: %w", f.name, err)
		}
		if err := d.applyMigration(ctx, f.version, string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", f.name, err)
		}
	}
	return nil
}

func (d *DB) applyMigration(ctx context.Context, version int, body string) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, body); err != nil {
		return fmt.Errorf("exec migration body: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at) VALUES (?, ?)",
		version, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	return tx.Commit()
}
