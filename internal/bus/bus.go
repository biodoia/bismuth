// Package bus is the WebSocket pub/sub event bus for bismuth.
//
// The bus has two roles:
//
//  1. IN-MEMORY FAN-OUT: any goroutine can call Publish(evt) and all
//     subscribed WebSocket clients receive it in <1ms.
//  2. PERSISTENT LOG: every published event is also appended to the
//     `events` SQLite table, so a new client can replay history.
//
// Wire format: JSON, one event per WS message. Event types are stable
// strings; payloads are JSON objects (opaque to the bus).
//
// Subscribers can filter on event type and/or agent_id.
package bus

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Event is the canonical on-wire event shape.
type Event struct {
	Seq     int64           `json:"seq"`
	Type    string          `json:"type"` // agent_spawned, agent_state, ...
	AgentID string          `json:"agent_id,omitempty"`
	TaskID  string          `json:"task_id,omitempty"`
	Payload json.RawMessage `json:"payload"` // opaque JSON
	TS      string          `json:"ts"`      // ISO8601 from server clock
}

// Bus is the central event hub.
type Bus struct {
	db *sql.DB

	mu          sync.RWMutex
	subs        map[*sub]struct{}
	nextLocalID int64
}

type sub struct {
	conn      *websocket.Conn
	eventCh   chan Event
	closeOnce sync.Once
	filter    Filter
}

// Filter restricts which events a subscriber receives.
type Filter struct {
	Types   []string // empty = all
	AgentID string   // empty = all
}

// New creates a bus. The bus uses the provided DB to persist every event
// before fan-out.
func New(db *sql.DB) *Bus {
	return &Bus{
		db:   db,
		subs: make(map[*sub]struct{}),
	}
}

// Close shuts down all subscribers.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs {
		s.closeOnce.Do(func() { close(s.eventCh) })
		delete(b.subs, s)
	}
}

// Publish persists the event and fans it out. It is safe to call from
// any goroutine.
func (b *Bus) Publish(ctx context.Context, evt Event) error {
	if evt.TS == "" {
		evt.TS = time.Now().UTC().Format(time.RFC3339)
	}
	res, err := b.db.ExecContext(ctx,
		`INSERT INTO events(type, agent_id, task_id, payload, ts) VALUES(?,?,?,?,?)`,
		evt.Type, nullStr(evt.AgentID), nullStr(evt.TaskID), safePayload(evt.Payload), evt.TS)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	evt.Seq = id

	b.mu.RLock()
	defer b.mu.RUnlock()
	for s := range b.subs {
		if s.filter.matches(evt) {
			select {
			case s.eventCh <- evt:
			default:
				// slow consumer; skip. Reconnect will replay from DB.
			}
		}
	}
	return nil
}

// Subscribe registers a WebSocket connection. The returned channel emits
// events until the connection closes. Cancelling ctx detaches.
func (b *Bus) Subscribe(ctx context.Context, conn *websocket.Conn, f Filter) <-chan Event {
	ch := make(chan Event, 64)
	s := &sub{conn: conn, eventCh: ch, filter: f}
	b.mu.Lock()
	b.subs[s] = struct{}{}
	b.mu.Unlock()

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		delete(b.subs, s)
		b.mu.Unlock()
		s.closeOnce.Do(func() { close(ch) })
	}()
	return ch
}

// Replay returns the last N events of given types from the persistent
// log, useful for new clients catching up after disconnect.
func (b *Bus) Replay(ctx context.Context, types []string, agentID string, limit int) ([]Event, error) {
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
	rows, err := b.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		var ag, ta sql.NullString
		var pl string
		if err := rows.Scan(&e.Seq, &e.Type, &ag, &ta, &pl, &e.TS); err != nil {
			return nil, err
		}
		e.AgentID = ag.String
		e.TaskID = ta.String
		e.Payload = json.RawMessage(pl)
		out = append(out, e)
	}
	return out, rows.Err()
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

func (f Filter) matches(e Event) bool {
	if len(f.Types) > 0 {
		ok := false
		for _, t := range f.Types {
			if t == e.Type {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if f.AgentID != "" && f.AgentID != e.AgentID {
		return false
	}
	return true
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func safePayload(p json.RawMessage) string {
	if len(p) == 0 {
		return "{}"
	}
	return string(p)
}
