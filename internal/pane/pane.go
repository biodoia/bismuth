// Package pane manages worker PTY instances.
//
// Each pane wraps:
//   - a PTY (charmbracelet/x/xpty) running the worker CLI
//   - a scrollback buffer (last N lines, ANSI)
//   - a state detector (delegated to herdr/HERDR semantics in V2; V1
//     uses a simple prompt-based heuristic: if the CLI shows a known
//     "working" indicator, state=working; if it shows the prompt, state=idle)
//
// The multiplexer exposes:
//
//   - spawn(agentID, cli, role, args...) -> pane_id
//   - send(pane_id, bytes)               // stdin write
//   - read(pane_id, n)                   // last N lines
//   - kill(pane_id)                      // SIGTERM, escalate SIGKILL
//   - attach(pane_id, ws)                // stream I/O via WebSocket
package pane

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/shared"
	"github.com/charmbracelet/x/xpty"
)

// Manager owns all live panes.
type Manager struct {
	db   *sql.DB
	bus  *bus.Bus
	cfg  config.PaneCfg

	mu    sync.RWMutex
	panes map[string]*Pane
}

// Pane is a single worker process with its PTY.
type Pane struct {
	ID       string
	AgentID  string
	Shell    string
	Cmd      []string
	Workdir  string
	Worktree string
	PTY      xpty.Pty
	Started  time.Time

	mu          sync.Mutex
	scrollback  []byte // last N lines, ANSI
	scrollMax   int
	lastState   string
	lastStateAt time.Time
}

// NewManager creates a pane manager. It does not spawn anything yet.
func NewManager(db *sql.DB, b *bus.Bus, cfg config.PaneCfg) *Manager {
	return &Manager{
		db:    db,
		bus:   b,
		cfg:   cfg,
		panes: make(map[string]*Pane),
	}
}

// Close kills all panes.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.panes {
		_ = p.PTY.Close()
		delete(m.panes, id)
	}
}

// Spawn starts a worker process and registers it. cmd[0] is the binary
// (e.g. "omx"), cmd[1:] are the arguments.
//
// TODO(sessione+1): wire to MCP installer (write .mcp.json in workdir
// before spawn), set up worktree, push state events.
func (m *Manager) Spawn(ctx context.Context, agentID, cli string, role string, cmd []string, env []string) (*Pane, error) {
	if len(cmd) == 0 {
		return nil, fmt.Errorf("cmd must not be empty")
	}

	p, err := xpty.NewPty(120, 40)
	if err != nil {
		return nil, fmt.Errorf("new pty: %w", err)
	}
	args := cmd[1:]
	c := exec.Command(cmd[0], args...)
	c.Dir = m.cfg.Workdir
	if env != nil {
		c.Env = append(c.Env, env...)
	}
	if err := p.Start(c); err != nil {
		return nil, fmt.Errorf("start %s: %w", cmd[0], err)
	}

	pane := &Pane{
		ID:        newPaneID(),
		AgentID:   agentID,
		Shell:     cmd[0],
		Cmd:       cmd,
		Workdir:   m.cfg.Workdir,
		Started:   time.Now(),
		PTY:       p,
		scrollMax: 5000,
		lastState: "working",
	}

	m.mu.Lock()
	m.panes[pane.ID] = pane
	m.mu.Unlock()

	go m.readLoop(pane)

	// Persist + publish
	_, _ = m.db.ExecContext(ctx,
		`INSERT INTO panes(id, agent_id, last_state, last_state_at) VALUES(?,?,?,?)`,
		pane.ID, agentID, pane.lastState, time.Now().UTC().Format(time.RFC3339))
	_ = m.bus.Publish(ctx, bus.Event{
		Type:    "pane_spawned",
		AgentID: agentID,
		Payload: shared.JSONRaw(map[string]any{"pane_id": pane.ID, "cli": cli, "role": role, "cmd": cmd}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	return pane, nil
}

// Send writes bytes to the pane's PTY stdin.
func (m *Manager) Send(ctx context.Context, paneID string, b []byte) error {
	m.mu.RLock()
	p, ok := m.panes[paneID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("pane %s not found", paneID)
	}
	_, err := p.PTY.Write(b)
	return err
}

// Read returns the last n lines of scrollback.
func (m *Manager) Read(paneID string, n int) ([]byte, error) {
	m.mu.RLock()
	p, ok := m.panes[paneID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("pane %s not found", paneID)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	// TODO(sessione+1): split ANSI-aware, return last n lines.
	return p.scrollback, nil
}

// Kill terminates the pane.
func (m *Manager) Kill(ctx context.Context, paneID string) error {
	m.mu.Lock()
	p, ok := m.panes[paneID]
	if ok {
		delete(m.panes, paneID)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("pane %s not found", paneID)
	}
	_ = p.PTY.Close()
	_ = m.bus.Publish(ctx, bus.Event{
		Type:    "pane_killed",
		AgentID: p.AgentID,
		Payload: shared.JSONRaw(map[string]any{"pane_id": paneID}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	return nil
}

func (m *Manager) readLoop(p *Pane) {
	buf := make([]byte, 4096)
	for {
		n, err := p.PTY.Read(buf)
		if n > 0 {
			p.mu.Lock()
			p.scrollback = append(p.scrollback, buf[:n]...)
			if len(p.scrollback) > p.scrollMax*200 {
				// truncate to last scrollMax bytes
				p.scrollback = p.scrollback[len(p.scrollback)-p.scrollMax*100:]
			}
			p.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func newPaneID() string {
	// simple timestamp-based id; replace with ULID in V1 implementation
	return fmt.Sprintf("pane-%d", time.Now().UnixNano())
}
