// Package mcp is the bismuth-team MCP server (stdio transport).
//
// Worker agents (omx, omc, omo, omp, claude, codex) install this MCP
// server and gain access to a "team awareness" tool set. This is what
// makes the swarm feel like a team: every agent knows who else is
// working on what, can post messages to peers, claim/release tasks
// from the bacheca, and query shared memory.
//
// Tools exposed (V1):
//
//   team_status()                    -> { agent_id, role, state, task_id, peers[] }
//   team_peers()                     -> [ { id, role, state, task_title } ]
//   team_post(peer_id, body)         -> { message_id, delivered_at }
//   team_read_inbox(limit)           -> [ messages ]
//   team_claim(task_id)              -> { claimed: true }
//   team_finish(task_id, summary)    -> { finished_at }
//   shared_memory(query, k)          -> [ { source, snippet, score } ]
//
// Wire: stdio JSON-RPC 2.0 (standard MCP).
// See https://modelcontextprotocol.io for the protocol spec.
package mcp

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"

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
		line, err := s.in.ReadBytes('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.writeError(nil, -32700, "parse error")
			continue
		}
		go s.handle(ctx, req)
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
	case "tools/list":
		s.writeResult(req.ID, map[string]any{
			"tools": []map[string]any{
				{"name": "team_status", "description": "Report my agent_id, role, state, current task."},
				{"name": "team_peers", "description": "List other agents in the team with their state."},
				{"name": "team_post", "description": "Send a message to a peer."},
				{"name": "team_read_inbox", "description": "Read messages addressed to me."},
				{"name": "team_claim", "description": "Claim a task from the bacheca."},
				{"name": "team_finish", "description": "Mark my current task as done with a summary."},
				{"name": "shared_memory", "description": "Query the shared memory (Cognee)."},
			},
		})
	case "tools/call":
		// TODO(sessione+1): dispatch to tool implementations.
		s.writeResult(req.ID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "tool call stub: " + string(req.Params)}},
		})
	default:
		s.writeError(req.ID, -32601, "method not found: "+req.Method)
	}
}

// Marshal helper to keep call sites short.
var _ = shared.JSONRaw
