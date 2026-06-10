package sharedmem

import "context"

// Source labels identify which backend produced or served a Memory.
// They are set in code only; the SQLite schema is unchanged.
const (
	// SourceFTS5 marks memories served by the local SQLite FTS5 Store.
	SourceFTS5 = "fts5"
	// SourceMem0 marks memories served by a Mem0 HTTP backend.
	SourceMem0 = "mem0"
)

// Provider is a pluggable storage backend for shared memories.
//
// *Store (SQLite FTS5) and *Mem0 (Mem0 REST API) both implement it,
// and NewFallback composes two Providers into one. The API layer can
// hold a Provider instead of a concrete *Store without behavior changes.
type Provider interface {
	// Post creates or updates a memory entry.
	Post(ctx context.Context, m *Memory) error
	// Query searches memories with a free-text query.
	Query(ctx context.Context, q string, limit int) ([]*Memory, error)
	// List returns memories for a specific agent.
	List(ctx context.Context, agentID string, limit int) ([]*Memory, error)
}

// Compile-time checks that all backends satisfy Provider.
var (
	_ Provider = (*Store)(nil)
	_ Provider = (*Mem0)(nil)
)
