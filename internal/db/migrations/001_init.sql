-- 001_init.sql
-- bismuth initial schema (V1)
-- See internal/db/db.go for the table reference.

CREATE TABLE IF NOT EXISTS schema_version (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Agents: worker pane instances, one row per spawned worker.
CREATE TABLE IF NOT EXISTS agents (
    id TEXT PRIMARY KEY,                  -- ulid
    role TEXT NOT NULL,                    -- planner | implementer | reviewer | ...
    name TEXT NOT NULL,                    -- human-readable, e.g. "omx-1"
    cli TEXT NOT NULL,                     -- omx | omc | omo | omp | claude | codex | opencode
    pid INTEGER,                           -- OS pid of the worker process
    state TEXT NOT NULL DEFAULT 'idle',    -- idle | planning | working | reviewing | done | blocked | killed
    pane_id TEXT,                          -- backend PTY id
    worktree_path TEXT,                    -- git worktree path
    branch TEXT,                           -- git branch name
    model TEXT,                            -- LLM model in use
    cost_usd REAL DEFAULT 0,               -- running cost
    tokens_in INTEGER DEFAULT 0,
    tokens_out INTEGER DEFAULT 0,
    task_id TEXT,                          -- current task
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    meta TEXT                              -- JSON blob for role-specific state
);
CREATE INDEX IF NOT EXISTS idx_agents_state ON agents(state);
CREATE INDEX IF NOT EXISTS idx_agents_role ON agents(role);

-- Tasks: the bacheca (task board) — what work is queued/in-flight/done.
CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,                   -- ulid
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'open',   -- open | assigned | in_progress | review | done | failed | cancelled
    priority INTEGER DEFAULT 0,            -- 0=normal, 1=high, 2=urgent
    parent_id TEXT,                        -- for sub-tasks
    assignee_agent_id TEXT,
    plan TEXT,                             -- markdown plan from planner
    branch TEXT,                           -- git branch for this task
    worktree_path TEXT,
    pr_url TEXT,                           -- GitHub PR URL when opened
    cost_ceiling_usd REAL DEFAULT 2.0,
    cost_used_usd REAL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    started_at TEXT,
    finished_at TEXT,
    meta TEXT
);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assignee ON tasks(assignee_agent_id);

-- Events: append-only bus log. Every WS pub also lands here.
CREATE TABLE IF NOT EXISTS events (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,                    -- agent_spawned | agent_state | agent_message | pane_output | task_assigned | task_done | error | ...
    agent_id TEXT,
    task_id TEXT,
    payload TEXT NOT NULL,                 -- JSON
    ts TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
CREATE INDEX IF NOT EXISTS idx_events_ts ON events(ts);
CREATE INDEX IF NOT EXISTS idx_events_agent ON events(agent_id);

-- Audit log: tamper-evident hash chain (each row hashes prev hash + own content).
CREATE TABLE IF NOT EXISTS audit_log (
    seq INTEGER PRIMARY KEY AUTOINCREMENT,
    actor TEXT NOT NULL,                   -- hermes | user:lisergico25 | agent:omx-1 | system
    action TEXT NOT NULL,                  -- spawn_agent | send_prompt | kill_agent | open_pr | merge | ...
    target TEXT,                           -- subject of action
    payload TEXT,                          -- JSON detail
    prev_hash TEXT NOT NULL,               -- hex SHA-256 of previous row
    row_hash TEXT NOT NULL,                -- hex SHA-256(prev_hash + salt + actor + action + target + payload + ts)
    ts TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Panes: PTY scrollback tail (last N lines) and last-state snapshot.
-- This is what the web "terminal remote" renders.
CREATE TABLE IF NOT EXISTS panes (
    id TEXT PRIMARY KEY,                   -- pane_id from agents
    agent_id TEXT,
    scrollback TEXT,                       -- last 5000 lines, ANSI
    last_state TEXT,                       -- idle/working/blocked/etc detection
    last_state_at TEXT,
    cols INTEGER DEFAULT 120,
    rows INTEGER DEFAULT 40
);

-- Messages: inter-agent mailbox (worker → worker, worker → lead).
CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,                   -- ulid
    from_agent_id TEXT NOT NULL,
    to_agent_id TEXT NOT NULL,             -- 'lead' for Hermes, or specific agent id
    kind TEXT NOT NULL,                    -- text | structured | review_request | review_response | claim | finish
    body TEXT NOT NULL,
    task_id TEXT,
    read_at TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_messages_to ON messages(to_agent_id, read_at);

-- Settings: key-value runtime config (overrides file config).
CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

INSERT OR IGNORE INTO schema_version (version) VALUES (1);
