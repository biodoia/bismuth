package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/biodoia/bismuth/internal/audit"
	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/db"
	"github.com/biodoia/bismuth/internal/logger"
	"github.com/biodoia/bismuth/internal/pane"
	"github.com/biodoia/bismuth/internal/roles"
	"github.com/biodoia/bismuth/internal/security"
	"github.com/biodoia/bismuth/internal/voice"
)

// testEnv sets up a full server with in-memory SQLite for testing.
type testEnv struct {
	srv    *Server
	store  *db.Store
	bus    *bus.Bus
	pm     *pane.Manager
	ts     *httptest.Server
	cancel context.CancelFunc
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	logger.Init("bismuth-test", "text")

	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	sqlDB := store.DB()
	busServer := bus.New(sqlDB)
	catalog := roles.DefaultCatalog()
	cfg := &config.Config{
		Server: config.ServerCfg{Port: 0},
		DB:     config.DBCfg{Path: ":memory:"},
		Audit:  config.AuditCfg{Salt: "test-salt-do-not-use-in-prod"},
		API:    config.APICfg{TailscaleOnly: false},
		Pane:   config.PaneCfg{},
	}
	pm := pane.NewManager(sqlDB, busServer, cfg.Pane)
	voiceGW, _ := voice.NewGateway(ctx, config.VoiceCfg{}, sqlDB, busServer)
	auditLog, _ := audit.New(sqlDB, "test-salt-do-not-use-in-prod")
	sec := security.New(config.SecurityCfg{})

	// repoRoot lives in a throwaway dir (not a git repo): worktree
	// creation fails best-effort, and tests never register worktrees
	// or branches in the real repository.
	srv := NewServer(cfg, store, busServer, pm, voiceGW, auditLog, sec, catalog, t.TempDir())

	ts := httptest.NewServer(srv.Handler())

	return &testEnv{srv: srv, store: store, bus: busServer, pm: pm, ts: ts, cancel: cancel}
}

func (e *testEnv) Close() {
	e.cancel()
	e.ts.Close()
	e.pm.Close() // stop worker panes (and their readLoops) before the DB closes
	e.store.Close()
}

func (e *testEnv) get(path string) (*http.Response, map[string]any) {
	resp, err := http.Get(e.ts.URL + path)
	if err != nil {
		return resp, nil
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var body map[string]any
	json.Unmarshal(raw, &body)
	return resp, body
}

func (e *testEnv) post(path string, payload any) (*http.Response, map[string]any) {
	b, _ := json.Marshal(payload)
	resp, err := http.Post(e.ts.URL+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return resp, nil
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var body map[string]any
	json.Unmarshal(raw, &body)
	return resp, body
}

// -------------------- Tests --------------------

func TestHealthz(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, body := env.get("/healthz")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["ok"] != true {
		t.Fatalf("expected ok=true, got %v", body["ok"])
	}
}

func TestListAgentsEmpty(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, body := env.get("/api/v1/agents")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body == nil {
		t.Fatal("body is nil — JSON decode failed")
	}
	raw := body["agents"]
	if raw == nil {
		return // null is equivalent to empty list
	}
	agents, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected agents array, got %T", raw)
	}
	if len(agents) != 0 {
		t.Fatalf("expected 0 agents, got %d", len(agents))
	}
}

func TestSpawnAgent(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, body := env.post("/api/v1/agents", map[string]any{
		"role": "implementer",
		"cli":  "bash",
		"task": "echo hello",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d body=%v", resp.StatusCode, body)
	}
	if body["agent_id"] == nil {
		t.Fatal("expected agent_id")
	}
	if body["role"] != "implementer" {
		t.Fatalf("expected role=implementer, got %v", body["role"])
	}
}

func TestSpawnAndReadAgent(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	_, sp := env.post("/api/v1/agents", map[string]any{
		"role": "implementer",
		"cli":  "bash",
		"task": "echo hello from agent",
	})
	agentID := sp["agent_id"].(string)

	resp, body := env.get("/api/v1/agents/" + agentID)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["id"] != agentID {
		t.Fatalf("expected id=%s, got %v", agentID, body["id"])
	}
}

func TestSpawnAndKillAgent(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	_, sp := env.post("/api/v1/agents", map[string]any{
		"role": "implementer",
		"cli":  "bash",
		"task": "sleep 30",
	})
	agentID := sp["agent_id"].(string)

	resp, body := env.post("/api/v1/agents/"+agentID+"/kill", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d body=%v", resp.StatusCode, body)
	}
}

func TestListRoles(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, body := env.get("/api/v1/roles")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	rolesList, ok := body["roles"].([]any)
	if !ok {
		t.Fatalf("expected roles array, got %T", body["roles"])
	}
	if len(rolesList) < 4 {
		t.Fatalf("expected at least 4 roles, got %d", len(rolesList))
	}
}

func TestSharedMemory(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	// post memory
	resp, _ := env.post("/api/v1/memory", map[string]any{
		"agent_id": "agt-test",
		"key":      "architecture decision",
		"value":    "use SQLite FTS5 for shared memory",
		"tags":     "arch,db",
	})
	if resp.StatusCode != 201 && resp.StatusCode != 200 {
		t.Fatalf("expected 200/201 on memory post, got %d", resp.StatusCode)
	}

	// query memory
	resp, body := env.get("/api/v1/memory?q=FTS5")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	memories, ok := body["memories"].([]any)
	if !ok {
		t.Fatalf("expected memories array, got %T", body["memories"])
	}
	if len(memories) != 1 {
		t.Fatalf("expected 1 memory, got %d", len(memories))
	}

	m := memories[0].(map[string]any)
	if m["key"] != "architecture decision" {
		t.Fatalf("unexpected key: %v", m["key"])
	}
}

func TestVoiceRooms(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, body := env.post("/api/v1/voice/rooms", map[string]any{
		"agent_id": "agt-voice-test",
	})
	if resp.StatusCode != 201 {
		t.Fatalf("expected 201, got %d body=%v", resp.StatusCode, body)
	}
	if body["state"] != "active" {
		t.Fatalf("expected state=active, got %v", body["state"])
	}

	// list rooms (stub returns empty)
	resp, body = env.get("/api/v1/voice/rooms")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// end room
	resp, _ = env.post("/api/v1/voice/rooms/end?room=bismuth-agt-voice-test", nil)
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, _ := http.Get(env.ts.URL + "/metrics")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte("bismuth_agents_spawned_total")) {
		t.Fatalf("expected bismuth_agents_spawned_total in metrics")
	}
}

func TestUnknownAgent404(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, _ := env.get("/api/v1/agents/agt-nonexistent")
	if resp.StatusCode != 404 {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestSpawnInvalidRole(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	resp, _ := env.post("/api/v1/agents", map[string]any{
		"role": "nonexistent_role",
		"cli":  "bash",
	})
	if resp.StatusCode != 400 {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

// TestRBACEnforcement verifies P7-g: an authenticated viewer cannot
// spawn, send to, or kill agents (403), while requests without a user
// (localhost CLI) keep working — covered by the other spawn tests.
func TestRBACEnforcement(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	viewer := &security.User{Email: "viewer@example.com", Role: security.RoleViewer}
	h := env.srv.Handler()

	do := func(method, path string, payload any) int {
		var rdr io.Reader
		if payload != nil {
			b, _ := json.Marshal(payload)
			rdr = bytes.NewReader(b)
		}
		req := httptest.NewRequest(method, path, rdr)
		req = req.WithContext(security.ContextWithUser(req.Context(), viewer))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w.Code
	}

	if code := do("POST", "/api/v1/agents", map[string]any{"role": "implementer", "cli": "bash", "task": "x"}); code != 403 {
		t.Errorf("viewer spawn: got %d, want 403", code)
	}
	if code := do("POST", "/api/v1/agents/agt-x/send", map[string]any{"data_b64": "bHM="}); code != 403 {
		t.Errorf("viewer send: got %d, want 403", code)
	}
	if code := do("POST", "/api/v1/agents/agt-x/kill", nil); code != 403 {
		t.Errorf("viewer kill: got %d, want 403", code)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
