package sharedmem

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

// newMem0TestProvider starts an httptest server and returns a Mem0
// provider pointed at it. The configured BaseURL carries a trailing
// slash to exercise URL normalization in NewMem0. Request-side
// assertions run inside the handler via t.Errorf, which is safe for
// concurrent use.
func newMem0TestProvider(t *testing.T, handler http.HandlerFunc) *Mem0 {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewMem0(Mem0Config{BaseURL: srv.URL + "/", APIKey: "sk-test"})
}

func TestMem0ConfigEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Mem0Config
		want bool
	}{
		{"empty", Mem0Config{}, false},
		{"whitespace url", Mem0Config{BaseURL: "   "}, false},
		{"key only", Mem0Config{APIKey: "k"}, false},
		{"url only", Mem0Config{BaseURL: "http://localhost:8888"}, true},
		{"url and key", Mem0Config{BaseURL: "https://api.mem0.ai", APIKey: "k"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.Enabled(); got != tc.want {
				t.Errorf("Enabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMem0Post(t *testing.T) {
	const wantBody = `{"messages":[{"role":"user","content":"use blue-green deploys"}],` +
		`"user_id":"agent-1","metadata":{"key":"deploy","tags":"ops,deploy"}}`
	p := newMem0TestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memories/" {
			t.Errorf("got %s %s, want POST /v1/memories/", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Token sk-test" {
			t.Errorf("Authorization = %q, want %q", got, "Token sk-test")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		body, _ := io.ReadAll(r.Body)
		if got := strings.TrimSpace(string(body)); got != wantBody {
			t.Errorf("body = %s, want %s", got, wantBody)
		}
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, `{"results":[{"id":"m1","event":"ADD"}]}`)
	})

	m := &Memory{AgentID: "agent-1", Key: "deploy", Value: "use blue-green deploys", Tags: "ops,deploy"}
	if err := p.Post(context.Background(), m); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if m.Source != SourceMem0 {
		t.Errorf("Source = %q, want %q", m.Source, SourceMem0)
	}
}

func TestMem0Query(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		wantBody string
		resp     string
		want     []*Memory
	}{
		{
			name:     "wrapped results with metadata",
			limit:    7,
			wantBody: `{"query":"deploy strategy","limit":7}`,
			resp: `{"results":[{"id":"id-1","memory":"remembered text","user_id":"agent-9",` +
				`"metadata":{"key":"k1","tags":"t1,t2"},"created_at":"2026-01-02T03:04:05Z",` +
				`"updated_at":"2026-01-03T04:05:06Z","score":0.42}]}`,
			want: []*Memory{{
				ID: "id-1", AgentID: "agent-9", Key: "k1", Value: "remembered text",
				Tags: "t1,t2", CreatedAt: "2026-01-02T03:04:05Z",
				UpdatedAt: "2026-01-03T04:05:06Z", Source: SourceMem0,
			}},
		},
		{
			name:     "bare array with content field and default limit",
			limit:    0,
			wantBody: `{"query":"deploy strategy","limit":20}`,
			resp:     `[{"id":"id-2","content":"from content","user_id":"a2"}]`,
			want:     []*Memory{{ID: "id-2", AgentID: "a2", Value: "from content", Source: SourceMem0}},
		},
		{
			name:     "memories wrapper, empty",
			limit:    3,
			wantBody: `{"query":"deploy strategy","limit":3}`,
			resp:     `{"memories":[]}`,
			want:     []*Memory{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := newMem0TestProvider(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/memories/search/" {
					t.Errorf("got %s %s, want POST /v1/memories/search/", r.Method, r.URL.Path)
				}
				if got := r.Header.Get("Authorization"); got != "Token sk-test" {
					t.Errorf("Authorization = %q, want %q", got, "Token sk-test")
				}
				body, _ := io.ReadAll(r.Body)
				if got := strings.TrimSpace(string(body)); got != tc.wantBody {
					t.Errorf("body = %s, want %s", got, tc.wantBody)
				}
				fmt.Fprint(w, tc.resp)
			})
			got, err := p.Query(context.Background(), "deploy strategy", tc.limit)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Query = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestMem0List(t *testing.T) {
	resp := `[{"id":"a","memory":"one","user_id":"agent x"},` +
		`{"id":"b","memory":"two","user_id":"agent x"},` +
		`{"id":"c","memory":"three","user_id":"agent x"}]`
	p := newMem0TestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/memories/" {
			t.Errorf("got %s %s, want GET /v1/memories/", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("user_id"); got != "agent x" {
			t.Errorf("user_id = %q, want %q", got, "agent x")
		}
		if got := r.Header.Get("Authorization"); got != "Token sk-test" {
			t.Errorf("Authorization = %q, want %q", got, "Token sk-test")
		}
		fmt.Fprint(w, resp)
	})

	got, err := p.List(context.Background(), "agent x", 2)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// The list endpoint has no portable limit parameter; the limit is
	// applied client-side.
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (client-side truncation)", len(got))
	}
	want := &Memory{ID: "a", AgentID: "agent x", Value: "one", Source: SourceMem0}
	if !reflect.DeepEqual(got[0], want) {
		t.Errorf("got[0] = %+v, want %+v", got[0], want)
	}
}

func TestMem0ListWithoutAgentID(t *testing.T) {
	p := newMem0TestProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.URL.Query()["user_id"]; present {
			t.Errorf("user_id param present, want omitted for empty agentID")
		}
		fmt.Fprint(w, `[]`)
	})
	if _, err := p.List(context.Background(), "", 5); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestMem0NoAuthHeaderWithoutAPIKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.Header["Authorization"]; present {
			t.Errorf("Authorization header present, want omitted when APIKey is empty")
		}
		fmt.Fprint(w, `[]`)
	}))
	t.Cleanup(srv.Close)
	p := NewMem0(Mem0Config{BaseURL: srv.URL})
	if _, err := p.List(context.Background(), "agent-1", 5); err != nil {
		t.Fatalf("List: %v", err)
	}
}

func TestMem0Errors(t *testing.T) {
	t.Run("non-2xx status surfaces", func(t *testing.T) {
		p := newMem0TestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"detail":"bad token"}`, http.StatusUnauthorized)
		})
		ctx := context.Background()
		if err := p.Post(ctx, &Memory{Value: "v"}); err == nil || !strings.Contains(err.Error(), "401") {
			t.Errorf("Post err = %v, want 401 status error", err)
		}
		if _, err := p.Query(ctx, "q", 1); err == nil || !strings.Contains(err.Error(), "401") {
			t.Errorf("Query err = %v, want 401 status error", err)
		}
		if _, err := p.List(ctx, "a", 1); err == nil || !strings.Contains(err.Error(), "401") {
			t.Errorf("List err = %v, want 401 status error", err)
		}
	})
	t.Run("malformed json wrapped with context", func(t *testing.T) {
		p := newMem0TestProvider(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{not json`)
		})
		ctx := context.Background()
		if _, err := p.Query(ctx, "q", 1); err == nil || !strings.Contains(err.Error(), "decode") {
			t.Errorf("Query err = %v, want decode error", err)
		}
		if _, err := p.List(ctx, "a", 1); err == nil || !strings.Contains(err.Error(), "decode") {
			t.Errorf("List err = %v, want decode error", err)
		}
	})
}
