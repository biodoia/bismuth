// Package api is the HTTP/WebSocket server for bismuth.
//
// Routes (V1):
//
//   GET  /healthz                                  -- liveness
//
//   GET  /api/v1/agents                            -- list agents
//   POST /api/v1/agents                            -- spawn agent { role, cli, task, args }
//   GET  /api/v1/agents/:id                        -- agent detail
//   POST /api/v1/agents/:id/send                   -- send bytes { data_b64 }
//   GET  /api/v1/agents/:id/read                   -- read last N lines ?n=200
//   POST /api/v1/agents/:id/kill                   -- terminate
//
//   GET  /api/v1/tasks                             -- list tasks
//   POST /api/v1/tasks                             -- create task { title, description, priority, parent_id }
//   GET  /api/v1/tasks/:id                         -- task detail
//   POST /api/v1/tasks/:id/assign                  -- assign { agent_id }
//   POST /api/v1/tasks/:id/merge                   -- merge branch
//
//   GET  /api/v1/roles                             -- role catalog
//   GET  /api/v1/events                            -- recent events ?types=&agent_id=&limit=
//
//   GET  /api/v1/ws                                -- WebSocket subscribe ?types=&agent_id=
//
//   POST /v1/voice/stt                             -- multipart audio -> { text }
//   POST /v1/voice/speak                           -- { text } -> { audio_b64, format }
//   POST /v1/voice/command                         -- { text } -> { action, args, text_response }
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
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/biodoia/bismuth/internal/audit"
	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/db"
	"github.com/biodoia/bismuth/internal/pane"
	"github.com/biodoia/bismuth/internal/roles"
	"github.com/biodoia/bismuth/internal/security"
	"github.com/biodoia/bismuth/internal/shared"
	"github.com/biodoia/bismuth/internal/voice"
	"github.com/biodoia/bismuth/internal/worktree"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Server bundles the dependencies.
type Server struct {
	cfg   config.APICfg
	store *db.Store
	bus   *bus.Bus
	pane  *pane.Manager
	voice *voice.Gateway
	audit *audit.Log
	sec   *security.Policy

	repoRoot string
	catalog  roles.Catalog

	upgrader websocket.Upgrader
	seq      atomic.Int64
}

// NewServer wires the HTTP routes.
func NewServer(
	cfg config.APICfg,
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
	return s
}

// Run starts the HTTP server until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	r := chi.NewRouter()

	r.Use(s.authMiddleware)
	r.Use(loggingMiddleware)

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

	// voice
	r.Post("/v1/voice/stt", s.voiceSTT)
	r.Post("/v1/voice/speak", s.voiceSpeak)
	r.Post("/v1/voice/command", s.voiceCommand)

	addr := ":9000"
	srv := &http.Server{
		Addr:              addr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	fmt.Fprintln(outWriter(), "bismuth listening on", addr)

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
		if !s.cfg.TailscaleOnly {
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
		for _, c := range s.cfg.AllowedCIDRs {
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
		fmt.Fprintf(outWriter(), "%s %s %d %s\n",
			r.Method, r.URL.Path, ww.status, time.Since(start))
	})
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
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

func outWriter() io.Writer { return stdOut }

var stdOut io.Writer = writerFor(nil)

func writerFor(_ any) io.Writer { return defaultWriter() }

func defaultWriter() io.Writer {
	if _w == nil {
		_w = newLockedWriter()
	}
	return _w
}

var _w io.Writer

type lockedWriter struct{}

func newLockedWriter() io.Writer { return &syncLocked{} }

type syncLocked struct{}

func (s *syncLocked) Write(p []byte) (int, error) { return writeStdout(p) }

// indirection so tests can swap; in prod just writes to stdout
func writeStdout(p []byte) (int, error) { return stdOutReal.Write(p) }

var stdOutReal io.Writer = stdoutAdapter{}

type stdoutAdapter struct{}

func (stdoutAdapter) Write(p []byte) (int, error) {
	return fmtPrint(p)
}

func fmtPrint(p []byte) (int, error) { return fmt.Print(string(p)) }

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
			ID:          taskID,
			Title:       firstLine(req.Task),
			Description: sql.NullString{String: req.Task, Valid: true},
			Status:      "open",
			Priority:    0,
			Branch:      sql.NullString{String: branch, Valid: branch != ""},
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
	_, _ = s.pane.Spawn(r.Context(), agentID, req.CLI, role.ID, cmd, []string{
		"BISMUTH_AGENT_ID=" + agentID,
		"BISMUTH_ROLE=" + role.ID,
		"BISMUTH_TASK_ID=" + taskID,
		"BISMUTH_PANE_ID=" + paneID,
		"BISMUTH_MODEL=" + role.DefaultModel,
		"BISMUTH_WORKDIR=" + wtDir,
	})

	_ = s.audit.Append(r.Context(), "user:lisergico25", "spawn_agent", agentID,
		shared.JSONRaw(map[string]any{"role": role.ID, "cli": req.CLI, "task": taskID}))

	_ = s.bus.Publish(r.Context(), bus.Event{
		Type:    "agent_spawned",
		AgentID: agentID,
		TaskID:  taskID,
		Payload: shared.JSONRaw(map[string]any{"role": role.ID, "cli": req.CLI, "pane_id": paneID}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
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
	// try persisted scrollback first
	if a.PaneID.Valid {
		p, err := s.store.GetPane(r.Context(), a.PaneID.String)
		if err == nil && p.Scrollback.Valid && p.Scrollback.String != "" {
			writeJSON(w, 200, map[string]any{
				"agent_id":   id,
				"pane_id":    a.PaneID.String,
				"scrollback": p.Scrollback.String,
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
