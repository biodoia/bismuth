// Package mcp is the bismuth-team MCP server (stdio JSON-RPC 2.0).
//
// Workers install this MCP and gain team-aware tools. The server is
// self-describing via initialize / tools/list and dispatches tools/call
// to handlers backed by the bismuth SQLite store.
//
// Tools (V1):
//
//   team_status()                    -> { agent_id, role, state, task_id, peers[] }
//   team_peers()                     -> [ { id, role, state, task_title } ]
//   team_post(peer_id, body)         -> { message_id, delivered_at }
//   team_read_inbox(limit)           -> [ messages ]
//   team_claim(task_id)              -> { claimed: true }
//   team_finish(task_id, summary)    -> { finished_at }
//   shared_memory(query, k)          -> [ { source, snippet, score } ]   (V2: Cognee)
package mcp

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/biodoia/bismuth/internal/shared"
)

// Server is the bismuth-team MCP server.
type Server struct {
	db *sql.DB
	in *bufio.Reader
}

// NewServer returns a server reading from os.Stdin and writing to os.Stdout.
func NewServer(db *sql.DB) *Server {
	return &Server{db: db, in: bufio.NewReader(os.Stdin)}
}

// Run starts the JSON-RPC loop. Blocks until stdin closes.
func (s *Server) Run(ctx context.Context) error {
	for {
		// ctx-aware read with a small grace so SIGTERM works cleanly
		line, err := readLineCtx(ctx, s.in)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "parse error: "+err.Error())
			continue
		}
		s.handle(ctx, req)
	}
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) writeError(id json.RawMessage, code int, msg string) {
	out, _ := json.Marshal(rpcResponse{
		JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg},
	})
	fmt.Fprintln(os.Stdout, string(out))
}

func (s *Server) writeResult(id json.RawMessage, result any) {
	out, _ := json.Marshal(rpcResponse{
		JSONRPC: "2.0", ID: id, Result: result,
	})
	fmt.Fprintln(os.Stdout, string(out))
}

func (s *Server) handle(ctx context.Context, req rpcRequest) {
	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, map[string]any{
			"protocolVersion": "2024-11-05",
			"serverInfo":      map[string]any{"name": "bismuth-team", "version": "0.1.0"},
			"capabilities":    map[string]any{"tools": map[string]any{}},
		})
	case "notifications/initialized":
		// no-op, client just confirms init
	case "tools/list":
		s.writeResult(req.ID, map[string]any{
			"tools": toolList(),
		})
	case "tools/call":
		s.dispatchTool(ctx, req.ID, req.Params)
	case "ping":
		s.writeResult(req.ID, map[string]any{"pong": time.Now().UTC()})
	default:
		s.writeError(req.ID, -32601, "method not found: "+req.Method)
	}
}

func toolList() []map[string]any {
	return []map[string]any{
		{
			"name":        "team_status",
			"description": "Report my agent_id, role, state, current task. Use at the start of every turn.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "team_peers",
			"description": "List other agents in the team with their role, state, and current task title.",
			"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			"name":        "team_post",
			"description": "Send a message to a peer agent. Use for cross-agent coordination, review requests, blockers.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"peer_id": map[string]any{"type": "string", "description": "Target agent id (or 'lead' for the orchestrator)"},
					"body":   map[string]any{"type": "string"},
					"kind":   map[string]any{"type": "string", "default": "text"},
				},
				"required": []string{"peer_id", "body"},
			},
		},
		{
			"name":        "team_read_inbox",
			"description": "Read messages addressed to me. Newest first. Optional limit.",
			"inputSchema": map[string]any{
				"type":     "object",
				"properties": map[string]any{"limit": map[string]any{"type": "integer", "default": 50}},
			},
		},
		{
			"name":        "team_claim",
			"description": "Claim a task from the bacheca. Marks it 'in_progress' and sets started_at.",
			"inputSchema": map[string]any{
				"type":     "object",
				"properties": map[string]any{"task_id": map[string]any{"type": "string"}},
				"required": []string{"task_id"},
			},
		},
		{
			"name":        "team_finish",
			"description": "Mark my current task as done with a short summary.",
			"inputSchema": map[string]any{
				"type":     "object",
				"properties": map[string]any{
					"task_id": map[string]any{"type": "string"},
					"summary": map[string]any{"type": "string"},
					"status":  map[string]any{"type": "string", "default": "done"},
				},
				"required": []string{"task_id", "summary"},
			},
		},
		{
			"name":        "shared_memory",
			"description": "Query the shared memory for prior decisions, specs, design notes. (V2: Cognee; V1: simple FTS5 fallback.)",
			"inputSchema": map[string]any{
				"type":     "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"k":     map[string]any{"type": "integer", "default": 5},
				},
				"required": []string{"query"},
			},
		},
	}
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) dispatchTool(ctx context.Context, id json.RawMessage, params json.RawMessage) {
	var p toolCallParams
	if err := json.Unmarshal(params, &p); err != nil {
		s.writeToolError(id, "invalid params: "+err.Error())
		return
	}
	switch p.Name {
	case "team_status":
		s.toolStatus(ctx, id, p.Arguments)
	case "team_peers":
		s.toolPeers(ctx, id, p.Arguments)
	case "team_post":
		s.toolPost(ctx, id, p.Arguments)
	case "team_read_inbox":
		s.toolInbox(ctx, id, p.Arguments)
	case "team_claim":
		s.toolClaim(ctx, id, p.Arguments)
	case "team_finish":
		s.toolFinish(ctx, id, p.Arguments)
	case "shared_memory":
		s.toolMemory(ctx, id, p.Arguments)
	default:
		s.writeToolError(id, "unknown tool: "+p.Name)
	}
}
func (s *Server) toolStatus(ctx context.Context, id json.RawMessage, args json.RawMessage) {
	// we identify the caller by BISMUTH_AGENT_ID env var, set by
	// api.spawnAgent when launching the worker.
	agentID := os.Getenv("BISMUTH_AGENT_ID")
	if agentID == "" {
		s.writeToolError(id, "BISMUTH_AGENT_ID not set in environment")
		return
	}
	peers, _ := s.peersSnapshot(ctx, agentID)
	row := s.db.QueryRowContext(ctx,
		`SELECT task_id, model, state FROM agents WHERE id = ?`, agentID)
	var taskID, model, state sql.NullString
	if err := row.Scan(&taskID, &model, &state); err != nil {
		s.writeToolError(id, "agent not found: "+err.Error())
		return
	}
	s.writeResult(id, map[string]any{
		"agent_id": agentID,
		"role":     os.Getenv("BISMUTH_ROLE"),
		"state":    state.String,
		"task_id":  taskID.String,
		"model":    model.String,
		"peers":    peers,
	})
}

func (s *Server) toolPeers(ctx context.Context, id json.RawMessage, _ json.RawMessage) {
	self := os.Getenv("BISMUTH_AGENT_ID")
	peers, err := s.peersSnapshot(ctx, self)
	if err != nil {
		s.writeToolError(id, err.Error())
		return
	}
	s.writeResult(id, peers)
}

func (s *Server) peersSnapshot(ctx context.Context, self string) ([]map[string]any, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT a.id, a.role, a.state, COALESCE(t.title, '')
		 FROM agents a LEFT JOIN tasks t ON t.id = a.task_id
		 WHERE a.id != ? ORDER BY a.created_at`, self)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []map[string]any
	for rows.Next() {
		var id, role, state, title string
		if err := rows.Scan(&id, &role, &state, &title); err != nil {
			return nil, err
		}
		out = append(out, map[string]any{
			"id":         id,
			"role":       role,
			"state":      state,
			"task_title": title,
		})
	}
	return out, nil
}

type postArgs struct {
	PeerID string `json:"peer_id"`
	Body   string `json:"body"`
	Kind   string `json:"kind"`
}

func (s *Server) toolPost(ctx context.Context, id json.RawMessage, args json.RawMessage) {
	var a postArgs
	if err := json.Unmarshal(args, &a); err != nil {
		s.writeToolError(id, "invalid args: "+err.Error())
		return
	}
	from := os.Getenv("BISMUTH_AGENT_ID")
	if from == "" {
		s.writeToolError(id, "BISMUTH_AGENT_ID not set")
		return
	}
	if a.Kind == "" {
		a.Kind = "text"
	}
	msgID := newULID("msg")
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages(id, from_agent_id, to_agent_id, kind, body, created_at)
		 VALUES(?,?,?,?,?,?)`,
		msgID, from, a.PeerID, a.Kind, a.Body, now)
	if err != nil {
		s.writeToolError(id, "insert: "+err.Error())
		return
	}
	s.writeResult(id, map[string]any{
		"message_id":   msgID,
		"delivered_at": now,
	})
}

type inboxArgs struct {
	Limit int `json:"limit"`
}

func (s *Server) toolInbox(ctx context.Context, id json.RawMessage, args json.RawMessage) {
	var a inboxArgs
	if err := json.Unmarshal(args, &a); err != nil && len(args) > 0 {
		s.writeToolError(id, "invalid args: "+err.Error())
		return
	}
	if a.Limit <= 0 {
		a.Limit = 50
	}
	to := os.Getenv("BISMUTH_AGENT_ID")
	if to == "" {
		s.writeToolError(id, "BISMUTH_AGENT_ID not set")
		return
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, from_agent_id, kind, body, task_id, read_at, created_at
		 FROM messages WHERE to_agent_id = ? ORDER BY created_at DESC LIMIT ?`,
		to, a.Limit)
	if err != nil {
		s.writeToolError(id, err.Error())
		return
	}
	defer rows.Close()
	type msg struct {
		ID        string  `json:"id"`
		From      string  `json:"from"`
		Kind      string  `json:"kind"`
		Body      string  `json:"body"`
		TaskID    *string `json:"task_id,omitempty"`
		ReadAt    *string `json:"read_at,omitempty"`
		CreatedAt string  `json:"created_at"`
	}
	var out []msg
	for rows.Next() {
		var m msg
		var from, kind, body, created string
		var taskID, readAt sql.NullString
		if err := rows.Scan(&m.ID, &from, &kind, &body, &taskID, &readAt, &created); err != nil {
			s.writeToolError(id, err.Error())
			return
		}
		m.From = from
		m.Kind = kind
		m.Body = body
		m.CreatedAt = created
		if taskID.Valid {
			t := taskID.String
			m.TaskID = &t
		}
		if readAt.Valid {
			r := readAt.String
			m.ReadAt = &r
		}
		out = append(out, m)
	}
	s.writeResult(id, out)
}

type claimArgs struct {
	TaskID string `json:"task_id"`
}

func (s *Server) toolClaim(ctx context.Context, id json.RawMessage, args json.RawMessage) {
	var a claimArgs
	if err := json.Unmarshal(args, &a); err != nil {
		s.writeToolError(id, "invalid args: "+err.Error())
		return
	}
	agentID := os.Getenv("BISMUTH_AGENT_ID")
	if agentID == "" {
		s.writeToolError(id, "BISMUTH_AGENT_ID not set")
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET assignee_agent_id=?, status='in_progress',
		 started_at=?, updated_at=? WHERE id=? AND status IN ('open','assigned')`,
		agentID, now, now, a.TaskID)
	if err != nil {
		s.writeToolError(id, err.Error())
		return
	}
	_, _ = s.db.ExecContext(ctx,
		`UPDATE agents SET task_id=?, state='working', updated_at=? WHERE id=?`,
		a.TaskID, now, agentID)
	s.writeResult(id, map[string]any{"claimed": true, "task_id": a.TaskID})
}

type finishArgs struct {
	TaskID string `json:"task_id"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

func (s *Server) toolFinish(ctx context.Context, id json.RawMessage, args json.RawMessage) {
	var a finishArgs
	if err := json.Unmarshal(args, &a); err != nil {
		s.writeToolError(id, "invalid args: "+err.Error())
		return
	}
	if a.Status == "" {
		a.Status = "done"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status=?, finished_at=?, updated_at=?, meta = json_set(COALESCE(meta,'{}'), '$.summary', ?)
		 WHERE id=?`,
		a.Status, now, now, a.Summary, a.TaskID)
	if err != nil {
		s.writeToolError(id, err.Error())
		return
	}
	agentID := os.Getenv("BISMUTH_AGENT_ID")
	if agentID != "" {
		_, _ = s.db.ExecContext(ctx,
			`UPDATE agents SET task_id=NULL, state='idle', updated_at=? WHERE id=?`,
			now, agentID)
	}
	s.writeResult(id, map[string]any{"finished_at": now, "status": a.Status})
}

type memoryArgs struct {
	Query string `json:"query"`
	K     int    `json:"k"`
}

func (s *Server) toolMemory(ctx context.Context, id json.RawMessage, args json.RawMessage) {
	var a memoryArgs
	if err := json.Unmarshal(args, &a); err != nil {
		s.writeToolError(id, "invalid args: "+err.Error())
		return
	}
	if a.K <= 0 {
		a.K = 5
	}
	// V1: simple LIKE search across events.payload + tasks.description
	// V2: Cognee graph + vector
	q := "%" + strings.ReplaceAll(a.Query, `"`, `""`) + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT 'event' AS src, payload, ts FROM events WHERE payload LIKE ? ORDER BY seq DESC LIMIT ?`,
		q, a.K)
	if err != nil {
		s.writeToolError(id, err.Error())
		return
	}
	defer rows.Close()
	type hit struct {
		Source  string `json:"source"`
		Snippet string `json:"snippet"`
		Score   int    `json:"score"`
	}
	var out []hit
	for rows.Next() {
		var src, payload, ts string
		if err := rows.Scan(&src, &payload, &ts); err != nil {
			continue
		}
		// crude snippet: first 200 chars
		snip := payload
		if len(snip) > 200 {
			snip = snip[:200] + "..."
		}
		// crude score: substring matches
		score := strings.Count(strings.ToLower(payload), strings.ToLower(a.Query))
		out = append(out, hit{Source: src, Snippet: snip, Score: score})
	}
	s.writeResult(id, out)
}

// ----------------- helpers ------------------------------------------------

func (s *Server) writeToolError(id json.RawMessage, msg string) {
	s.writeResult(id, map[string]any{
		"content": []map[string]any{{
			"type": "text",
			"text": "error: " + msg,
		}},
		"isError": true,
	})
}

func newULID(prefix string) string {
	// Simple ULID-like: 16 random bytes hex-encoded, with timestamp prefix
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + strconv.FormatInt(time.Now().UnixMilli(), 36) + "-" + hex.EncodeToString(b)
}

func readLineCtx(ctx context.Context, r *bufio.Reader) ([]byte, error) {
	type result struct {
		line []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		line, err := r.ReadBytes('\n')
		ch <- result{line, err}
	}()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case r := <-ch:
		// strip trailing \n
		line := r.line
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		return line, r.err
	}
}

// Marshal helper kept for callers.
var _ = shared.JSONRaw
