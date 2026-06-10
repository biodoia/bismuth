// Package bridge — telegram.go
//
// Telegram sink (sendMessage) and the chat command loop (getUpdates
// long polling). Commands are the operator's remote control:
//
//	/status            -- agents + tasks summary
//	/agents            -- list agent ids, roles, states
//	/tasks             -- list task ids, titles, status
//	/kill <agent_id>   -- POST /api/v1/agents/<id>/kill
//
// Anything else gets a short help text.
package bridge

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// tgUpdate mirrors the subset of the Telegram Update object we use.
type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	Text string `json:"text"`
	Chat tgChat `json:"chat"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

// sendTelegram delivers text to the configured chat. Best-effort.
func (b *Bridge) sendTelegram(ctx context.Context, text string) {
	endpoint := b.cfg.TelegramAPIBase + "/bot" + b.cfg.TelegramToken + "/sendMessage"
	_ = b.postJSON(ctx, endpoint, map[string]any{
		"chat_id": b.cfg.TelegramChatID,
		"text":    clip(text, telegramMaxLen),
	})
}

// commandLoop long-polls getUpdates and answers chat commands until ctx
// is cancelled. Errors and empty polls back off for PollInterval so a
// broken network (or an instantly-answering test fake) cannot hot-loop.
func (b *Bridge) commandLoop(ctx context.Context) {
	var offset int64
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		updates, err := b.getUpdates(ctx, offset)
		for _, u := range updates {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message == nil || !b.fromConfiguredChat(u.Message.Chat) {
				continue
			}
			if reply := b.handleCommand(ctx, u.Message.Text); reply != "" {
				b.sendTelegram(ctx, reply)
			}
		}
		if err != nil || len(updates) == 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(b.cfg.PollInterval):
			}
		}
	}
}

// getUpdates performs one long poll (held up to 10s server-side).
func (b *Bridge) getUpdates(ctx context.Context, offset int64) ([]tgUpdate, error) {
	endpoint := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=10",
		b.cfg.TelegramAPIBase, b.cfg.TelegramToken, offset)
	var resp struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := b.getJSON(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("getUpdates: ok=false")
	}
	return resp.Result, nil
}

// fromConfiguredChat accepts only the operator chat.
func (b *Bridge) fromConfiguredChat(c tgChat) bool {
	return strconv.FormatInt(c.ID, 10) == b.cfg.TelegramChatID
}

// ----------------- commands ------------------------------------------------

const helpText = `commands:
/status - agents and tasks summary
/agents - list agents
/tasks - list tasks
/kill <agent_id> - kill an agent`

// handleCommand dispatches one chat message and returns the reply.
func (b *Bridge) handleCommand(ctx context.Context, text string) string {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return helpText
	}
	cmd := fields[0]
	// Telegram appends "@botname" to commands in group chats.
	if i := strings.IndexByte(cmd, '@'); i > 0 {
		cmd = cmd[:i]
	}
	switch cmd {
	case "/status":
		return b.cmdStatus(ctx)
	case "/agents":
		return b.cmdAgents(ctx)
	case "/tasks":
		return b.cmdTasks(ctx)
	case "/kill":
		if len(fields) < 2 {
			return "usage: /kill <agent_id>"
		}
		return b.cmdKill(ctx, fields[1])
	}
	return helpText
}

// apiAgent / apiTask mirror the control-API response fields the bridge
// renders; everything else in the payload is ignored.
type apiAgent struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	State string `json:"state"`
}

type apiTask struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

func (b *Bridge) fetchAgents(ctx context.Context) ([]apiAgent, error) {
	var resp struct {
		Agents []apiAgent `json:"agents"`
	}
	err := b.getJSON(ctx, b.cfg.APIBase+"/api/v1/agents", &resp)
	return resp.Agents, err
}

func (b *Bridge) fetchTasks(ctx context.Context) ([]apiTask, error) {
	var resp struct {
		Tasks []apiTask `json:"tasks"`
	}
	err := b.getJSON(ctx, b.cfg.APIBase+"/api/v1/tasks", &resp)
	return resp.Tasks, err
}

func (b *Bridge) cmdStatus(ctx context.Context) string {
	agents, errA := b.fetchAgents(ctx)
	tasks, errT := b.fetchTasks(ctx)
	if errA != nil || errT != nil {
		return "error: control API unreachable"
	}
	return agentLines(agents) + "\n" + taskLines(tasks)
}

func (b *Bridge) cmdAgents(ctx context.Context) string {
	agents, err := b.fetchAgents(ctx)
	if err != nil {
		return "error: " + err.Error()
	}
	return agentLines(agents)
}

func (b *Bridge) cmdTasks(ctx context.Context) string {
	tasks, err := b.fetchTasks(ctx)
	if err != nil {
		return "error: " + err.Error()
	}
	return taskLines(tasks)
}

func (b *Bridge) cmdKill(ctx context.Context, agentID string) string {
	endpoint := b.cfg.APIBase + "/api/v1/agents/" + url.PathEscape(agentID) + "/kill"
	if err := b.postJSON(ctx, endpoint, map[string]any{}); err != nil {
		return "error: kill " + agentID + ": " + err.Error()
	}
	return "ok: killed " + agentID
}

// agentLines renders "agents: N" plus the first maxListLines entries.
func agentLines(agents []apiAgent) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "agents: %d", len(agents))
	for i, a := range agents {
		if i == maxListLines {
			fmt.Fprintf(&sb, "\n… %d more", len(agents)-maxListLines)
			break
		}
		fmt.Fprintf(&sb, "\n- %s [%s] %s", a.ID, a.Role, a.State)
	}
	return sb.String()
}

// taskLines renders "tasks: N" plus the first maxListLines entries.
func taskLines(tasks []apiTask) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "tasks: %d", len(tasks))
	for i, t := range tasks {
		if i == maxListLines {
			fmt.Fprintf(&sb, "\n… %d more", len(tasks)-maxListLines)
			break
		}
		fmt.Fprintf(&sb, "\n- %s [%s] %s", t.ID, t.Status, t.Title)
	}
	return sb.String()
}
