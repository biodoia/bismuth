// Package db is the SQLite persistence layer for bismuth.
//
// Uses modernc.org/sqlite (pure Go, cgo-free). Schema lives in
// /migrations/*.sql and is applied at startup via the embedded FS.
//
// Tables (V1): see migrations/001_init.sql for the canonical schema.
package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store wraps the SQLite handle plus prepared statements.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the SQLite database at path and applies
// pending migrations.
func Open(ctx context.Context, path string) (*Store, error) {
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}

	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", path)
	raw, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	raw.SetMaxOpenConns(1) // SQLite + WAL is happiest single-writer

	s := &Store{db: raw}
	if err := s.migrate(ctx); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close releases the underlying connection.
func (s *Store) Close() error { return s.db.Close() }

// DB returns the underlying *sql.DB for advanced callers.
func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context) error {
	// Track applied migrations so non-idempotent statements (ALTER TABLE
	// ADD COLUMN has no IF NOT EXISTS in SQLite) run exactly once.
	if _, err := s.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS schema_migrations(
			name TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now')))`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names) // 001_*.sql, 002_*.sql, ...
	for _, n := range names {
		var seen int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations WHERE name=?`, n).Scan(&seen); err != nil {
			return err
		}
		if seen > 0 {
			continue
		}
		b, err := fs.ReadFile(migrationsFS, "migrations/"+n)
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("apply %s: %w", n, err)
		}
		if _, err := s.db.ExecContext(ctx,
			`INSERT INTO schema_migrations(name) VALUES(?)`, n); err != nil {
			return err
		}
	}
	return nil
}
