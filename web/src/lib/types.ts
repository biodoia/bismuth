// lib/types.ts — shared TypeScript types matching Go structs in
// internal/bus/bus.go and internal/db/migrations/001_init.sql.

export type Event = {
  seq: number;
  type: string; // agent_spawned, agent_state, pane_output, agent_message, task_*, ...
  agent_id?: string;
  task_id?: string;
  payload: any; // opaque JSON
  ts: string;   // ISO8601
};

export type Agent = {
  id: string;
  role: string;
  name: string;
  cli: string;
  state: "idle" | "planning" | "working" | "reviewing" | "done" | "blocked" | "killed";
  pane_id?: string;
  worktree_path?: string;
  branch?: string;
  model?: string;
  cost_usd: number;
  task_id?: string;
};

export type Task = {
  id: string;
  title: string;
  description?: string;
  status: "open" | "assigned" | "in_progress" | "review" | "done" | "failed" | "cancelled";
  priority: number;
  parent_id?: string;
  assignee_agent_id?: string;
  branch?: string;
  pr_url?: string;
  cost_ceiling_usd: number;
  cost_used_usd: number;
};

// AuditEntry matches internal/audit Entry (GET /api/v1/audit).
// payload is a JSON-encoded string server-side, but tolerate objects.
export type AuditEntry = {
  seq: number;
  ts: string;
  actor: string;
  action: string;
  target: string;
  payload: unknown;
  row_hash: string;
};

// VoiceCommandResponse matches POST /v1/voice/command.
// ignored=true means wake-word not detected in continuous mode.
export type VoiceCommandResponse = {
  heard: string;
  action: string;
  args?: string[];
  text_response?: string;
  ignored?: boolean;
};
