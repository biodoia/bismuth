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
	"os"
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
	db  *sql.DB
	bus *bus.Bus
	cfg config.PaneCfg

	// initialInputDelay is how long to wait after a worker starts before
	// typing its task/prompt, giving the CLI time to bring up its input
	// loop. Set to 0 in tests for determinism.
	initialInputDelay time.Duration

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

	// Coalesced persistence: avoid DB churn on high-frequency output.
	// Flush to DB when pending >= persistAfter bytes OR persistEvery
	// has elapsed since the first chunk in the current batch.
	pending      []byte
	persistBytes int
	persistAfter int           // default 256
	persistEvery time.Duration // default 500ms
	persistTimer *time.Timer
}

// NewManager creates a pane manager. It does not spawn anything yet.
func NewManager(db *sql.DB, b *bus.Bus, cfg config.PaneCfg) *Manager {
	return &Manager{
		db:                db,
		bus:               b,
		cfg:               cfg,
		panes:             make(map[string]*Pane),
		initialInputDelay: defaultInitialInputDelay,
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

// SpawnSpec describes a worker process to launch.
type SpawnSpec struct {
	AgentID string
	// PaneID must match the pane_id stored on the agent so that /read,
	// /send and /kill resolve the same pane. Generated if empty.
	PaneID string
	CLI    string
	Role   string
	// Workdir is where the worker runs — normally the agent's git
	// worktree. Falls back to the manager's configured workdir if empty.
	Workdir string
	// Cmd[0] is the binary (e.g. "omx"), Cmd[1:] are the arguments.
	Cmd []string
	// Env holds per-agent vars (BISMUTH_*, provider API keys). They are
	// layered on top of the inherited process environment, not replacing it.
	Env []string
	// InitialInput is the task/prompt delivered to the worker's stdin once
	// it starts, as if a human typed it and pressed Enter.
	InitialInput []byte
}

// Spawn starts a worker process and registers it.
//
// TODO(sessione+1): wire to MCP installer (write .mcp.json in workdir
// before spawn), push state events.
func (m *Manager) Spawn(ctx context.Context, spec SpawnSpec) (*Pane, error) {
	if len(spec.Cmd) == 0 {
		return nil, fmt.Errorf("cmd must not be empty")
	}

	workdir := spec.Workdir
	if workdir == "" {
		workdir = m.cfg.Workdir
	}

	p, err := xpty.NewPty(120, 40)
	if err != nil {
		return nil, fmt.Errorf("new pty: %w", err)
	}
	c := exec.Command(spec.Cmd[0], spec.Cmd[1:]...)
	c.Dir = workdir
	// Inherit the server's environment (PATH, HOME, ...) so the worker CLI
	// and any subprocess it launches can resolve binaries and config; then
	// layer the per-agent vars on top (later entries win).
	c.Env = append(os.Environ(), spec.Env...)
	if err := p.Start(c); err != nil {
		_ = p.Close() // don't leak the PTY's file descriptors
		return nil, fmt.Errorf("start %s: %w", spec.Cmd[0], err)
	}

	paneID := spec.PaneID
	if paneID == "" {
		paneID = newPaneID()
	}
	pane := &Pane{
		ID:        paneID,
		AgentID:   spec.AgentID,
		Shell:     spec.Cmd[0],
		Cmd:       spec.Cmd,
		Workdir:   workdir,
		Started:   time.Now(),
		PTY:       p,
		scrollMax: 5000,
		lastState: "working",
	}

	m.mu.Lock()
	m.panes[pane.ID] = pane
	m.mu.Unlock()

	go m.readLoop(pane, spec.AgentID)

	// Deliver the task as the worker's first input.
	if len(spec.InitialInput) > 0 {
		go m.deliverInitialInput(pane, spec.InitialInput)
	}

	// Persist + publish
	_, _ = m.db.ExecContext(ctx,
		`INSERT INTO panes(id, agent_id, last_state, last_state_at) VALUES(?,?,?,?)`,
		pane.ID, spec.AgentID, pane.lastState, time.Now().UTC().Format(time.RFC3339))
	_ = m.bus.Publish(ctx, bus.Event{
		Type:    "pane_spawned",
		AgentID: spec.AgentID,
		Payload: shared.JSONRaw(map[string]any{"pane_id": pane.ID, "cli": spec.CLI, "role": spec.Role, "cmd": spec.Cmd}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})
	return pane, nil
}

// deliverInitialInput types the task/prompt into the worker's PTY once it
// has had a moment to start its input loop, then sends Enter (CR).
func (m *Manager) deliverInitialInput(p *Pane, input []byte) {
	if m.initialInputDelay > 0 {
		time.Sleep(m.initialInputDelay)
	}
	payload := make([]byte, 0, len(input)+1)
	payload = append(payload, input...)
	payload = append(payload, '\r')
	_, _ = p.PTY.Write(payload)
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

// readLoop pumps PTY output into both:
//   - in-memory scrollback buffer (for fast /read)
//   - persistent DB blob (for /read and crash recovery)
//   - bus events (for /ws realtime feed)
func (m *Manager) readLoop(p *Pane, agentID string) {
	buf := make([]byte, 4096)
	for {
		n, err := p.PTY.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			p.mu.Lock()
			p.scrollback = append(p.scrollback, chunk...)
			if len(p.scrollback) > p.scrollMax*200 {
				p.scrollback = p.scrollback[len(p.scrollback)-p.scrollMax*100:]
			}
			p.mu.Unlock()

			// persist (best-effort) + publish
			_ = m.persistChunk(p, chunk)
			_ = m.bus.Publish(context.Background(), bus.Event{
				Type:    "pane_output",
				AgentID: agentID,
				Payload: shared.JSONRaw(map[string]any{"pane_id": p.ID, "bytes": len(chunk)}),
				TS:      time.Now().UTC().Format(time.RFC3339),
			})
		}
		if err != nil {
			return
		}
	}
}

func (m *Manager) persistChunk(p *Pane, chunk []byte) error {
	// Coalesced persistence: only write to DB when we've buffered
	// persistAfter bytes OR persistEvery has elapsed. Avoids DB churn
	// for high-frequency output (e.g. tail -f).
	p.mu.Lock()
	p.pending = append(p.pending, chunk...)
	p.persistBytes += len(chunk)
	curBytes := p.persistBytes
	threshold := p.persistAfter
	timer := p.persistTimer
	every := p.persistEvery
	p.mu.Unlock()

	if threshold <= 0 {
		threshold = defaultPersistAfter
	}
	if every <= 0 {
		every = defaultPersistEvery
	}

	flush := func() {
		p.mu.Lock()
		toFlush := p.pending
		p.pending = nil
		p.persistBytes = 0
		if p.persistTimer != nil {
			p.persistTimer.Stop()
			p.persistTimer = nil
		}
		p.mu.Unlock()
		if len(toFlush) == 0 {
			return
		}
		_ = m.writeScrollback(p.ID, toFlush)
	}

	if curBytes >= threshold {
		flush()
		return nil
	}
	if timer == nil {
		p.mu.Lock()
		// double-check inside the lock
		if p.persistTimer == nil {
			p.persistTimer = time.AfterFunc(every, flush)
		}
		p.mu.Unlock()
	}
	return nil
}

// writeScrollback does the actual DB write with truncation.
func (m *Manager) writeScrollback(paneID string, chunk []byte) error {
	const maxBlob = 200 * 1024
	row := m.db.QueryRow(`SELECT scrollback FROM panes WHERE id = ?`, paneID)
	var cur sql.NullString
	if err := row.Scan(&cur); err != nil {
		return err
	}
	combined := cur.String + string(chunk)
	if len(combined) > maxBlob {
		combined = combined[len(combined)-maxBlob:]
	}
	_, err := m.db.Exec(`UPDATE panes SET scrollback = ?, last_state_at = ? WHERE id = ?`,
		combined, time.Now().UTC().Format(time.RFC3339), paneID)
	return err
}

func newPaneID() string {
	// simple timestamp-based id; replace with ULID in V1 implementation
	return fmt.Sprintf("pane-%d", time.Now().UnixNano())
}

const (
	defaultPersistAfter = 256 // bytes
	defaultPersistEvery = 500 * time.Millisecond
	// defaultInitialInputDelay gives an interactive worker CLI time to
	// bring up its input loop before we type the task prompt into it.
	defaultInitialInputDelay = 500 * time.Millisecond
)
