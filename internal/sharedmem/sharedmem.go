// Package sharedmem implements a shared memory store backed by SQLite FTS5.
//
// Agents can POST memories (key-value pairs with tags) and QUERY
// the shared memory using full-text search. This enables cross-agent
// knowledge sharing without external dependencies.
//
// Schema:
//
//	CREATE TABLE memories (
//	  id TEXT PRIMARY KEY,
//	  agent_id TEXT NOT NULL,
//	  key TEXT NOT NULL,
//	  value TEXT NOT NULL,
//	  tags TEXT DEFAULT '',  -- comma-separated
//	  created_at TEXT NOT NULL,
//	  updated_at TEXT NOT NULL
//	);
//	CREATE VIRTUAL TABLE memories_fts USING fts5(
//	  key, value, tags,
//	  content=memories,
//	  content_rowid=rowid
//	);
package sharedmem

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Store manages shared memories in SQLite with FTS5.
type Store struct {
	db *sql.DB
}

// New creates a new shared memory store. It creates the schema
// if it doesn't exist. The db must be an open SQLite connection.
func New(db *sql.DB) (*Store, error) {
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("sharedmem migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			key TEXT NOT NULL,
			value TEXT NOT NULL,
			tags TEXT DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}
	// FTS5 virtual table — if it already exists, skip
	_, _ = s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
			key, value, tags,
			content=memories,
			content_rowid=rowid
		);
	`)
	// Triggers to keep FTS in sync
	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS memories_ai AFTER INSERT ON memories BEGIN
			INSERT INTO memories_fts(rowid, key, value, tags)
			VALUES (new.rowid, new.key, new.value, new.tags);
		END;
	`)
	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS memories_ad AFTER DELETE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, key, value, tags)
			VALUES ('delete', old.rowid, old.key, old.value, old.tags);
		END;
	`)
	_, _ = s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS memories_au AFTER UPDATE ON memories BEGIN
			INSERT INTO memories_fts(memories_fts, rowid, key, value, tags)
			VALUES ('delete', old.rowid, old.key, old.value, old.tags);
			INSERT INTO memories_fts(rowid, key, value, tags)
			VALUES (new.rowid, new.key, new.value, new.tags);
		END;
	`)
	return nil
}

// Memory represents a shared memory entry.
type Memory struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	Key       string `json:"key"`
	Value     string `json:"value"`
	Tags      string `json:"tags"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	// Source identifies the provider that produced this entry
	// (SourceFTS5 for the local Store, SourceMem0 for Mem0).
	// It is set in code only and is not persisted in the database.
	Source string `json:"source"`
}

// Post creates or updates a memory entry.
func (s *Store) Post(ctx context.Context, m *Memory) error {
	now := time.Now().UTC().Format(time.RFC3339)
	if m.ID == "" {
		m.ID = fmt.Sprintf("mem-%d", time.Now().UnixNano())
	}
	if m.CreatedAt == "" {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if m.Source == "" {
		m.Source = SourceFTS5
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO memories (id, agent_id, key, value, tags, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			value = excluded.value,
			tags = excluded.tags,
			updated_at = excluded.updated_at
	`, m.ID, m.AgentID, m.Key, m.Value, m.Tags, m.CreatedAt, m.UpdatedAt)
	return err
}

// Query searches memories using FTS5 full-text search.
func (s *Store) Query(ctx context.Context, query string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.agent_id, m.key, m.value, m.tags, m.created_at, m.updated_at
		FROM memories m
		JOIN memories_fts f ON m.rowid = f.rowid
		WHERE memories_fts MATCH ?
		ORDER BY rank
		LIMIT ?
	`, query, limit)
	if err != nil {
		// Fallback to LIKE if FTS query is invalid
		return s.queryLike(ctx, query, limit)
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m := &Memory{Source: SourceFTS5}
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Key, &m.Value, &m.Tags, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// queryLike is a fallback when FTS5 syntax is invalid.
func (s *Store) queryLike(ctx context.Context, query string, limit int) ([]*Memory, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, key, value, tags, created_at, updated_at
		FROM memories
		WHERE key LIKE ? OR value LIKE ? OR tags LIKE ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, pattern, pattern, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m := &Memory{Source: SourceFTS5}
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Key, &m.Value, &m.Tags, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// List returns all memories for a specific agent.
func (s *Store) List(ctx context.Context, agentID string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, key, value, tags, created_at, updated_at
		FROM memories
		WHERE agent_id = ?
		ORDER BY updated_at DESC
		LIMIT ?
	`, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m := &Memory{Source: SourceFTS5}
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Key, &m.Value, &m.Tags, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}

// Delete removes a memory by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM memories WHERE id = ?`, id)
	return err
}

// SearchByTag returns memories matching all given tags.
func (s *Store) SearchByTag(ctx context.Context, tags []string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	// Build a query that matches all tags
	conditions := make([]string, len(tags))
	args := make([]any, len(tags))
	for i, tag := range tags {
		conditions[i] = "tags LIKE ?"
		args[i] = "%" + tag + "%"
	}
	query := fmt.Sprintf(`
		SELECT id, agent_id, key, value, tags, created_at, updated_at
		FROM memories
		WHERE %s
		ORDER BY updated_at DESC
		LIMIT ?
	`, strings.Join(conditions, " AND "))
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Memory
	for rows.Next() {
		m := &Memory{Source: SourceFTS5}
		if err := rows.Scan(&m.ID, &m.AgentID, &m.Key, &m.Value, &m.Tags, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		results = append(results, m)
	}
	return results, rows.Err()
}
