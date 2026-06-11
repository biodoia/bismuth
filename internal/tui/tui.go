// Package tui implements the bismuth terminal UI using bubbletea v1.
//
// Shows a real-time dashboard with:
//   - Left panel: agent list (role, cli, state, cost)
//   - Right panel: event feed (live via polling)
//   - Bottom bar: keybindings
package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- API models ---

type agent struct {
	ID        string  `json:"id"`
	Role      string  `json:"role"`
	CLI       string  `json:"cli"`
	State     string  `json:"state"`
	CostUSD   float64 `json:"cost_usd"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	Model     *string `json:"model"`
	Name      string  `json:"name"`
}

type event struct {
	Seq     int64           `json:"seq"`
	Type    string          `json:"type"`
	AgentID string          `json:"agent_id"`
	Payload json.RawMessage `json:"payload"`
	TS      string          `json:"ts"`
}

// --- tea messages ---

type agentsMsg struct{ agents []agent }
type eventsMsg struct{ events []event }
type errMsg struct{ err error }
type tickMsg time.Time

// --- model ---

type model struct {
	baseURL string
	agents  []agent
	events  []event
	cursor  int
	err     error
	width   int
	height  int
}

func newModel(baseURL string) model {
	return model{baseURL: strings.TrimRight(baseURL, "/")}
}

// --- styles ---

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#22c55e")).
			Background(lipgloss.Color("#09090b")).
			Padding(0, 1)
	agentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#a1a1aa"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e")).Bold(true)
	barStyle      = lipgloss.NewStyle().
			Background(lipgloss.Color("#18181b")).
			Foreground(lipgloss.Color("#a1a1aa")).
			Padding(0, 1)
	eventStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
)

var stateColors = map[string]lipgloss.Color{
	"idle":        "#a1a1aa",
	"running":     "#22c55e",
	"killed":      "#ef4444",
	"paused_cost": "#f59e0b",
}

// --- bubbletea interface ---

func (m model) Init() tea.Cmd {
	return tea.Batch(fetchAgents(m.baseURL), fetchEvents(m.baseURL), tick())
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			return m, tea.Batch(fetchAgents(m.baseURL), fetchEvents(m.baseURL))
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.agents)-1 {
				m.cursor++
			}
		}

	case tickMsg:
		return m, tea.Batch(fetchAgents(m.baseURL), fetchEvents(m.baseURL), tick())

	case agentsMsg:
		m.agents = msg.agents
		if m.cursor >= len(m.agents) {
			m.cursor = max(0, len(m.agents)-1)
		}
		m.err = nil

	case eventsMsg:
		m.events = msg.events
		m.err = nil

	case errMsg:
		m.err = msg.err
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 {
		return "loading..."
	}

	w := m.width

	title := titleStyle.Render("bismuth")
	bar := barStyle.Render(fmt.Sprintf("%s  [q]uit  [r]efresh  [up/down]  agents:%d  events:%d",
		title, len(m.agents), len(m.events)))

	agents := m.renderAgents(w * 2 / 5)
	events := m.renderEvents(w*3/5-3, m.height-4)

	main := lipgloss.JoinHorizontal(lipgloss.Top, agents, " | ", events)

	if m.err != nil {
		main += fmt.Sprintf("\nerr: %v", m.err)
	}

	return bar + "\n" + main + "\n" + strings.Repeat("─", w)
}

func (m model) renderAgents(w int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Agents\n"))

	if len(m.agents) == 0 {
		b.WriteString("  (no agents)\n")
	}

	for i, a := range m.agents {
		st := agentStyle
		prefix := "  "
		if i == m.cursor {
			st = selectedStyle
			prefix = "> "
		}

		sc := "#a1a1aa"
		if c, ok := stateColors[a.State]; ok {
			sc = string(c)
		}
		stateStr := lipgloss.NewStyle().Foreground(lipgloss.Color(sc)).Render(a.State)

		line := fmt.Sprintf("%s%-12s %-6s %s $%.2f", prefix, a.Role, a.CLI, stateStr, a.CostUSD)
		b.WriteString(st.Width(w).Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

func (m model) renderEvents(w, h int) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Events\n"))

	max := h - 2
	if max > len(m.events) {
		max = len(m.events)
	}
	if max > 20 {
		max = 20
	}

	for i := 0; i < max; i++ {
		e := m.events[i]
		ts := ""
		if len(e.TS) > 19 {
			ts = e.TS[11:19]
		}
		line := fmt.Sprintf("  #%4d %-16s %s %-8s", e.Seq, e.Type, ts, shortID(e.AgentID))
		b.WriteString(eventStyle.Width(w).Render(line))
		b.WriteString("\n")
	}

	return b.String()
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// --- commands ---

func tick() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func fetchAgents(baseURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(baseURL + "/api/v1/agents")
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()
		var result struct {
			Agents []agent `json:"agents"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return errMsg{err}
		}
		return agentsMsg{result.Agents}
	}
}

func fetchEvents(baseURL string) tea.Cmd {
	return func() tea.Msg {
		resp, err := http.Get(baseURL + "/api/v1/events?limit=50")
		if err != nil {
			return errMsg{err}
		}
		defer resp.Body.Close()
		var result struct {
			Events []event `json:"events"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return errMsg{err}
		}
		return eventsMsg{result.Events}
	}
}

// Run starts the TUI.
func Run(baseURL string) error {
	m := newModel(baseURL)
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
