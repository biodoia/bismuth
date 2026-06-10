package sharedmem

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

// Compile-time Provider assertions for every backend.
var (
	_ Provider = (*Store)(nil)
	_ Provider = (*Mem0)(nil)
	_ Provider = (*fallbackProvider)(nil)
)

// fakeProvider is a scriptable Provider for fallback tests.
type fakeProvider struct {
	err     error     // returned by every method when non-nil
	results []*Memory // returned by Query/List on success
	posts   []*Memory // record of Post calls
	queries int
	lists   int
}

func (f *fakeProvider) Post(_ context.Context, m *Memory) error {
	f.posts = append(f.posts, m)
	return f.err
}

func (f *fakeProvider) Query(_ context.Context, _ string, _ int) ([]*Memory, error) {
	f.queries++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func (f *fakeProvider) List(_ context.Context, _ string, _ int) ([]*Memory, error) {
	f.lists++
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

func TestFallbackPost(t *testing.T) {
	errPrimary := errors.New("primary down")
	errFallback := errors.New("fallback down")
	tests := []struct {
		name    string
		perr    error
		ferr    error
		wantErr error
	}{
		{"both ok", nil, nil, nil},
		{"primary error wins", errPrimary, nil, errPrimary},
		{"fallback error is best-effort", nil, errFallback, nil},
		{"both fail returns primary", errPrimary, errFallback, errPrimary},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			primary := &fakeProvider{err: tc.perr}
			fallback := &fakeProvider{err: tc.ferr}
			p := NewFallback(primary, fallback)

			err := p.Post(context.Background(), &Memory{Key: "k", Value: "v"})
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Post err = %v, want %v", err, tc.wantErr)
			}
			// Post always writes to BOTH providers, regardless of errors.
			if len(primary.posts) != 1 || len(fallback.posts) != 1 {
				t.Errorf("posts: primary=%d fallback=%d, want 1 and 1",
					len(primary.posts), len(fallback.posts))
			}
		})
	}
}

func TestFallbackRead(t *testing.T) {
	primaryRes := []*Memory{{ID: "p1", Source: SourceMem0}}
	fallbackRes := []*Memory{{ID: "f1", Source: SourceFTS5}}
	errPrimary := errors.New("primary down")
	errFallback := errors.New("fallback down")

	ops := []struct {
		name string
		call func(p Provider) ([]*Memory, error)
	}{
		{"Query", func(p Provider) ([]*Memory, error) { return p.Query(context.Background(), "q", 5) }},
		{"List", func(p Provider) ([]*Memory, error) { return p.List(context.Background(), "agent", 5) }},
	}
	for _, op := range ops {
		t.Run(op.name, func(t *testing.T) {
			t.Run("primary ok serves primary", func(t *testing.T) {
				primary := &fakeProvider{results: primaryRes}
				fallback := &fakeProvider{results: fallbackRes}
				got, err := op.call(NewFallback(primary, fallback))
				if err != nil {
					t.Fatalf("%s: %v", op.name, err)
				}
				if !reflect.DeepEqual(got, primaryRes) {
					t.Errorf("%s = %+v, want primary results", op.name, got)
				}
				if fallback.queries+fallback.lists != 0 {
					t.Errorf("fallback consulted although primary succeeded")
				}
			})
			t.Run("primary error serves fallback", func(t *testing.T) {
				primary := &fakeProvider{err: errPrimary}
				fallback := &fakeProvider{results: fallbackRes}
				got, err := op.call(NewFallback(primary, fallback))
				if err != nil {
					t.Fatalf("%s: %v", op.name, err)
				}
				if !reflect.DeepEqual(got, fallbackRes) {
					t.Errorf("%s = %+v, want fallback results", op.name, got)
				}
			})
			t.Run("both error joins both", func(t *testing.T) {
				primary := &fakeProvider{err: errPrimary}
				fallback := &fakeProvider{err: errFallback}
				_, err := op.call(NewFallback(primary, fallback))
				if !errors.Is(err, errPrimary) || !errors.Is(err, errFallback) {
					t.Errorf("%s err = %v, want both primary and fallback errors", op.name, err)
				}
			})
		})
	}
}

// newTestStore opens a throwaway SQLite database and migrates the schema.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "mem.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := New(db)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// TestStoreSourceFTS5 verifies that Store results carry Source "fts5"
// even though the column is not persisted in the database.
func TestStoreSourceFTS5(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m := &Memory{AgentID: "agent-1", Key: "deploy", Value: "use canary releases", Tags: "ops"}
	if err := s.Post(ctx, m); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if m.Source != SourceFTS5 {
		t.Errorf("posted Source = %q, want %q", m.Source, SourceFTS5)
	}

	queried, err := s.Query(ctx, "canary", 10)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(queried) != 1 {
		t.Fatalf("Query returned %d results, want 1", len(queried))
	}
	if queried[0].Source != SourceFTS5 || queried[0].Value != "use canary releases" {
		t.Errorf("Query result = %+v, want Source %q and original value", queried[0], SourceFTS5)
	}

	listed, err := s.List(ctx, "agent-1", 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 || listed[0].Source != SourceFTS5 {
		t.Errorf("List = %+v, want one result with Source %q", listed, SourceFTS5)
	}
}
