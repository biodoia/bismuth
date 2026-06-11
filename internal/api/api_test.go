package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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

// TestAuditEndpoint verifies P7-h: spawning leaves an audit entry and
// /api/v1/audit returns the trail newest-first.
func TestAuditEndpoint(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	_, _ = env.post("/api/v1/agents", map[string]any{
		"role": "implementer", "cli": "bash", "task": "echo audit-me",
	})

	resp, body := env.get("/api/v1/audit?limit=10")
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	entries, ok := body["entries"].([]any)
	if !ok || len(entries) == 0 {
		t.Fatalf("expected audit entries, got %v", body["entries"])
	}
	first := entries[0].(map[string]any)
	if first["action"] != "spawn_agent" {
		t.Errorf("newest entry action = %v, want spawn_agent", first["action"])
	}
	if first["row_hash"] == "" || first["row_hash"] == nil {
		t.Errorf("entry missing row_hash")
	}
}

// TestVoiceCommandWakeWord verifies P7-b: continuous-mode utterances
// without the wake word are ignored; with it, the command is parsed.
func TestVoiceCommandWakeWord(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	// Ambient speech: ignored.
	resp, body := env.post("/v1/voice/command", map[string]any{
		"text": "che ore sono", "continuous": true,
	})
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if body["ignored"] != true {
		t.Fatalf("expected ignored=true, got %v", body)
	}

	// Wake word present: parsed, wake word stripped.
	_, body = env.post("/v1/voice/command", map[string]any{
		"text": "bismuth status agenti", "continuous": true,
	})
	if body["action"] != "status" {
		t.Errorf("action = %v, want status", body["action"])
	}

	// Push-to-talk (non-continuous): no gating.
	_, body = env.post("/v1/voice/command", map[string]any{
		"text": "status", "continuous": false,
	})
	if body["action"] != "status" {
		t.Errorf("non-continuous action = %v, want status", body["action"])
	}
}

// TestTenantScoping verifies P7-e: agents spawned under a tenant are
// only listed for that tenant.
func TestTenantScoping(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	postT := func(tenant string) string {
		b, _ := json.Marshal(map[string]any{"role": "implementer", "cli": "bash", "task": "echo hi"})
		req, _ := http.NewRequest("POST", env.ts.URL+"/api/v1/agents", bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Bismuth-Tenant", tenant)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 201 {
			t.Fatalf("spawn under tenant %q: %d", tenant, resp.StatusCode)
		}
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		id, _ := body["agent_id"].(string)
		return id
	}
	listT := func(tenant string) int {
		req, _ := http.NewRequest("GET", env.ts.URL+"/api/v1/agents", nil)
		if tenant != "" {
			req.Header.Set("X-Bismuth-Tenant", tenant)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		agents, _ := body["agents"].([]any)
		return len(agents)
	}

	getT := func(agentID, tenant string) int {
		req, _ := http.NewRequest("GET", env.ts.URL+"/api/v1/agents/"+agentID, nil)
		if tenant != "" {
			req.Header.Set("X-Bismuth-Tenant", tenant)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}

	agentID := postT("team-a")
	if n := listT("team-a"); n != 1 {
		t.Errorf("team-a sees %d agents, want 1", n)
	}
	if n := listT("team-b"); n != 0 {
		t.Errorf("team-b sees %d agents, want 0", n)
	}
	if n := listT(""); n != 0 { // default namespace
		t.Errorf("default tenant sees %d agents, want 0", n)
	}

	// Per-id isolation: a guessed id from another tenant must 404.
	if code := getT(agentID, "team-a"); code != 200 {
		t.Errorf("owner tenant get: %d, want 200", code)
	}
	if code := getT(agentID, "team-b"); code != 404 {
		t.Errorf("cross-tenant get: %d, want 404", code)
	}
	if code := getT(agentID, ""); code != 404 { // default namespace
		t.Errorf("default-tenant get of team-a agent: %d, want 404", code)
	}
}

// TestAgentStreamSSE verifies P7-j: /stream speaks SSE and delivers the
// initial state snapshot.
func TestAgentStreamSSE(t *testing.T) {
	env := newTestEnv(t)
	defer env.Close()

	_, sp := env.post("/api/v1/agents", map[string]any{
		"role": "implementer", "cli": "bash", "task": "echo stream-me",
	})
	agentID, _ := sp["agent_id"].(string)
	if agentID == "" {
		t.Fatalf("no agent_id in spawn response: %v", sp)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", env.ts.URL+"/api/v1/agents/"+agentID+"/stream", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream request: %v", err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	// Read until the initial state event arrives.
	sc := bufio.NewScanner(resp.Body)
	sawState := false
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "event: state") {
			sawState = true
			break
		}
	}
	if !sawState {
		t.Fatalf("never received initial state event")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
