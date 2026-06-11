// bridge package: hermetic tests against httptest fakes for the
// Telegram API, the Discord webhook and the bismuth control API.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/biodoia/bismuth/internal/db"
)

const (
	testToken  = "TESTTOKEN"
	testChatID = "42"
	// testPoll keeps the loops snappy; total package runtime stays well
	// under the 5s budget.
	testPoll = 15 * time.Millisecond
)

// ----------------- test helpers --------------------------------------------

// sentMsg is one message captured by a fake sink.
type sentMsg struct {
	ChatID string
	Text   string
}

// msgRecorder collects messages delivered to a fake sink.
type msgRecorder struct {
	mu   sync.Mutex
	msgs []sentMsg
}

func (r *msgRecorder) add(m sentMsg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, m)
}

func (r *msgRecorder) snapshot() []sentMsg {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]sentMsg(nil), r.msgs...)
}

func (r *msgRecorder) contains(sub string) bool {
	for _, m := range r.snapshot() {
		if strings.Contains(m.Text, sub) {
			return true
		}
	}
	return false
}

// waitFor polls cond every 10ms until it holds or timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return cond()
}

func writeTestJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// tgUpd builds one canned Telegram update.
func tgUpd(id, chatID int64, text string) map[string]any {
	return map[string]any{
		"update_id": id,
		"message": map[string]any{
			"chat": map[string]any{"id": chatID},
			"text": text,
		},
	}
}

// fakeTelegram fakes api.telegram.org: sendMessage records into sent,
// getUpdates serves the canned updates whose update_id >= offset (so a
// correctly advancing offset drains the queue exactly once).
func fakeTelegram(t *testing.T, sent *msgRecorder, updates []map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/bot"+testToken+"/sendMessage", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ChatID any    `json:"chat_id"`
			Text   string `json:"text"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent.add(sentMsg{ChatID: fmt.Sprint(body.ChatID), Text: body.Text})
		writeTestJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/bot"+testToken+"/getUpdates", func(w http.ResponseWriter, r *http.Request) {
		offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
		result := []map[string]any{}
		for _, u := range updates {
			if id, ok := u["update_id"].(int64); ok && id >= offset {
				result = append(result, u)
			}
		}
		writeTestJSON(w, map[string]any{"ok": true, "result": result})
	})
	return httptest.NewServer(mux)
}

// fakeDiscord records webhook posts ({content}).
func fakeDiscord(t *testing.T, sent *msgRecorder) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Content string `json:"content"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		sent.add(sentMsg{Text: body.Content})
		w.WriteHeader(http.StatusNoContent)
	}))
}

// openTestDB opens an in-memory store and returns an insert helper for
// the events table.
func openTestDB(t *testing.T, ctx context.Context) (*db.Store, func(typ, agentID, taskID, payload string)) {
	t.Helper()
	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	insert := func(typ, agentID, taskID, payload string) {
		t.Helper()
		if _, err := store.DB().ExecContext(ctx,
			`INSERT INTO events(type, agent_id, task_id, payload, ts) VALUES(?,?,?,?,?)`,
			typ, agentID, taskID, payload, "2026-06-10T00:00:00Z"); err != nil {
			t.Fatal(err)
		}
	}
	return store, insert
}

// startBridge runs br.Run in the background and fails the test if it
// does not stop promptly once ctx is cancelled.
func startBridge(t *testing.T, ctx context.Context, br *Bridge) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- br.Run(ctx) }()
	t.Cleanup(func() {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Run did not stop after ctx cancel")
		}
	})
}

// ----------------- tests ---------------------------------------------------

func TestConfigEnabled(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"empty", Config{}, false},
		{"token only", Config{TelegramToken: "t"}, false},
		{"chat only", Config{TelegramChatID: "1"}, false},
		{"telegram complete", Config{TelegramToken: "t", TelegramChatID: "1"}, true},
		{"discord only", Config{DiscordWebhookURL: "http://hook"}, true},
		{"both", Config{TelegramToken: "t", TelegramChatID: "1", DiscordWebhookURL: "http://hook"}, true},
	}
	for _, c := range cases {
		if got := c.cfg.Enabled(); got != c.want {
			t.Errorf("%s: Enabled() = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	br := New(Config{}, nil)
	if br.cfg.APIBase != "http://127.0.0.1:9000" {
		t.Errorf("APIBase = %q", br.cfg.APIBase)
	}
	if br.cfg.PollInterval != 3*time.Second {
		t.Errorf("PollInterval = %v", br.cfg.PollInterval)
	}
	if br.cfg.TelegramAPIBase != "https://api.telegram.org" {
		t.Errorf("TelegramAPIBase = %q", br.cfg.TelegramAPIBase)
	}
}

func TestFormatEvent(t *testing.T) {
	cases := []struct {
		name string
		e    event
		want string
	}{
		{"spawned with role+cli",
			event{Type: "agent_spawned", AgentID: "agt-1", Payload: `{"role":"implementer","cli":"omx"}`},
			"🤖 agent spawned: agt-1 (implementer/omx)"},
		{"spawned bare",
			event{Type: "agent_spawned", AgentID: "agt-2", Payload: `{}`},
			"🤖 agent spawned: agt-2"},
		{"killed",
			event{Type: "agent_killed", AgentID: "agt-1", Payload: `{}`},
			"💀 agent killed: agt-1"},
		{"assigned",
			event{Type: "task_assigned", AgentID: "agt-1", TaskID: "tsk-9", Payload: `{}`},
			"📌 task assigned: tsk-9 → agt-1"},
		{"approval required",
			event{Type: "human_approval_required", TaskID: "tsk-9", Payload: `{"action":"merge","branch":"bismuth/tsk-9"}`},
			"✋ human approval required: tsk-9 (merge/bismuth/tsk-9)"},
		{"state exited",
			event{Type: "agent_state", AgentID: "agt-1", Payload: `{"pane_id":"p1","state":"exited"}`},
			"🏁 agent exited: agt-1"},
		{"state working skipped",
			event{Type: "agent_state", AgentID: "agt-1", Payload: `{"state":"working"}`},
			""},
		{"unrelated type skipped",
			event{Type: "pane_output", AgentID: "agt-1", Payload: `{}`},
			""},
	}
	for _, c := range cases {
		if got := formatEvent(c.e); got != c.want {
			t.Errorf("%s: formatEvent = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestClip(t *testing.T) {
	if got := clip("short", 10); got != "short" {
		t.Errorf("clip(short) = %q", got)
	}
	long := strings.Repeat("x", 5000)
	if got := clip(long, telegramMaxLen); len(got) > telegramMaxLen {
		t.Errorf("clip left %d bytes, want <= %d", len(got), telegramMaxLen)
	}
	// Never split a multi-byte rune at the cut point.
	if got := clip("aé"+strings.Repeat("é", 10), 4); !strings.HasSuffix(got, "…") || len(got) > 4 {
		t.Errorf("clip on rune boundary = %q (%d bytes)", got, len(got))
	}
}

// TestNotifierDeliversToTelegramAndDiscord inserts qualifying event rows
// after Run starts and asserts both sinks receive them — and that
// history, non-exited agent_state and unrelated types stay out.
func TestNotifierDeliversToTelegramAndDiscord(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, insert := openTestDB(t, ctx)

	// History: present before Run, must never be replayed.
	insert("agent_spawned", "agt-old", "", `{"role":"historian","cli":"omx"}`)

	tgSent := &msgRecorder{}
	tg := fakeTelegram(t, tgSent, nil)
	defer tg.Close()

	dcSent := &msgRecorder{}
	dc := fakeDiscord(t, dcSent)
	defer dc.Close()

	br := New(Config{
		TelegramToken:     testToken,
		TelegramChatID:    testChatID,
		DiscordWebhookURL: dc.URL,
		TelegramAPIBase:   tg.URL,
		PollInterval:      testPoll,
	}, store.DB())
	startBridge(t, ctx, br)

	// Give the notifier time to snapshot MAX(seq) before new rows land.
	time.Sleep(100 * time.Millisecond)

	insert("agent_spawned", "agt-1", "tsk-1", `{"role":"implementer","cli":"omx"}`)
	insert("agent_state", "agt-1", "", `{"pane_id":"p1","state":"working"}`) // filtered out
	insert("pane_output", "agt-1", "", `{"bytes":12}`)                       // filtered out
	insert("agent_state", "agt-1", "", `{"pane_id":"p1","state":"exited"}`)

	if !waitFor(t, 3*time.Second, func() bool {
		return tgSent.contains("agent spawned: agt-1") && tgSent.contains("agent exited: agt-1") &&
			dcSent.contains("agent spawned: agt-1") && dcSent.contains("agent exited: agt-1")
	}) {
		t.Fatalf("notifications not delivered; telegram=%v discord=%v", tgSent.snapshot(), dcSent.snapshot())
	}

	// sendMessage must target the configured chat.
	for _, m := range tgSent.snapshot() {
		if m.ChatID != testChatID {
			t.Errorf("sendMessage chat_id = %q, want %q", m.ChatID, testChatID)
		}
	}

	// Delivery is sequential in seq order, so once "exited" landed in
	// both sinks anything delivered before it is already recorded.
	for _, m := range append(tgSent.snapshot(), dcSent.snapshot()...) {
		if strings.Contains(m.Text, "agt-old") {
			t.Errorf("history replayed: %q", m.Text)
		}
		if strings.Contains(m.Text, "working") {
			t.Errorf("non-exited agent_state delivered: %q", m.Text)
		}
	}

	cancel()
}

// TestTelegramCommandsDriveControlAPI feeds /status and /kill through
// the getUpdates fake and asserts the bridge calls the control API fake
// (kill POST included) and replies in the chat. A command from a
// foreign chat id must be ignored.
func TestTelegramCommandsDriveControlAPI(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, _ := openTestDB(t, ctx)

	killed := &msgRecorder{}
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/agents":
			writeTestJSON(w, map[string]any{"agents": []map[string]any{
				{"id": "agt-1", "role": "implementer", "state": "working"},
			}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tasks":
			writeTestJSON(w, map[string]any{"tasks": []map[string]any{
				{"id": "tsk-1", "title": "Fix bug", "status": "open"},
			}})
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/api/v1/agents/") && strings.HasSuffix(r.URL.Path, "/kill"):
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/agents/"), "/kill")
			killed.add(sentMsg{Text: id})
			writeTestJSON(w, map[string]any{"ok": true})
		default:
			http.NotFound(w, r)
		}
	}))
	defer api.Close()

	updates := []map[string]any{
		tgUpd(1, 42, "/status"),
		tgUpd(2, 999, "/kill agt-intruder"), // wrong chat: must be ignored
		tgUpd(3, 42, "/kill agt-1"),
		tgUpd(4, 42, "/frobnicate"), // unknown: help text
	}
	tgSent := &msgRecorder{}
	tg := fakeTelegram(t, tgSent, updates)
	defer tg.Close()

	br := New(Config{
		TelegramToken:   testToken,
		TelegramChatID:  testChatID,
		TelegramAPIBase: tg.URL,
		APIBase:         api.URL,
		PollInterval:    testPoll,
	}, store.DB())
	startBridge(t, ctx, br)

	if !waitFor(t, 3*time.Second, func() bool {
		return tgSent.contains("agents: 1") &&
			tgSent.contains("ok: killed agt-1") &&
			tgSent.contains("/kill <agent_id>") // help text for /frobnicate
	}) {
		t.Fatalf("command replies missing; got %v", tgSent.snapshot())
	}

	// /status reply: compact summary with counts and first lines.
	var status string
	for _, m := range tgSent.snapshot() {
		if strings.Contains(m.Text, "agents: 1") {
			status = m.Text
		}
	}
	for _, want := range []string{"agt-1", "implementer", "working", "tasks: 1", "tsk-1", "Fix bug"} {
		if !strings.Contains(status, want) {
			t.Errorf("/status reply missing %q: %q", want, status)
		}
	}

	// The kill POST hit the control API for agt-1 only; the foreign-chat
	// command never reached it. Updates are processed in one batch, so
	// once the agt-1 kill is recorded the intruder verdict is final.
	if !killed.contains("agt-1") {
		t.Fatal("kill POST for agt-1 never hit the control API")
	}
	if killed.contains("agt-intruder") {
		t.Errorf("kill from foreign chat reached the control API: %v", killed.snapshot())
	}

	cancel()
}

// TestDiscordOnlyNotifier runs the bridge with just the webhook
// configured: notifications flow, and no Telegram loop is needed.
func TestDiscordOnlyNotifier(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	store, insert := openTestDB(t, ctx)

	dcSent := &msgRecorder{}
	dc := fakeDiscord(t, dcSent)
	defer dc.Close()

	br := New(Config{
		DiscordWebhookURL: dc.URL,
		PollInterval:      testPoll,
	}, store.DB())
	startBridge(t, ctx, br)

	time.Sleep(100 * time.Millisecond)
	insert("agent_killed", "agt-9", "", `{}`)

	if !waitFor(t, 3*time.Second, func() bool {
		return dcSent.contains("agent killed: agt-9")
	}) {
		t.Fatalf("discord content not posted; got %v", dcSent.snapshot())
	}

	cancel()
}
