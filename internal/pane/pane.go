// Package pane manages worker PTY instances.
//
// Each pane wraps:
//   - a PTY (charmbracelet/x/xpty) running the worker CLI
//   - a scrollback buffer (last N lines, ANSI)
//   - a state detector (V1 heuristic: output activity means the worker
//     is working; a short quiet period means it is idle)
//
// The multiplexer exposes:
//
//   - Spawn(spec)        -> pane
//   - Send(pane_id, b)   // stdin write
//   - Read(pane_id, n)   // last N lines
//   - Kill(pane_id)      // close the PTY
package pane

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

	// idleAfter is how long a pane must stay silent before the V1 state
	// heuristic flips it from "working" to "idle".
	idleAfter time.Duration

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

	// stateTimer flips the pane to "idle" after a quiet period.
	stateTimer *time.Timer

	// subs receive live output/state frames (SSE attach). Sends are
	// non-blocking: a slow consumer drops frames and backfills via /read.
	subs map[chan StreamEvent]struct{}
}

// StreamEvent is one frame delivered to pane stream subscribers.
type StreamEvent struct {
	Type  string // "output" | "state"
	Data  []byte // output chunk (Type == "output")
	State string // new state (Type == "state")
}

// NewManager creates a pane manager. It does not spawn anything yet.
func NewManager(db *sql.DB, b *bus.Bus, cfg config.PaneCfg) *Manager {
	return &Manager{
		db:                db,
		bus:               b,
		cfg:               cfg,
		panes:             make(map[string]*Pane),
		initialInputDelay: defaultInitialInputDelay,
		idleAfter:         defaultIdleAfter,
	}
}

// Close kills all panes.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, p := range m.panes {
		p.mu.Lock()
		if p.stateTimer != nil {
			p.stateTimer.Stop()
			p.stateTimer = nil
		}
		if p.persistTimer != nil {
			p.persistTimer.Stop()
			p.persistTimer = nil
		}
		p.mu.Unlock()
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
	// MCPConfig, when non-empty, is written to <workdir>/.mcp.json before
	// the worker starts, so CLIs that read it pick up the bismuth-team MCP
	// server. Best-effort: a write failure does not abort the spawn.
	MCPConfig []byte
}

// Spawn starts a worker process and registers it.
func (m *Manager) Spawn(ctx context.Context, spec SpawnSpec) (*Pane, error) {
	if len(spec.Cmd) == 0 {
		return nil, fmt.Errorf("cmd must not be empty")
	}

	workdir := spec.Workdir
	if workdir == "" {
		workdir = m.cfg.Workdir
	}

	// MCP installer: drop the team MCP config where the worker CLI will
	// look for it. Best-effort — the worker is still useful without it.
	if len(spec.MCPConfig) > 0 && workdir != "" {
		_ = os.WriteFile(filepath.Join(workdir, ".mcp.json"), spec.MCPConfig, 0o644)
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

	// Persist + publish before the I/O goroutines start: state
	// transitions (working/idle/exited) UPDATE this row and must never
	// race with — or be overwritten by — the initial INSERT.
	_, _ = m.db.ExecContext(ctx,
		`INSERT INTO panes(id, agent_id, last_state, last_state_at) VALUES(?,?,?,?)`,
		pane.ID, spec.AgentID, pane.lastState, time.Now().UTC().Format(time.RFC3339))
	_ = m.bus.Publish(ctx, bus.Event{
		Type:    "pane_spawned",
		AgentID: spec.AgentID,
		Payload: shared.JSONRaw(map[string]any{"pane_id": pane.ID, "cli": spec.CLI, "role": spec.Role, "cmd": spec.Cmd}),
		TS:      time.Now().UTC().Format(time.RFC3339),
	})

	go m.readLoop(pane, spec.AgentID)

	// The PTY master does not see EOF while we hold the slave side open,
	// so child exit is detected by reaping the process directly.
	go m.watchExit(pane, spec.AgentID, c)

	// Deliver the task as the worker's first input.
	if len(spec.InitialInput) > 0 {
		go m.deliverInitialInput(pane, spec.InitialInput)
	}

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

// Attach subscribes to a pane's live output/state stream. The returned
// cancel func must be called to detach. The channel is never closed by
// the manager; consumers stop on cancel/ctx instead.
func (m *Manager) Attach(paneID string) (<-chan StreamEvent, func(), error) {
	m.mu.RLock()
	p, ok := m.panes[paneID]
	m.mu.RUnlock()
	if !ok {
		return nil, nil, fmt.Errorf("pane %s not found", paneID)
	}
	ch := make(chan StreamEvent, 256)
	p.mu.Lock()
	if p.subs == nil {
		p.subs = make(map[chan StreamEvent]struct{})
	}
	p.subs[ch] = struct{}{}
	p.mu.Unlock()
	cancel := func() {
		p.mu.Lock()
		delete(p.subs, ch)
		p.mu.Unlock()
	}
	return ch, cancel, nil
}

// broadcast fans a frame out to all subscribers without blocking.
func (p *Pane) broadcast(ev StreamEvent) {
	p.mu.Lock()
	for ch := range p.subs {
		select {
		case ch <- ev:
		default: // slow consumer: drop, /read backfills
		}
	}
	p.mu.Unlock()
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

// Read returns the last n lines of scrollback (all of it when n <= 0).
func (m *Manager) Read(paneID string, n int) ([]byte, error) {
	m.mu.RLock()
	p, ok := m.panes[paneID]
	m.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("pane %s not found", paneID)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return LastLines(p.scrollback, n), nil
}

// LastLines returns the last n lines of b. Splitting happens on '\n'
// only, so ANSI escape sequences (which never contain a newline) are
// kept intact within their line. n <= 0 returns b unchanged.
func LastLines(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return b
	}
	// Ignore a trailing newline so "a\nb\n" counts as two lines.
	end := len(b)
	if b[end-1] == '\n' {
		end--
	}
	seen := 0
	for i := end - 1; i >= 0; i-- {
		if b[i] == '\n' {
			seen++
			if seen == n {
				return b[i+1:]
			}
		}
	}
	return b
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
	p.mu.Lock()
	if p.stateTimer != nil {
		p.stateTimer.Stop()
		p.stateTimer = nil
	}
	p.mu.Unlock()
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

			// Live stream fanout. chunk aliases buf (reused on the next
			// Read), so subscribers get their own copy.
			p.broadcast(StreamEvent{Type: "output", Data: append([]byte(nil), chunk...)})

			// V1 state heuristic: output means working; quiet means idle.
			m.markState(p, agentID, "working")
			m.armIdleTimer(p, agentID)
		}
		if err != nil {
			// Process exited or PTY closed.
			p.mu.Lock()
			if p.stateTimer != nil {
				p.stateTimer.Stop()
				p.stateTimer = nil
			}
			p.mu.Unlock()
			m.markState(p, agentID, "exited")
			return
		}
	}
}

// watchExit reaps the worker process and marks the pane exited. This is
// the reliable exit signal: readLoop alone never sees EOF because the
// pane keeps the PTY slave open on the parent side.
func (m *Manager) watchExit(p *Pane, agentID string, c *exec.Cmd) {
	_ = c.Wait()
	p.mu.Lock()
	if p.stateTimer != nil {
		p.stateTimer.Stop()
		p.stateTimer = nil
	}
	p.mu.Unlock()
	m.markState(p, agentID, "exited")
}

// markState records a state transition for the pane (and its agent),
// persisting it and publishing an agent_state event. No-op when the
// state is unchanged; "exited" is terminal.
func (m *Manager) markState(p *Pane, agentID, state string) {
	p.mu.Lock()
	if p.lastState == state || p.lastState == "exited" {
		p.mu.Unlock()
		return
	}
	p.lastState = state
	p.lastStateAt = time.Now()
	p.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = m.db.Exec(`UPDATE panes SET last_state=?, last_state_at=? WHERE id=?`, state, now, p.ID)
	_, _ = m.db.Exec(`UPDATE agents SET state=?, updated_at=? WHERE id=? AND state NOT IN ('killed','failed')`, state, now, agentID)
	_ = m.bus.Publish(context.Background(), bus.Event{
		Type:    "agent_state",
		AgentID: agentID,
		Payload: shared.JSONRaw(map[string]any{"pane_id": p.ID, "state": state}),
		TS:      now,
	})
	p.broadcast(StreamEvent{Type: "state", State: state})
}

// armIdleTimer (re)starts the quiet-period timer that flips the pane to
// idle when no output arrives for idleAfter.
func (m *Manager) armIdleTimer(p *Pane, agentID string) {
	after := m.idleAfter
	if after <= 0 {
		after = defaultIdleAfter
	}
	p.mu.Lock()
	if p.stateTimer != nil {
		p.stateTimer.Stop()
	}
	p.stateTimer = time.AfterFunc(after, func() { m.markState(p, agentID, "idle") })
	p.mu.Unlock()
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
	// defaultIdleAfter is the quiet period after which a pane with no
	// output is considered idle (V1 heuristic).
	defaultIdleAfter = 2 * time.Second
)
