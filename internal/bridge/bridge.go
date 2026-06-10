// Package bridge forwards bismuth events to Telegram and Discord and
// answers Telegram chat commands — the operator's phone becomes the
// remote control.
//
// The bridge runs two loops, both stopped by ctx:
//
//  1. NOTIFIER: polls the events table every PollInterval for new rows
//     (seq > lastSeen) of operator-relevant types, formats each as a
//     short human message and delivers it to every configured sink
//     (Telegram sendMessage, Discord webhook). Delivery is best-effort:
//     errors are swallowed and never crash the loop.
//  2. COMMANDS (Telegram only): long-polls getUpdates and answers
//     /status, /agents, /tasks and /kill by calling the bismuth control
//     API on APIBase. Only messages from the configured chat id are
//     accepted; everyone else who finds the bot is ignored.
//
// History is never replayed: lastSeen starts at the current MAX(seq).
package bridge

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	defaultAPIBase         = "http://127.0.0.1:9000"
	defaultPollInterval    = 3 * time.Second
	defaultTelegramAPIBase = "https://api.telegram.org"

	telegramMaxLen = 3500 // keep replies under Telegram's 4096-char hard cap
	discordMaxLen  = 2000 // Discord webhook content hard cap
	maxListLines   = 15   // per-section line cap for /status, /agents, /tasks
)

// Config selects the sinks and where to find the control API. Zero
// fields get production defaults in New; tests point APIBase and
// TelegramAPIBase at httptest servers.
type Config struct {
	TelegramToken     string        // bot token from @BotFather
	TelegramChatID    string        // numeric chat id the bridge talks to
	DiscordWebhookURL string        // full webhook URL (notifications only)
	APIBase           string        // bismuth control API base URL
	PollInterval      time.Duration // events-table poll cadence
	TelegramAPIBase   string        // Telegram API base, overridden in tests
}

// Enabled reports whether at least one sink is fully configured.
func (c Config) Enabled() bool {
	return c.telegramConfigured() || c.DiscordWebhookURL != ""
}

func (c Config) telegramConfigured() bool {
	return c.TelegramToken != "" && c.TelegramChatID != ""
}

// Bridge is the notifier plus the Telegram command responder.
type Bridge struct {
	cfg    Config
	db     *sql.DB
	client *http.Client
}

// New creates a bridge reading events from sqlDB (the bismuth store's
// underlying *sql.DB). It does not start anything yet; call Run.
func New(cfg Config, sqlDB *sql.DB) *Bridge {
	if cfg.APIBase == "" {
		cfg.APIBase = defaultAPIBase
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = defaultPollInterval
	}
	if cfg.TelegramAPIBase == "" {
		cfg.TelegramAPIBase = defaultTelegramAPIBase
	}
	cfg.APIBase = strings.TrimSuffix(cfg.APIBase, "/")
	cfg.TelegramAPIBase = strings.TrimSuffix(cfg.TelegramAPIBase, "/")
	return &Bridge{
		cfg: cfg,
		db:  sqlDB,
		// The client must outlive the Telegram long poll (timeout=10
		// server-side), so its timeout sits above that.
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// Run starts the notifier loop and, when Telegram is configured, the
// command loop. It blocks until ctx is cancelled.
func (b *Bridge) Run(ctx context.Context) error {
	if !b.cfg.Enabled() {
		<-ctx.Done()
		return ctx.Err()
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); b.notifyLoop(ctx) }()
	if b.cfg.telegramConfigured() {
		wg.Add(1)
		go func() { defer wg.Done(); b.commandLoop(ctx) }()
	}
	wg.Wait()
	return ctx.Err()
}

// ----------------- notifier ------------------------------------------------

// event is one row of the events table (read-only for the bridge).
type event struct {
	Seq     int64
	Type    string
	AgentID string
	TaskID  string
	Payload string
	TS      string
}

// notifyLoop polls the events table every PollInterval and pushes new
// qualifying events to the sinks. lastSeen starts at the current
// MAX(seq) so a bismuth restart does not replay history at the operator.
func (b *Bridge) notifyLoop(ctx context.Context) {
	lastSeen := b.maxSeq(ctx)
	t := time.NewTicker(b.cfg.PollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		// Snapshot the high-water mark first: rows inserted after it
		// are simply picked up on the next tick, never skipped.
		newMax := b.maxSeq(ctx)
		if newMax <= lastSeen {
			continue
		}
		for _, e := range b.pendingEvents(ctx, lastSeen, newMax) {
			if msg := formatEvent(e); msg != "" {
				b.deliver(ctx, msg)
			}
		}
		lastSeen = newMax
	}
}

// maxSeq returns the current MAX(seq) of the events table (0 on error).
func (b *Bridge) maxSeq(ctx context.Context) int64 {
	var seq int64
	_ = b.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM events`).Scan(&seq)
	return seq
}

// pendingEvents returns the operator-relevant events with lo < seq <= hi
// in insertion order. Best-effort: errors yield what was scanned so far.
func (b *Bridge) pendingEvents(ctx context.Context, lo, hi int64) []event {
	rows, err := b.db.QueryContext(ctx, `
		SELECT seq, type, COALESCE(agent_id, ''), COALESCE(task_id, ''), payload, ts
		FROM events
		WHERE seq > ? AND seq <= ?
		  AND (type IN ('agent_spawned', 'agent_killed', 'task_assigned', 'human_approval_required')
		       OR (type = 'agent_state' AND payload LIKE '%exited%'))
		ORDER BY seq ASC`, lo, hi)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.Seq, &e.Type, &e.AgentID, &e.TaskID, &e.Payload, &e.TS); err != nil {
			return out
		}
		out = append(out, e)
	}
	return out
}

// formatEvent renders one event as a short human notification. An empty
// string means "do not notify".
func formatEvent(e event) string {
	var p struct {
		Role   string `json:"role"`
		CLI    string `json:"cli"`
		Action string `json:"action"`
		Branch string `json:"branch"`
	}
	_ = json.Unmarshal([]byte(e.Payload), &p)
	switch e.Type {
	case "agent_spawned":
		if extra := joinSlash(p.Role, p.CLI); extra != "" {
			return "🤖 agent spawned: " + e.AgentID + " (" + extra + ")"
		}
		return "🤖 agent spawned: " + e.AgentID
	case "agent_killed":
		return "💀 agent killed: " + e.AgentID
	case "task_assigned":
		if e.AgentID != "" {
			return "📌 task assigned: " + e.TaskID + " → " + e.AgentID
		}
		return "📌 task assigned: " + e.TaskID
	case "human_approval_required":
		msg := "✋ human approval required: " + e.TaskID
		if extra := joinSlash(p.Action, p.Branch); extra != "" {
			msg += " (" + extra + ")"
		}
		return msg
	case "agent_state":
		// Only exit transitions reach the operator. The SQL filter
		// already enforces this; the guard keeps formatEvent total.
		if strings.Contains(e.Payload, "exited") {
			return "🏁 agent exited: " + e.AgentID
		}
	}
	return ""
}

// joinSlash joins the non-empty parts with "/" (e.g. "role/cli").
func joinSlash(parts ...string) string {
	var keep []string
	for _, p := range parts {
		if p != "" {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "/")
}

// ----------------- sinks ---------------------------------------------------

// deliver pushes text to every configured sink. Best-effort: failures
// are swallowed so a dead webhook can never stall or crash the loops.
func (b *Bridge) deliver(ctx context.Context, text string) {
	if b.cfg.telegramConfigured() {
		b.sendTelegram(ctx, text)
	}
	if b.cfg.DiscordWebhookURL != "" {
		b.sendDiscord(ctx, text)
	}
}

// sendDiscord posts {content} to the Discord webhook.
func (b *Bridge) sendDiscord(ctx context.Context, text string) {
	_ = b.postJSON(ctx, b.cfg.DiscordWebhookURL, map[string]any{
		"content": clip(text, discordMaxLen),
	})
}

// ----------------- HTTP helpers --------------------------------------------

// postJSON POSTs v as JSON to endpoint and discards the response body.
func (b *Bridge) postJSON(ctx context.Context, endpoint string, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("POST %s: status %d", endpoint, resp.StatusCode)
	}
	return nil
}

// getJSON GETs endpoint and decodes the JSON response into out.
func (b *Bridge) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("GET %s: status %d", endpoint, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// clip truncates s to at most n bytes without splitting a rune.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}
