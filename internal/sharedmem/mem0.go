package sharedmem

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// mem0Timeout bounds every HTTP request to the Mem0 server.
const mem0Timeout = 10 * time.Second

// Mem0Config configures the Mem0 HTTP provider.
type Mem0Config struct {
	// BaseURL is the root of the Mem0 REST API without the /v1 suffix,
	// e.g. "https://api.mem0.ai" or a self-hosted server URL.
	BaseURL string
	// APIKey, when non-empty, is sent as "Authorization: Token <APIKey>".
	APIKey string
}

// Enabled reports whether the configuration points at a Mem0 server.
func (c Mem0Config) Enabled() bool {
	return strings.TrimSpace(c.BaseURL) != ""
}

// Mem0 is a Provider backed by the Mem0 REST API (https://docs.mem0.ai).
//
// Mapping between Memory and the Mem0 wire format:
//   - Post   -> POST {BaseURL}/v1/memories/ with
//     {messages:[{role:"user",content:Value}], user_id:AgentID,
//     metadata:{key:Key, tags:Tags}}
//   - Query  -> POST {BaseURL}/v1/memories/search/ with {query, limit}
//   - List   -> GET  {BaseURL}/v1/memories/?user_id=AgentID
//
// Responses are decoded from either a bare JSON array or an object
// wrapping the items under "results" or "memories", which covers the
// shapes used across Mem0 API versions.
type Mem0 struct {
	cfg    Mem0Config
	client *http.Client
}

// NewMem0 creates a Mem0 provider with a ~10s request timeout.
func NewMem0(cfg Mem0Config) *Mem0 {
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	return &Mem0{
		cfg:    cfg,
		client: &http.Client{Timeout: mem0Timeout},
	}
}

// mem0Message is a single conversation message in an add request.
type mem0Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// mem0Metadata carries the Memory fields that Mem0 has no native slot for.
type mem0Metadata struct {
	Key  string `json:"key"`
	Tags string `json:"tags"`
}

// mem0AddRequest is the body of POST /v1/memories/.
type mem0AddRequest struct {
	Messages []mem0Message `json:"messages"`
	UserID   string        `json:"user_id,omitempty"`
	Metadata mem0Metadata  `json:"metadata"`
}

// mem0SearchRequest is the body of POST /v1/memories/search/.
type mem0SearchRequest struct {
	Query  string `json:"query"`
	UserID string `json:"user_id,omitempty"`
	Limit  int    `json:"limit"`
}

// mem0Item is the wire shape of a memory in Mem0 API responses.
type mem0Item struct {
	ID        string         `json:"id"`
	Memory    string         `json:"memory"`
	Content   string         `json:"content"`
	UserID    string         `json:"user_id"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
}

// toMemory maps a Mem0 wire item into a Memory with Source "mem0".
func (it mem0Item) toMemory() *Memory {
	value := it.Memory
	if value == "" {
		value = it.Content
	}
	m := &Memory{
		ID:        it.ID,
		AgentID:   it.UserID,
		Value:     value,
		CreatedAt: it.CreatedAt,
		UpdatedAt: it.UpdatedAt,
		Source:    SourceMem0,
	}
	if k, ok := it.Metadata["key"].(string); ok {
		m.Key = k
	}
	if t, ok := it.Metadata["tags"].(string); ok {
		m.Tags = t
	}
	return m
}

// Post stores a memory in Mem0 via POST /v1/memories/.
func (p *Mem0) Post(ctx context.Context, m *Memory) error {
	body := mem0AddRequest{
		Messages: []mem0Message{{Role: "user", Content: m.Value}},
		UserID:   m.AgentID,
		Metadata: mem0Metadata{Key: m.Key, Tags: m.Tags},
	}
	resp, err := p.do(ctx, http.MethodPost, "/v1/memories/", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := mem0Status(resp); err != nil {
		return fmt.Errorf("mem0 post: %w", err)
	}
	// Drain the body so the connection can be reused; the add response
	// payload (event list) carries nothing we need to surface.
	_, _ = io.Copy(io.Discard, resp.Body)
	if m.Source == "" {
		m.Source = SourceMem0
	}
	return nil
}

// Query searches Mem0 via POST /v1/memories/search/.
func (p *Mem0) Query(ctx context.Context, q string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 20
	}
	body := mem0SearchRequest{Query: q, Limit: limit}
	resp, err := p.do(ctx, http.MethodPost, "/v1/memories/search/", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := mem0Status(resp); err != nil {
		return nil, fmt.Errorf("mem0 search: %w", err)
	}
	items, err := decodeMem0Items(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mem0 search: decode response: %w", err)
	}
	return mem0ToMemories(items, limit), nil
}

// List returns memories for one agent via GET /v1/memories/?user_id=...
// The Mem0 list endpoint has no portable limit parameter, so the limit
// is applied client-side.
func (p *Mem0) List(ctx context.Context, agentID string, limit int) ([]*Memory, error) {
	if limit <= 0 {
		limit = 50
	}
	path := "/v1/memories/"
	if agentID != "" {
		path += "?user_id=" + url.QueryEscape(agentID)
	}
	resp, err := p.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := mem0Status(resp); err != nil {
		return nil, fmt.Errorf("mem0 list: %w", err)
	}
	items, err := decodeMem0Items(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("mem0 list: decode response: %w", err)
	}
	return mem0ToMemories(items, limit), nil
}

// do sends one JSON request to the Mem0 server with auth headers set.
func (p *Mem0) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("mem0: encode %s %s: %w", method, path, err)
		}
		rdr = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.cfg.BaseURL+path, rdr)
	if err != nil {
		return nil, fmt.Errorf("mem0: build %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Token "+p.cfg.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("mem0: %s %s: %w", method, path, err)
	}
	return resp, nil
}

// mem0Status returns an error describing a non-2xx response,
// including a snippet of the body for diagnostics.
func mem0Status(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(snippet)))
}

// decodeMem0Items accepts the response shapes used across Mem0 versions:
// a bare JSON array of items, or an object wrapping the array under
// "results" (search, v1.1 outputs) or "memories".
func decodeMem0Items(r io.Reader) ([]mem0Item, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var items []mem0Item
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, err
		}
		return items, nil
	}
	var wrapped struct {
		Results  []mem0Item `json:"results"`
		Memories []mem0Item `json:"memories"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return nil, err
	}
	if wrapped.Results != nil {
		return wrapped.Results, nil
	}
	return wrapped.Memories, nil
}

// mem0ToMemories maps wire items into Memories, truncating to limit.
func mem0ToMemories(items []mem0Item, limit int) []*Memory {
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	out := make([]*Memory, 0, len(items))
	for _, it := range items {
		out = append(out, it.toMemory())
	}
	return out
}
