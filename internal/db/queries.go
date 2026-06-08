// Package db — queries.go
//
// All CRUD operations on the 7 tables defined in migrations/001_init.sql.
// Every function takes ctx and uses the prepared-statement-per-call pattern
// (SQLite + WAL + single connection makes this fast and safe).
//
// Conventions:
//   - all IDs are caller-provided strings (we use ULIDs in V1.1; for now
//     any unique string works). Helpers NewID() / NewPaneID() / etc.
//   - timestamps are RFC3339 UTC strings, generated server-side
//   - JSON payloads use shared.JSONRaw
//   - functions return typed structs, not sql.Rows
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/biodoia/bismuth/internal/shared"
)

// ----------------- shared helpers -----------------------------------------

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullableFloat(f float64) any { return f }

// ----------------- agents ---------------------------------------------------

type Agent struct {
	ID           string
	Role         string
	Name         string
	CLI          string
	PID          sql.NullInt64
	State        string
	PaneID       sql.NullString
	WorktreePath sql.NullString
	Branch       sql.NullString
	Model        sql.NullString
	CostUSD      float64
	TokensIn     int64
	TokensOut    int64
	TaskID       sql.NullString
	CreatedAt    string
	UpdatedAt    string
	Meta         json.RawMessage
}

func (s *Store) InsertAgent(ctx context.Context, a *Agent) error {
	if a.ID == "" {
		return fmt.Errorf("agent.ID required")
	}
	if a.CreatedAt == "" {
		a.CreatedAt = now()
	}
	a.UpdatedAt = now()
	if a.State == "" {
		a.State = "idle"
	}
	if a.Meta == nil {
		a.Meta = shared.JSONRaw(map[string]any{})
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agents(id, role, name, cli, pid, state, pane_id, worktree_path,
			branch, model, cost_usd, tokens_in, tokens_out, task_id, created_at,
			updated_at, meta)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.Role, a.Name, a.CLI, a.PID, a.State, a.PaneID, a.WorktreePath,
		a.Branch, a.Model, nullableFloat(a.CostUSD), a.TokensIn, a.TokensOut,
		a.TaskID, a.CreatedAt, a.UpdatedAt, string(a.Meta))
	return err
}

func (s *Store) UpdateAgentState(ctx context.Context, id, state string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET state=?, updated_at=? WHERE id=?`,
		state, now(), id)
	return err
}

func (s *Store) UpdateAgentMeta(ctx context.Context, id string, meta any) error {
	b, _ := json.Marshal(meta)
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET meta=?, updated_at=? WHERE id=?`,
		string(b), now(), id)
	return err
}

func (s *Store) AddAgentCost(ctx context.Context, id string, costUSD float64, inTok, outTok int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET cost_usd = cost_usd + ?, tokens_in = tokens_in + ?,
			tokens_out = tokens_out + ?, updated_at = ? WHERE id = ?`,
		nullableFloat(costUSD), inTok, outTok, now(), id)
	return err
}

func (s *Store) GetAgent(ctx context.Context, id string) (*Agent, error) {
	row := s.db.QueryRowContext(ctx, agentSelect+` WHERE id = ?`, id)
	return scanAgent(row)
}

func (s *Store) ListAgents(ctx context.Context, state string) ([]*Agent, error) {
	q := agentSelect
	args := []any{}
	if state != "" {
		q += ` WHERE state = ?`
		args = append(args, state)
	}
	q += ` ORDER BY created_at ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) DeleteAgent(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id = ?`, id)
	return err
}

const agentSelect = `SELECT id, role, name, cli, pid, state, pane_id,
	worktree_path, branch, model, cost_usd, tokens_in, tokens_out, task_id,
	created_at, updated_at, meta FROM agents`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanAgent(r rowScanner) (*Agent, error) {
	var a Agent
	var meta string
	if err := r.Scan(&a.ID, &a.Role, &a.Name, &a.CLI, &a.PID, &a.State,
		&a.PaneID, &a.WorktreePath, &a.Branch, &a.Model, &a.CostUSD,
		&a.TokensIn, &a.TokensOut, &a.TaskID, &a.CreatedAt, &a.UpdatedAt,
		&meta); err != nil {
		return nil, err
	}
	a.Meta = json.RawMessage(meta)
	return &a, nil
}

// ----------------- tasks ---------------------------------------------------

type Task struct {
	ID            string
	Title         string
	Description   sql.NullString
	Status        string
	Priority      int
	ParentID      sql.NullString
	AssigneeAgent sql.NullString
	Plan          sql.NullString
	Branch        sql.NullString
	WorktreePath  sql.NullString
	PRURL         sql.NullString
	CostCeilUSD   float64
	CostUsedUSD   float64
	CreatedAt     string
	UpdatedAt     string
	StartedAt     sql.NullString
	FinishedAt    sql.NullString
	Meta          json.RawMessage
}

func (s *Store) InsertTask(ctx context.Context, t *Task) error {
	if t.ID == "" {
		return fmt.Errorf("task.ID required")
	}
	if t.CreatedAt == "" {
		t.CreatedAt = now()
	}
	t.UpdatedAt = now()
	if t.Status == "" {
		t.Status = "open"
	}
	if t.CostCeilUSD == 0 {
		t.CostCeilUSD = 2.0
	}
	if t.Meta == nil {
		t.Meta = shared.JSONRaw(map[string]any{})
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO tasks(id, title, description, status, priority, parent_id,
			assignee_agent_id, plan, branch, worktree_path, pr_url, cost_ceiling_usd,
			cost_used_usd, created_at, updated_at, started_at, finished_at, meta)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.Title, t.Description, t.Status, t.Priority, t.ParentID,
		t.AssigneeAgent, t.Plan, t.Branch, t.WorktreePath, t.PRURL,
		nullableFloat(t.CostCeilUSD), nullableFloat(t.CostUsedUSD),
		t.CreatedAt, t.UpdatedAt, t.StartedAt, t.FinishedAt, string(t.Meta))
	return err
}

func (s *Store) UpdateTaskStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status=?, updated_at=? WHERE id=?`,
		status, now(), id)
	return err
}

func (s *Store) AssignTask(ctx context.Context, taskID, agentID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET assignee_agent_id=?, status='assigned', updated_at=? WHERE id=?`,
		agentID, now(), taskID)
	return err
}

func (s *Store) MarkTaskStarted(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status='in_progress', started_at=?, updated_at=? WHERE id=?`,
		now(), now(), id)
	return err
}

func (s *Store) MarkTaskFinished(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET status=?, finished_at=?, updated_at=? WHERE id=?`,
		status, now(), now(), id)
	return err
}

func (s *Store) AddTaskCost(ctx context.Context, id string, costUSD float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET cost_used_usd = cost_used_usd + ?, updated_at = ? WHERE id = ?`,
		nullableFloat(costUSD), now(), id)
	return err
}

func (s *Store) GetTask(ctx context.Context, id string) (*Task, error) {
	row := s.db.QueryRowContext(ctx, taskSelect+` WHERE id = ?`, id)
	return scanTask(row)
}

func (s *Store) ListTasks(ctx context.Context, status string) ([]*Task, error) {
	q := taskSelect
	args := []any{}
	if status != "" {
		q += ` WHERE status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY priority DESC, created_at ASC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

const taskSelect = `SELECT id, title, description, status, priority, parent_id,
	assignee_agent_id, plan, branch, worktree_path, pr_url, cost_ceiling_usd,
	cost_used_usd, created_at, updated_at, started_at, finished_at, meta FROM tasks`

func scanTask(r rowScanner) (*Task, error) {
	var t Task
	var meta string
	if err := r.Scan(&t.ID, &t.Title, &t.Description, &t.Status, &t.Priority,
		&t.ParentID, &t.AssigneeAgent, &t.Plan, &t.Branch, &t.WorktreePath,
		&t.PRURL, &t.CostCeilUSD, &t.CostUsedUSD, &t.CreatedAt, &t.UpdatedAt,
		&t.StartedAt, &t.FinishedAt, &meta); err != nil {
		return nil, err
	}
	t.Meta = json.RawMessage(meta)
	return &t, nil
}

// ----------------- events (read-side; writes happen in bus) ----------------

// RecentEvents returns the last N events (for /api/v1/events).
func (s *Store) RecentEvents(ctx context.Context, types []string, agentID string, limit int) ([]*StoredEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	q := `SELECT seq, type, agent_id, task_id, payload, ts FROM events WHERE 1=1`
	args := []any{}
	if len(types) > 0 {
		q += ` AND type IN (` + placeholders(len(types)) + `)`
		for _, t := range types {
			args = append(args, t)
		}
	}
	if agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	q += ` ORDER BY seq DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*StoredEvent
	for rows.Next() {
		var e StoredEvent
		var pl string
		var ag, ta sql.NullString
		if err := rows.Scan(&e.Seq, &e.Type, &ag, &ta, &pl, &e.TS); err != nil {
			return nil, err
		}
		e.AgentID = ag.String
		e.TaskID = ta.String
		e.Payload = json.RawMessage(pl)
		out = append(out, &e)
	}
	return out, rows.Err()
}

type StoredEvent struct {
	Seq     int64
	Type    string
	AgentID string
	TaskID  string
	Payload json.RawMessage
	TS      string
}

func placeholders(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			out += ","
		}
		out += "?"
	}
	return out
}

// ----------------- messages (mailbox) -------------------------------------

type Message struct {
	ID         string
	FromAgent  string
	ToAgent    string
	Kind       string
	Body       string
	TaskID     sql.NullString
	ReadAt     sql.NullString
	CreatedAt  string
}

func (s *Store) PostMessage(ctx context.Context, m *Message) error {
	if m.CreatedAt == "" {
		m.CreatedAt = now()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages(id, from_agent_id, to_agent_id, kind, body, task_id, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		m.ID, m.FromAgent, m.ToAgent, m.Kind, m.Body, m.TaskID, m.CreatedAt)
	return err
}

func (s *Store) ReadInbox(ctx context.Context, toAgent string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, from_agent_id, to_agent_id, kind, body, task_id, read_at, created_at
		 FROM messages WHERE to_agent_id = ? ORDER BY created_at DESC LIMIT ?`,
		toAgent, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.FromAgent, &m.ToAgent, &m.Kind, &m.Body,
			&m.TaskID, &m.ReadAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (s *Store) MarkMessageRead(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE messages SET read_at=? WHERE id=?`, now(), id)
	return err
}

// ----------------- panes ---------------------------------------------------

type Pane struct {
	ID            string
	AgentID       sql.NullString
	Scrollback    sql.NullString
	LastState     sql.NullString
	LastStateAt   sql.NullString
	Cols          int
	Rows          int
}

func (s *Store) UpsertPane(ctx context.Context, p *Pane) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO panes(id, agent_id, scrollback, last_state, last_state_at, cols, rows)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(id) DO UPDATE SET
		   agent_id=excluded.agent_id,
		   scrollback=excluded.scrollback,
		   last_state=excluded.last_state,
		   last_state_at=excluded.last_state_at,
		   cols=excluded.cols, rows=excluded.rows`,
		p.ID, p.AgentID, p.Scrollback, p.LastState, p.LastStateAt, p.Cols, p.Rows)
	return err
}

func (s *Store) AppendPaneScrollback(ctx context.Context, id string, chunk []byte) error {
	// naive: read full, append, truncate to last 200KB, write back.
	// Sufficient for V1 (last-N-lines read). V2 use ring buffer or
	// append-only blob.
	row := s.db.QueryRow(`SELECT scrollback FROM panes WHERE id = ?`, id)
	var cur sql.NullString
	if err := row.Scan(&cur); err != nil {
		return err
	}
	combined := cur.String + string(chunk)
	const max = 200 * 1024
	if len(combined) > max {
		combined = combined[len(combined)-max:]
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE panes SET scrollback = ? WHERE id = ?`, combined, id)
	return err
}

func (s *Store) GetPane(ctx context.Context, id string) (*Pane, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, scrollback, last_state, last_state_at, cols, rows
		 FROM panes WHERE id = ?`, id)
	var p Pane
	if err := row.Scan(&p.ID, &p.AgentID, &p.Scrollback, &p.LastState,
		&p.LastStateAt, &p.Cols, &p.Rows); err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) ListPanes(ctx context.Context) ([]*Pane, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, scrollback, last_state, last_state_at, cols, rows
		 FROM panes ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Pane
	for rows.Next() {
		var p Pane
		if err := rows.Scan(&p.ID, &p.AgentID, &p.Scrollback, &p.LastState,
			&p.LastStateAt, &p.Cols, &p.Rows); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ----------------- settings ------------------------------------------------

func (s *Store) GetSetting(ctx context.Context, key string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key)
	var v string
	if err := row.Scan(&v); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return v, true, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings(key, value, updated_at) VALUES(?,?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, now())
	return err
}
