// Package api is the HTTP/WebSocket server for bismuth.
//
// Routes (V1):
//
//	GET  /healthz                                  -- liveness
//
//	GET  /api/v1/agents                            -- list agents
//	POST /api/v1/agents                            -- spawn agent { role, cli, task, args }
//	GET  /api/v1/agents/:id                        -- agent detail
//	POST /api/v1/agents/:id/send                   -- send bytes { data_b64 }
//	GET  /api/v1/agents/:id/read                   -- read last N lines ?n=200
//	POST /api/v1/agents/:id/kill                   -- terminate
//
//	GET  /api/v1/tasks                             -- list tasks
//	POST /api/v1/tasks                             -- create task { title, description, priority, parent_id }
//	GET  /api/v1/tasks/:id                         -- task detail
//	POST /api/v1/tasks/:id/assign                  -- assign { agent_id }
//	POST /api/v1/tasks/:id/merge                   -- merge branch
//
//	GET  /api/v1/roles                             -- role catalog
//	GET  /api/v1/events                            -- recent events ?types=&agent_id=&limit=
//
//	GET  /api/v1/ws                                -- WebSocket subscribe ?types=&agent_id=
//
//	POST /v1/voice/stt                             -- multipart audio -> { text }
//	POST /v1/voice/speak                           -- { text } -> { audio_b64, format }
//	POST /v1/voice/command                         -- { text } -> { action, args, text_response }
package api

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/biodoia/bismuth/internal/audit"
	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/db"
	"github.com/biodoia/bismuth/internal/logger"
	"github.com/biodoia/bismuth/internal/metrics"
	"github.com/biodoia/bismuth/internal/pane"
	"github.com/biodoia/bismuth/internal/roles"
	"github.com/biodoia/bismuth/internal/security"
	"github.com/biodoia/bismuth/internal/shared"
	"github.com/biodoia/bismuth/internal/sharedmem"
	"github.com/biodoia/bismuth/internal/voice"
	"github.com/biodoia/bismuth/internal/worktree"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server bundles the dependencies.
type Server struct {
	cfg    *config.Config
	apicfg config.APICfg
	store  *db.Store
	bus    *bus.Bus
	pane   *pane.Manager
	voice  *voice.Gateway
	audit  *audit.Log
	sec    *security.Policy
	mem    *sharedmem.Store

	repoRoot string
	catalog  roles.Catalog

	upgrader websocket.Upgrader
	seq      atomic.Int64
}

// NewServer wires the HTTP routes.
func NewServer(
	cfg *config.Config,
	store *db.Store,
	b *bus.Bus,
	pm *pane.Manager,
	v *voice.Gateway,
	a *audit.Log,
	sec *security.Policy,
	c roles.Catalog,
	repoRoot string,
) *Server {
	s := &Server{
		cfg:      cfg,
		apicfg:   cfg.API,
		store:    store,
		bus:      b,
		pane:     pm,
		voice:    v,
		audit:    a,
		sec:      sec,
		repoRoot: repoRoot,
		catalog:  c,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
	}
	// Initialize shared memory store (nil-safe, 503 if FTS5 unavailable)
	s.mem, _ = sharedmem.New(store.DB())
	return s
}

// Run starts the HTTP server until ctx is cancelled.
// Handler returns the fully-wired HTTP handler (for testing with httptest).
func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()

	r.Use(s.authMiddleware)
	r.Use(loggingMiddleware)
	r.Use(metricsMiddleware)

	// Prometheus metrics endpoint
	r.Handle("/metrics", promhttp.Handler())

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().UTC()})
	})

	// agents
	r.Route("/api/v1/agents", func(r chi.Router) {
		r.Get("/", s.listAgents)
		r.Post("/", s.spawnAgent)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.getAgent)
			r.Post("/send", s.sendToAgent)
			r.Get("/read", s.readAgent)
			r.Post("/kill", s.killAgent)
		})
	})

	// tasks
	r.Route("/api/v1/tasks", func(r chi.Router) {
		r.Get("/", s.listTasks)
		r.Post("/", s.createTask)
		r.Route("/{id}", func(r chi.Router) {
			r.Get("/", s.getTask)
			r.Post("/assign", s.assignTask)
			r.Post("/merge", s.mergeTask)
		})
	})

	r.Get("/api/v1/roles", s.listRoles)
	r.Get("/api/v1/events", s.recentEvents)
	r.Get("/api/v1/ws", s.wsSubscribe)

	// voice (V1 HTTP)
	r.Post("/v1/voice/stt", s.voiceSTT)
	r.Post("/v1/voice/speak", s.voiceSpeak)
	r.Post("/v1/voice/command", s.voiceCommand)

	// voice rooms (LiveKit stub)
	r.Post("/api/v1/voice/rooms", s.createVoiceRoom)
	r.Get("/api/v1/voice/rooms", s.listVoiceRooms)
	r.Post("/api/v1/voice/rooms/end", s.endVoiceRoom)

	// shared memory
	r.Post("/api/v1/memory", s.postMemory)
	r.Get("/api/v1/memory", s.queryMemory)

	return r
}

// Run starts the HTTP server on the configured port.
func (s *Server) Run(ctx context.Context) error {
	addr := ":9000"
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	logger.Info("bismuth listening", "addr", addr)

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()
	return srv.ListenAndServe()
}

// ----------------- middleware ---------------------------------------------

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Tailscale user headers (set by aigoproxy/tsnet)
		if email := r.Header.Get("Tailscale-User-Login"); email != "" {
			name := r.Header.Get("Tailscale-User-Name")
			if u := security.UserFromHeaders(email, name); u != nil {
				r = r.WithContext(security.ContextWithUser(r.Context(), u))
			}
		}

		if !s.apicfg.TailscaleOnly {
			next.ServeHTTP(w, r)
			return
		}
		// Tailscale CGNAT range: 100.64.0.0/10
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		ip := net.ParseIP(host)
		allowed := ip != nil && (ip.IsLoopback() || isTailscale(ip))
		for _, c := range s.apicfg.AllowedCIDRs {
			if _, n, _ := net.ParseCIDR(c); n != nil && n.Contains(ip) {
				allowed = true
			}
		}
		if !allowed {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isTailscale(ip net.IP) bool {
	_, cgnat, _ := net.ParseCIDR("100.64.0.0/10")
	return cgnat != nil && cgnat.Contains(ip)
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		logger.Info("http", "method", r.Method, "path", r.URL.Path, "status", ww.status, "dur_ms", time.Since(start).Milliseconds())
	})
}

// metricsMiddleware instruments every request with Prometheus counters and histograms.
func metricsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		elapsed := time.Since(start).Seconds()
		path := normalizePath(r.URL.Path)
		status := fmt.Sprintf("%d", ww.status)
		metrics.APIRequests.WithLabelValues(r.Method, path, status).Inc()
		metrics.APILatency.WithLabelValues(r.Method, path).Observe(elapsed)
	})
}

// normalizePath collapses parameterized routes for label cardinality.
func normalizePath(p string) string {
	// Collapse /api/v1/agents/{id}/... variants
	parts := strings.Split(p, "/")
	for i, part := range parts {
		if len(part) > 20 && strings.Contains(part, "-") {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(c int) {
	s.status = c
	s.ResponseWriter.WriteHeader(c)
}

// ----------------- helpers ------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(v)
	if err != nil {
		logger.Error("writeJSON marshal error", "err", err)
		w.WriteHeader(500)
		w.Write([]byte(`{"error":"marshal failed"}`))
		return
	}
	w.WriteHeader(code)
	w.Write(b)
	w.Write([]byte("\n"))
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

// ----------------- agent handlers ----------------------------------------

func (s *Server) listAgents(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	agents, err := s.store.ListAgents(r.Context(), state)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"agents": agents})
}

func (s *Server) getAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.store.GetAgent(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, a)
}

type spawnReq struct {
	Role string   `json:"role"`
	CLI  string   `json:"cli"`
	Task string   `json:"task"`
	Args []string `json:"args"`
}

func (s *Server) spawnAgent(w http.ResponseWriter, r *http.Request) {
	// RBAC (P7-g): authenticated users need spawn rights. Requests with
	// no user (localhost CLI, no Tailscale headers) pass — the network
	// gate in authMiddleware is the V1 boundary for those.
	if u := security.UserFromContext(r.Context()); u != nil && !u.CanSpawn() {
		_ = s.audit.Append(r.Context(), "user:"+u.Email, "denied_spawn", "", nil)
		writeErr(w, 403, errors.New("role "+u.Role+" cannot spawn agents"))
		return
	}
	var req spawnReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.Role == "" || req.CLI == "" {
		writeErr(w, 400, errors.New("--role and --cli required"))
		return
	}
	role, ok := s.findRole(req.Role)
	if !ok {
		writeErr(w, 400, fmt.Errorf("unknown role %q", req.Role))
		return
	}

	agentID := newID("agt")
	paneID := newID("pane")
	taskID := newID("tsk")

	// worktree
	var wtPath, branch string
	if s.sec.AllowsCommand("git") {
		wt := worktree.New(s.repoRoot)
		wtPath, branch, _ = wt.Create(r.Context(), taskID, "main")
	}

	// persist task first (so agent can reference it)
	if req.Task != "" {
		_ = s.store.InsertTask(r.Context(), &db.Task{
			ID:           taskID,
			Title:        firstLine(req.Task),
			Description:  sql.NullString{String: req.Task, Valid: true},
			Status:       "open",
			Priority:     0,
			Branch:       sql.NullString{String: branch, Valid: branch != ""},
			WorktreePath: sql.NullString{String: wtPath, Valid: wtPath != ""},
		})
	}

	// persist agent
	name := req.CLI + "-" + shortID()
	_ = s.store.InsertAgent(r.Context(), &db.Agent{
		ID:           agentID,
		Role:         role.ID,
		Name:         name,
		CLI:          req.CLI,
		State:        "idle",
		PaneID:       sql.NullString{String: paneID, Valid: true},
		WorktreePath: sql.NullString{String: wtPath, Valid: wtPath != ""},
		Branch:       sql.NullString{String: branch, Valid: branch != ""},
		Model:        sql.NullString{String: role.DefaultModel, Valid: role.DefaultModel != ""},
		TaskID:       sql.NullString{String: taskID, Valid: true},
	})

	// spawn pane
	wtDir := wtPath
	if wtDir == "" {
		wtDir = s.repoRoot
	}
	cmd := []string{req.CLI}
	cmd = append(cmd, req.Args...)
	// Build env vars: BISMUTH_* + provider API keys from cli_env config
	envVars := []string{
		"BISMUTH_AGENT_ID=" + agentID,
		"BISMUTH_ROLE=" + role.ID,
		"BISMUTH_TASK_ID=" + taskID,
		"BISMUTH_PANE_ID=" + paneID,
		"BISMUTH_MODEL=" + role.DefaultModel,
		"BISMUTH_WORKDIR=" + wtDir,
	}
	// Inject provider API keys (OPENAI_API_KEY, ANTHROPIC_API_KEY, etc.)
	if providerEnv := s.cfg.EnvForCLI(req.CLI); len(providerEnv) > 0 {
		envVars = append(envVars, providerEnv...)
	}
	// PaneID must match the id stored on the agent so /read, /send and
	// /kill resolve the same pane. The worker runs in its worktree,
	// receives the task as its first prompt, and gets the bismuth-team
	// MCP server installed via .mcp.json.
	if _, err := s.pane.Spawn(r.Context(), pane.SpawnSpec{
		AgentID:      agentID,
		PaneID:       paneID,
		CLI:          req.CLI,
		Role:         role.ID,
		Workdir:      wtDir,
		Cmd:          cmd,
		Env:          envVars,
		InitialInput: []byte(req.Task),
		MCPConfig:    s.mcpConfigJSON(agentID),
	}); err != nil {
		logger.Error("spawn worker failed", "agent_id", agentID, "cli", req.CLI, "err", err)
		// Use a fresh context: if the spawn failed because the request was
		// cancelled, r.Context() is dead and the state update would be lost.
		_ = s.store.UpdateAgentState(context.Background(), agentID, "failed")
		writeErr(w, 500, fmt.Errorf("spawn worker: %w", err))
		return
	}

	_ = s.audit.Append(r.Context(), "user:lisergico25", "spawn_agent", agentID,
		shared.JSONRaw(map[string]any{"role": role.ID, "cli": req.CLI, "task": taskID}))

	_ = s.bus.Publish(r.Context(), bus.Event{
		Type:    "agent_spawned",
		AgentID: agentID,
		TaskID:  taskID,
		Payload: shared.JSONRaw(map[string]any{"role": role.ID, "cli": req.CLI, "pane_id": paneID}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	metrics.IncAgentSpawned(role.ID, req.CLI)
	metrics.IncEvent("agent_spawned")
	logger.Info("agent spawned",
		"agent_id", agentID, "role", role.ID, "cli", req.CLI,
		"task_id", taskID, "worktree", wtPath, "branch", branch,
	)
	writeJSON(w, 201, map[string]any{
		"agent_id":      agentID,
		"pane_id":       paneID,
		"task_id":       taskID,
		"role":          role.ID,
		"worktree_path": wtPath,
		"branch":        branch,
	})
}

type sendReq struct {
	DataB64 string `json:"data_b64"`
}

func (s *Server) sendToAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Driving a worker's stdin is an operator action, same as spawning.
	if u := security.UserFromContext(r.Context()); u != nil && !u.CanSpawn() {
		_ = s.audit.Append(r.Context(), "user:"+u.Email, "denied_send", id, nil)
		writeErr(w, 403, errors.New("role "+u.Role+" cannot send to agents"))
		return
	}
	var req sendReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	b, err := base64.StdEncoding.DecodeString(req.DataB64)
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	// policy check on the first word
	if !s.sec.AllowsCommand(string(b)) {
		writeErr(w, 403, fmt.Errorf("command not allowed: %q", firstWord(string(b))))
		_ = s.audit.Append(r.Context(), "user:lisergico25", "blocked_send", id, shared.JSONRaw(map[string]any{"reason": "policy"}))
		return
	}
	a, err := s.store.GetAgent(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	if !a.PaneID.Valid {
		writeErr(w, 409, errors.New("agent has no pane"))
		return
	}
	if err := s.pane.Send(r.Context(), a.PaneID.String, b); err != nil {
		writeErr(w, 500, err)
		return
	}
	_ = s.bus.Publish(r.Context(), bus.Event{
		Type:    "pane_input",
		AgentID: id,
		Payload: shared.JSONRaw(map[string]any{"bytes": len(b)}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) readAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a, err := s.store.GetAgent(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	n := 200
	if q := r.URL.Query().Get("n"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			n = v
		}
	}
	// try persisted scrollback first
	if a.PaneID.Valid {
		p, err := s.store.GetPane(r.Context(), a.PaneID.String)
		if err == nil && p.Scrollback.Valid && p.Scrollback.String != "" {
			writeJSON(w, 200, map[string]any{
				"agent_id":   id,
				"pane_id":    a.PaneID.String,
				"scrollback": string(pane.LastLines([]byte(p.Scrollback.String), n)),
				"last_state": p.LastState.String,
			})
			return
		}
	}
	// empty pane (no scrollback yet, or no pane)
	writeJSON(w, 200, map[string]any{
		"agent_id":   id,
		"pane_id":    a.PaneID.String,
		"scrollback": "",
		"last_state": "",
	})
}

func (s *Server) killAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if u := security.UserFromContext(r.Context()); u != nil && !u.CanKill() {
		_ = s.audit.Append(r.Context(), "user:"+u.Email, "denied_kill", id, nil)
		writeErr(w, 403, errors.New("role "+u.Role+" cannot kill agents"))
		return
	}
	a, err := s.store.GetAgent(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	if a.PaneID.Valid {
		_ = s.pane.Kill(r.Context(), a.PaneID.String)
	}
	_ = s.store.UpdateAgentState(r.Context(), id, "killed")
	_ = s.audit.Append(r.Context(), "user:lisergico25", "kill_agent", id, nil)
	_ = s.bus.Publish(r.Context(), bus.Event{
		Type:    "agent_killed",
		AgentID: id,
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	metrics.IncAgentKilled(a.Role)
	metrics.IncEvent("agent_killed")
	writeJSON(w, 200, map[string]any{"ok": true})
}

// ----------------- task handlers -----------------------------------------

func (s *Server) listTasks(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	tasks, err := s.store.ListTasks(r.Context(), status)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"tasks": tasks})
}

func (s *Server) getTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := s.store.GetTask(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	writeJSON(w, 200, t)
}

type createTaskReq struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    int    `json:"priority"`
	ParentID    string `json:"parent_id"`
}

func (s *Server) createTask(w http.ResponseWriter, r *http.Request) {
	var req createTaskReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.Title == "" {
		writeErr(w, 400, errors.New("title required"))
		return
	}
	t := &db.Task{
		ID:          newID("tsk"),
		Title:       req.Title,
		Description: sql.NullString{String: req.Description, Valid: req.Description != ""},
		Status:      "open",
		Priority:    req.Priority,
		ParentID:    sql.NullString{String: req.ParentID, Valid: req.ParentID != ""},
	}
	if err := s.store.InsertTask(r.Context(), t); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 201, t)
}

type assignReq struct {
	AgentID string `json:"agent_id"`
}

func (s *Server) assignTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req assignReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if err := s.store.AssignTask(r.Context(), id, req.AgentID); err != nil {
		writeErr(w, 500, err)
		return
	}
	_ = s.bus.Publish(r.Context(), bus.Event{
		Type:    "task_assigned",
		TaskID:  id,
		AgentID: req.AgentID,
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) mergeTask(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	t, err := s.store.GetTask(r.Context(), id)
	if err != nil {
		writeErr(w, 404, err)
		return
	}
	if !t.Branch.Valid || t.Branch.String == "" {
		writeErr(w, 409, errors.New("task has no branch to merge"))
		return
	}
	if s.sec.RequiresHumanApproval("git_push") {
		_ = s.bus.Publish(r.Context(), bus.Event{
			Type:   "human_approval_required",
			TaskID: id,
			Payload: shared.JSONRaw(map[string]any{
				"action": "merge",
				"branch": t.Branch.String,
			}),
			TS: time.Now().UTC().Format(time.RFC3339),
		})
		writeJSON(w, 202, map[string]any{"ok": "approval_required", "branch": t.Branch.String})
		return
	}
	// best-effort push
	if t.WorktreePath.Valid {
		wt := worktree.New(s.WorktreeRoot(t))
		_ = wt.Push(r.Context(), t.WorktreePath.String, t.Branch.String)
	}
	_ = s.store.MarkTaskFinished(r.Context(), id, "merged")
	writeJSON(w, 200, map[string]any{"ok": true})
}

// WorktreeRoot returns the repo root (helper for worktree ops).
func (s *Server) WorktreeRoot(t *db.Task) string {
	_ = t
	return s.repoRoot
}

// ----------------- misc handlers -----------------------------------------

func (s *Server) listRoles(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"roles": s.catalog.Roles})
}

func (s *Server) recentEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typesRaw := q.Get("types")
	var types []string
	if typesRaw != "" {
		for _, t := range strings.Split(typesRaw, ",") {
			types = append(types, strings.TrimSpace(t))
		}
	}
	agentID := q.Get("agent_id")
	limit := 200
	if l := q.Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	events, err := s.store.RecentEvents(r.Context(), types, agentID, limit)
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"events": events})
}

func (s *Server) wsSubscribe(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	typesRaw := q.Get("types")
	var types []string
	if typesRaw != "" {
		for _, t := range strings.Split(typesRaw, ",") {
			types = append(types, strings.TrimSpace(t))
		}
	}
	filter := bus.Filter{Types: types, AgentID: q.Get("agent_id")}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	// Set sensible connection limits
	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer conn.Close()

	// Replay recent events so client catches up
	recent, _ := s.store.RecentEvents(r.Context(), types, filter.AgentID, 50)
	for _, e := range recent {
		b, _ := json.Marshal(e)
		_ = conn.WriteMessage(websocket.TextMessage, b)
	}

	ch := s.bus.Subscribe(ctx, conn, filter)

	// write pump: events from bus + ping ticker
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					cancel()
					return
				}
			case evt, ok := <-ch:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				b, _ := json.Marshal(evt)
				if err := conn.WriteMessage(websocket.TextMessage, b); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// read pump: keep alive + detect close
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// ----------------- voice handlers ----------------------------------------

func (s *Server) voiceSTT(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeErr(w, 400, err)
		return
	}
	f, header, err := r.FormFile("file")
	if err != nil {
		writeErr(w, 400, err)
		return
	}
	defer f.Close()
	audio, _ := io.ReadAll(f)
	lang := r.FormValue("lang")
	if lang == "" {
		lang = "it"
	}
	text, err := s.voice.Transcribe(r.Context(), audio, lang)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	_ = s.audit.Append(r.Context(), "user:lisergico25", "voice_stt", "", shared.JSONRaw(map[string]any{"bytes": header.Size, "lang": lang, "text": text}))
	writeJSON(w, 200, map[string]any{"text": text})
}

type speakReq struct {
	Text string `json:"text"`
}

func (s *Server) voiceSpeak(w http.ResponseWriter, r *http.Request) {
	var req speakReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	audio, err := s.voice.Speak(r.Context(), req.Text)
	if err != nil {
		writeErr(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"audio_b64": base64.StdEncoding.EncodeToString(audio),
		"format":    "mp3",
	})
}

type voiceCommandReq struct {
	Text string `json:"text"`
}

// voiceCommand parses the transcribed text and dispatches a bismuth
// action. V1: simple keyword match. V2: use Hermes/LLM to interpret.
func (s *Server) voiceCommand(w http.ResponseWriter, r *http.Request) {
	var req voiceCommandReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	action, args := parseVoiceCommand(req.Text)
	resp := map[string]any{"heard": req.Text, "action": action, "args": args, "text_response": fmt.Sprintf("Ho capito: %s %v", action, args)}
	writeJSON(w, 200, resp)
}

// parseVoiceCommand is a tiny V1 keyword router. V2 uses an LLM.
func parseVoiceCommand(s string) (string, []string) {
	low := strings.ToLower(strings.TrimSpace(s))
	tok := strings.Fields(low)
	if len(tok) == 0 {
		return "noop", nil
	}
	switch {
	case strings.HasPrefix(low, "spawn") || strings.HasPrefix(low, "lancia"):
		return "spawn", tok
	case strings.HasPrefix(low, "status") || strings.HasPrefix(low, "stato"):
		return "status", tok
	case strings.HasPrefix(low, "kill") || strings.HasPrefix(low, "ferma") || strings.HasPrefix(low, "ammazza"):
		return "kill", tok
	case strings.HasPrefix(low, "send") || strings.HasPrefix(low, "manda") || strings.HasPrefix(low, "dì a"):
		return "send", tok
	case strings.HasPrefix(low, "merge"):
		return "merge", tok
	case strings.HasPrefix(low, "list") || strings.HasPrefix(low, "elenca"):
		return "list", tok
	}
	return "unknown", tok
}

// ----------------- helpers ------------------------------------------------

func (s *Server) findRole(id string) (roles.Role, bool) {
	for _, r := range s.catalog.Roles {
		if r.ID == id {
			return r, true
		}
	}
	return roles.Role{}, false
}

// mcpConfigJSON builds the .mcp.json dropped into each worker's workdir
// so the worker CLI picks up the bismuth-team MCP server (this binary's
// `mcp` subcommand) backed by the same SQLite database.
func (s *Server) mcpConfigJSON(agentID string) []byte {
	exe, err := os.Executable()
	if err != nil {
		exe = "bismuth"
	}
	dbPath := s.cfg.DB.Path
	if abs, err := filepath.Abs(dbPath); err == nil {
		dbPath = abs
	}
	b, err := json.Marshal(map[string]any{
		"mcpServers": map[string]any{
			"bismuth-team": map[string]any{
				"command": exe,
				"args":    []string{"mcp"},
				"env": map[string]string{
					"BISMUTH_MCP_DB":   dbPath,
					"BISMUTH_AGENT_ID": agentID,
				},
			},
		},
	})
	if err != nil {
		return nil
	}
	return b
}

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + "-" + hex.EncodeToString(b)
}

func shortID() string { return newID("")[4:] }

func firstLine(s string) string {
	if i := strings.Index(s, "\n"); i > 0 {
		return s[:i]
	}
	return s
}

func firstWord(s string) string {
	tok := strings.Fields(s)
	if len(tok) == 0 {
		return ""
	}
	return tok[0]
}

// execCmd is a tiny exec wrapper exposed via the API for debugging
// (e.g. "list git branches"). Off by default. Used by V1 TUI to fetch
// repo info without spawning a worker.
var _ = exec.Command

// ----------------- shared memory handlers ---------------------------------

func (s *Server) postMemory(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
		Key     string `json:"key"`
		Value   string `json:"value"`
		Tags    string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.AgentID == "" || req.Key == "" || req.Value == "" {
		writeErr(w, 400, errors.New("agent_id, key, value required"))
		return
	}
	if s.mem == nil {
		writeErr(w, 503, errors.New("shared memory not initialized"))
		return
	}
	m := &sharedmem.Memory{
		AgentID: req.AgentID,
		Key:     req.Key,
		Value:   req.Value,
		Tags:    req.Tags,
	}
	if err := s.mem.Post(r.Context(), m); err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 201, map[string]any{"ok": true, "id": m.ID})
}

func (s *Server) queryMemory(w http.ResponseWriter, r *http.Request) {
	if s.mem == nil {
		writeErr(w, 503, errors.New("shared memory not initialized"))
		return
	}
	q := r.URL.Query().Get("q")
	agentID := r.URL.Query().Get("agent_id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 20
	}

	var results []*sharedmem.Memory
	var err error
	if agentID != "" && q == "" {
		results, err = s.mem.List(r.Context(), agentID, limit)
	} else if q != "" {
		results, err = s.mem.Query(r.Context(), q, limit)
	} else {
		results, err = s.mem.List(r.Context(), "", limit)
	}
	if err != nil {
		writeErr(w, 500, err)
		return
	}
	writeJSON(w, 200, map[string]any{"memories": results})
}

// ----------------- voice room handlers (LiveKit stub) ---------------------

func (s *Server) createVoiceRoom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, 400, err)
		return
	}
	if req.AgentID == "" {
		writeErr(w, 400, fmt.Errorf("agent_id required"))
		return
	}
	logger.Info("voice room create request", "agent_id", req.AgentID)
	// Stub: return placeholder room info
	writeJSON(w, 201, map[string]any{
		"room_name": "bismuth-" + req.AgentID,
		"agent_id":  req.AgentID,
		"state":     "active",
		"note":      "LiveKit stub — connect real SFU for production",
	})
}

func (s *Server) listVoiceRooms(w http.ResponseWriter, r *http.Request) {
	// Stub: return empty list
	writeJSON(w, 200, map[string]any{"rooms": []any{}})
}

func (s *Server) endVoiceRoom(w http.ResponseWriter, r *http.Request) {
	roomName := r.URL.Query().Get("room")
	if roomName == "" {
		writeErr(w, 400, fmt.Errorf("room query param required"))
		return
	}
	logger.Info("voice room end request", "room", roomName)
	writeJSON(w, 200, map[string]any{"room": roomName, "state": "ended"})
}
