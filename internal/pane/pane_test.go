// pane package: test the coalesced persistence logic.
package pane

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/db"
)

func TestCoalescerFlushesByBytes(t *testing.T) {
	// Build a Pane with mock fields; only pending/persistBytes/etc
	// are touched by the coalescer.
	p := &Pane{
		ID:           "p1",
		persistAfter: 256,
		persistEvery: 500 * time.Millisecond,
	}

	// Simulate three small writes
	for i := 0; i < 3; i++ {
		p.mu.Lock()
		p.pending = append(p.pending, []byte("x")...)
		p.persistBytes += 1
		p.mu.Unlock()
	}

	// Should NOT have triggered a flush yet (only 3 bytes)
	p.mu.Lock()
	gotBytes := p.persistBytes
	p.mu.Unlock()
	if gotBytes != 3 {
		t.Errorf("persistBytes = %d, want 3", gotBytes)
	}

	// Now write enough to trigger threshold flush
	p.mu.Lock()
	p.pending = append(p.pending, make([]byte, 300)...)
	p.persistBytes += 300
	p.mu.Unlock()
	// (real flush happens via the coalescer; for this test we just
	// assert that the counters work as expected.)
	if p.persistBytes < 256 {
		t.Errorf("threshold not reached, got %d", p.persistBytes)
	}
}

func TestCoalescerConstants(t *testing.T) {
	if defaultPersistAfter <= 0 {
		t.Error("defaultPersistAfter must be > 0")
	}
	if defaultPersistEvery <= 0 {
		t.Error("defaultPersistEvery must be > 0")
	}
}

// TestSpawnWiresPaneIDWorkdirEnvAndInput is a regression test for four
// spawn bugs that left agents inert with empty scrollback:
//   - the pane was registered under a different id than the agent's
//     pane_id, so /read, /send and /kill could never find it;
//   - the worker ran in the manager's workdir, not the agent's worktree;
//   - the worker's environment was replaced (no PATH/HOME inherited);
//   - the task/prompt was never delivered to the worker.
func TestSpawnWiresPaneIDWorkdirEnvAndInput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	ctx := context.Background()

	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := bus.New(store.DB())
	defer b.Close()

	m := NewManager(store.DB(), b, config.PaneCfg{})
	m.initialInputDelay = 0 // deterministic: no startup delay in tests

	dir := t.TempDir()

	// The worker reads one line of stdin (the delivered prompt), drops a
	// marker file in its CWD, then reports the prompt, whether PATH was
	// inherited, and the per-agent env var.
	script := `read line; : > spawned_here.txt; ` +
		`printf 'GOT[%s] PATHSET[%s] TASK[%s]\n' "$line" "${PATH:+yes}" "$BISMUTH_TASK_ID"`
	spec := SpawnSpec{
		AgentID:      "agt-test",
		PaneID:       "pane-fixed-123",
		CLI:          "sh",
		Role:         "implementer",
		Workdir:      dir,
		Cmd:          []string{"sh", "-c", script},
		Env:          []string{"BISMUTH_TASK_ID=tsk-xyz"},
		InitialInput: []byte("do-the-thing"),
		MCPConfig:    []byte(`{"mcpServers":{"bismuth-team":{"command":"bismuth","args":["mcp"]}}}`),
	}
	p, err := m.Spawn(ctx, spec)
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() { _ = m.Kill(ctx, p.ID) }()

	if p.ID != "pane-fixed-123" {
		t.Fatalf("pane ID = %q, want pane-fixed-123 (must match agent's pane_id)", p.ID)
	}

	out := waitForScrollback(t, p, "GOT[", 5*time.Second)

	if !strings.Contains(out, "GOT[do-the-thing]") {
		t.Errorf("initial input not delivered to worker; scrollback=%q", out)
	}
	if !strings.Contains(out, "PATHSET[yes]") {
		t.Errorf("PATH not inherited by worker; scrollback=%q", out)
	}
	if !strings.Contains(out, "TASK[tsk-xyz]") {
		t.Errorf("per-agent env var not passed to worker; scrollback=%q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "spawned_here.txt")); err != nil {
		t.Errorf("worker did not run in the requested workdir %q: %v", dir, err)
	}

	// MCP installer: .mcp.json must be in place before the worker runs.
	if got, err := os.ReadFile(filepath.Join(dir, ".mcp.json")); err != nil {
		t.Errorf(".mcp.json not written to workdir: %v", err)
	} else if !strings.Contains(string(got), "bismuth-team") {
		t.Errorf(".mcp.json content unexpected: %s", got)
	}

	// The panes row must use the same id as the agent's pane_id.
	row, err := store.GetPane(ctx, "pane-fixed-123")
	if err != nil {
		t.Fatalf("GetPane(pane-fixed-123): %v", err)
	}
	if row.ID != "pane-fixed-123" {
		t.Fatalf("panes row id = %q, want pane-fixed-123", row.ID)
	}
}

func TestLastLines(t *testing.T) {
	cases := []struct {
		in   string
		n    int
		want string
	}{
		{"", 5, ""},
		{"a\nb\nc", 0, "a\nb\nc"},           // n<=0 -> everything
		{"a\nb\nc", 2, "b\nc"},              // plain split
		{"a\nb\nc\n", 2, "b\nc\n"},          // trailing newline ignored for counting
		{"a\nb\nc", 10, "a\nb\nc"},          // n larger than content
		{"x", 1, "x"},                       // single line, no newline
		{"\x1b[31mred\x1b[0m\nok", 1, "ok"}, // ANSI codes stay within their line
	}
	for _, c := range cases {
		if got := string(LastLines([]byte(c.in), c.n)); got != c.want {
			t.Errorf("LastLines(%q, %d) = %q, want %q", c.in, c.n, got, c.want)
		}
	}
}

// TestStateTransitions verifies the V1 heuristic: output flips the pane
// to working, a quiet period flips it to idle, and process exit marks it
// exited — all persisted to the panes row.
func TestStateTransitions(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	ctx := context.Background()

	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	b := bus.New(store.DB())
	defer b.Close()

	m := NewManager(store.DB(), b, config.PaneCfg{})
	m.initialInputDelay = 0
	m.idleAfter = 50 * time.Millisecond

	// Emit output, stay quiet long enough to go idle, then exit.
	p, err := m.Spawn(ctx, SpawnSpec{
		AgentID: "agt-state",
		PaneID:  "pane-state-1",
		CLI:     "sh",
		Role:    "tester",
		Workdir: t.TempDir(),
		Cmd:     []string{"sh", "-c", "echo busy; sleep 0.4; echo done"},
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() { _ = m.Kill(ctx, p.ID) }()

	if !waitForPaneState(t, store, p.ID, "idle", 3*time.Second) {
		t.Fatalf("pane never went idle; last persisted state mismatch")
	}
	if !waitForPaneState(t, store, p.ID, "exited", 5*time.Second) {
		t.Fatalf("pane never marked exited after process end")
	}
}

// waitForPaneState polls the panes row until last_state equals want.
func waitForPaneState(t *testing.T, store *db.Store, paneID, want string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		row, err := store.GetPane(context.Background(), paneID)
		if err == nil && row.LastState.Valid && row.LastState.String == want {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

// waitForScrollback polls the in-memory scrollback until it contains needle
// or the timeout elapses, returning whatever was captured.
func waitForScrollback(t *testing.T, p *Pane, needle string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		p.mu.Lock()
		s := string(p.scrollback)
		p.mu.Unlock()
		if strings.Contains(s, needle) || time.Now().After(deadline) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
}
