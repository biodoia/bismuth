// components/Agents.tsx — agent sidebar (240px) with spawn controls, clickable cards.
// Wireframe v1 design system. Agent list lives in the zustand store
// (polled by App, patched live by SSE state events).
// Agent cards are drop targets for task drag-and-drop assignment (P7-i).

import { useState, useEffect, useCallback } from "react";
import { assignTask } from "../lib/api";
import { useBismuthStore } from "../lib/store";
import { TASK_DRAG_MIME } from "./Tasks";

interface AgentsProps {
  selectedAgentId: string | null;
  onSelectAgent: (id: string) => void;
}

const STATE_DOT: Record<string, string> = {
  working: "#22C55E",
  idle: "#EAB308",
  killed: "#EF4444",
  done: "#3B82F6",
  error: "#EF4444",
  planning: "#22C55E",
  reviewing: "#EAB308",
  blocked: "#EF4444",
  exited: "#EF4444",
};

const STATE_LABEL: Record<string, string> = {
  working: "working",
  idle: "idle",
  killed: "killed",
  done: "done",
  error: "error",
  planning: "planning",
  reviewing: "reviewing",
  blocked: "blocked",
  exited: "exited",
};

const ROLE_ICONS: Record<string, string> = {
  implementer: "⌨",
  reviewer: "👁",
  architect: "🏗",
  tester: "🧪",
  planner: "📋",
  orchestrator: "🎼",
  hermes: "🕊",
  researcher: "🔍",
};

const ROLES = [
  "implementer",
  "reviewer",
  "architect",
  "tester",
  "planner",
  "orchestrator",
  "hermes",
  "researcher",
];

// isTaskDrag: only react to our own drag payloads.
function isTaskDrag(e: React.DragEvent): boolean {
  return Array.from(e.dataTransfer.types).includes(TASK_DRAG_MIME);
}

export default function Agents({ selectedAgentId, onSelectAgent }: AgentsProps) {
  const agents = useBismuthStore((s) => s.agents);
  const fetchAgents = useBismuthStore((s) => s.fetchAgents);
  const fetchTasks = useBismuthStore((s) => s.fetchTasks);
  const showToast = useBismuthStore((s) => s.showToast);

  const [roles, setRoles] = useState<string[]>(ROLES);
  const [spawning, setSpawning] = useState(false);
  const [spawnRole, setSpawnRole] = useState("implementer");
  const [spawnTask, setSpawnTask] = useState("");
  const [error, setError] = useState("");
  const [dropTargetId, setDropTargetId] = useState<string | null>(null);

  const fetchRoles = useCallback(async () => {
    try {
      const r = await fetch("/api/v1/roles");
      if (!r.ok) return;
      const body = await r.json();
      if (body.roles?.length) {
        setRoles(body.roles.map((rl: { name: string }) => rl.name));
      }
    } catch {
      /* fallback to defaults */
    }
  }, []);

  useEffect(() => {
    fetchRoles();
  }, [fetchRoles]);

  const handleSpawn = async () => {
    setSpawning(true);
    setError("");
    try {
      const r = await fetch("/api/v1/agents", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          role: spawnRole,
          cli: "bash",
          task: spawnTask || "awaiting instructions",
        }),
      });
      if (!r.ok) {
        const b = await r.json();
        setError(b.error || "spawn failed");
      } else {
        setTimeout(fetchAgents, 500);
      }
    } catch (e: unknown) {
      setError((e as Error)?.message || "spawn failed");
    } finally {
      setSpawning(false);
    }
  };

  const handleKill = async (e: React.MouseEvent, id: string) => {
    e.stopPropagation();
    await fetch(`/api/v1/agents/${id}/kill`, { method: "POST" });
    setTimeout(fetchAgents, 500);
  };

  // handleDrop: task dropped on an agent card → POST assign + optimistic refresh.
  const handleDrop = async (e: React.DragEvent, agentId: string) => {
    e.preventDefault();
    setDropTargetId(null);
    const taskId =
      e.dataTransfer.getData(TASK_DRAG_MIME) || e.dataTransfer.getData("text/plain");
    if (!taskId) return;
    try {
      await assignTask(taskId, agentId);
      showToast(`task ${taskId.slice(0, 10)} → ${agentId.slice(0, 12)}`, "success");
    } catch (err: unknown) {
      showToast(
        `assign failed: ${(err as Error)?.message || "error"}`,
        "error"
      );
    }
    // Optimistic refresh either way: server is the source of truth.
    fetchTasks();
    fetchAgents();
  };

  const active = agents.filter(
    (a) => a.state === "working" || a.state === "idle" || a.state === "planning" || a.state === "reviewing"
  );
  const completed = agents.filter(
    (a) => !active.includes(a)
  );

  return (
    <div className="flex flex-col h-full">
      {/* Section header */}
      <div className="flex items-center justify-between px-3 py-2 border-b"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <h2 className="text-xs font-semibold text-[#ededed] uppercase tracking-wider">
          Agents
        </h2>
        <span className="text-[10px] text-[#555]">
          {active.length} active
        </span>
      </div>

      {/* Spawn bar */}
      <div className="px-3 py-2 border-b space-y-1.5" style={{ borderColor: "rgba(255,255,255,0.06)" }}>
        <div className="flex gap-1.5">
          <select
            className="spawn-select flex-1 text-xs px-2 py-1.5 rounded bg-[#161718] text-[#ededed] border hover:border-[rgba(255,255,255,0.10)] focus:border-[#00D4AA] outline-none transition-colors"
            style={{ borderColor: "rgba(255,255,255,0.06)" }}
            value={spawnRole}
            onChange={(e) => setSpawnRole(e.target.value)}
          >
            {roles.map((r) => (
              <option key={r} value={r}>
                {ROLE_ICONS[r] || "🤖"} {r}
              </option>
            ))}
          </select>
          <button
            onClick={handleSpawn}
            disabled={spawning}
            className="spawn-btn text-xs px-3 py-1.5 rounded font-medium bg-[rgba(0,212,170,0.15)] text-[#00D4AA] border border-[rgba(0,212,170,0.25)] hover:bg-[rgba(0,212,170,0.25)] disabled:opacity-40 transition-colors"
          >
            {spawning ? "..." : "+ spawn"}
          </button>
        </div>
        <input
          className="w-full text-[10px] px-2 py-1 rounded bg-[#161718] text-[#888] border hover:border-[rgba(255,255,255,0.10)] focus:border-[#00D4AA] outline-none placeholder:text-[#555] transition-colors"
          style={{ borderColor: "rgba(255,255,255,0.06)" }}
          placeholder="task description (optional)"
          value={spawnTask}
          onChange={(e) => setSpawnTask(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") handleSpawn();
          }}
        />
        {error && <p className="text-[10px] text-[#EF4444]">{error}</p>}
      </div>

      {/* Agent cards */}
      <ul className="flex-1 overflow-y-auto p-2 space-y-1">
        {active.length === 0 && (
          <li className="text-[10px] text-[#555] py-8 text-center">
            — no active agents —
          </li>
        )}
        {active.map((a) => {
          const isSelected = selectedAgentId === a.id;
          const isDropTarget = dropTargetId === a.id;
          const dotColor = STATE_DOT[a.state] || "#555";
          return (
            <li
              key={a.id}
              onClick={() => onSelectAgent(a.id)}
              onDragOver={(e) => {
                if (!isTaskDrag(e)) return;
                e.preventDefault();
                e.dataTransfer.dropEffect = "move";
                setDropTargetId(a.id);
              }}
              onDragLeave={(e) => {
                // Ignore leave events caused by entering a child node.
                if (e.currentTarget.contains(e.relatedTarget as Node)) return;
                setDropTargetId((cur) => (cur === a.id ? null : cur));
              }}
              onDrop={(e) => handleDrop(e, a.id)}
              className="group rounded p-2 cursor-pointer transition-all border"
              style={{
                background: isDropTarget ? "rgba(0,212,170,0.08)" : isSelected ? "#161718" : "#0f1011",
                borderColor: isDropTarget
                  ? "#00D4AA"
                  : isSelected
                  ? "#00D4AA"
                  : "rgba(255,255,255,0.06)",
                boxShadow: isDropTarget ? "0 0 0 2px rgba(0,212,170,0.35)" : undefined,
              }}
            >
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2 min-w-0">
                  <span className="text-sm">
                    {ROLE_ICONS[a.role] || "🤖"}
                  </span>
                  <div className="min-w-0">
                    <div
                      className="text-[11px] font-mono truncate"
                      style={{ fontFamily: "'JetBrains Mono', monospace" }}
                    >
                      {a.id.slice(0, 16)}
                    </div>
                    <div className="text-[10px] text-[#555]">
                      {a.role} · {a.cli}
                    </div>
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  {/* Status badge with dot-before */}
                  <span className="flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded bg-[#161718]">
                    <span
                      className="inline-block w-1.5 h-1.5 rounded-full"
                      style={{ background: dotColor }}
                    />
                    <span style={{ color: dotColor }}>
                      {STATE_LABEL[a.state] || a.state}
                    </span>
                  </span>
                  {/* Kill button */}
                  <button
                    onClick={(e) => handleKill(e, a.id)}
                    className="text-[10px] text-[#555] hover:text-[#EF4444] px-1 opacity-0 group-hover:opacity-100 transition-opacity"
                    title="Kill agent"
                  >
                    ✕
                  </button>
                </div>
              </div>
              {a.task_id && (
                <div
                  className="text-[10px] text-[#555] mt-1 truncate"
                  style={{ fontFamily: "'JetBrains Mono', monospace" }}
                >
                  task: {a.task_id}
                </div>
              )}
              {isDropTarget && (
                <div className="text-[9px] text-[#00D4AA] mt-1">
                  ⇣ drop to assign task
                </div>
              )}
            </li>
          );
        })}
      </ul>

      {/* Completed agents (collapsed) */}
      {completed.length > 0 && (
        <div className="border-t px-3 py-2" style={{ borderColor: "rgba(255,255,255,0.06)" }}>
          <details className="text-xs">
            <summary className="text-[#555] cursor-pointer hover:text-[#888] transition-colors">
              {completed.length} completed
            </summary>
            <ul className="mt-1.5 space-y-0.5">
              {completed.slice(0, 20).map((a) => (
                <li
                  key={a.id}
                  className="flex items-center gap-2 text-[10px] text-[#555] cursor-pointer hover:text-[#888] transition-colors"
                  style={{ fontFamily: "'JetBrains Mono', monospace" }}
                >
                  <span
                    className="inline-block w-1.5 h-1.5 rounded-full shrink-0"
                    style={{ background: STATE_DOT[a.state] || "#555" }}
                  />
                  <span className="truncate">{a.id.slice(0, 20)}</span>
                  <span className="ml-auto text-[#555]">{a.role}</span>
                </li>
              ))}
            </ul>
          </details>
        </div>
      )}
    </div>
  );
}
