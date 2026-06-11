// components/Tasks.tsx — compact task list inside the Feed column (P7-i).
//
// Tasks are draggable (native HTML5 DnD, no libs): drag a row onto an
// agent card in the Agents zone to POST /api/v1/tasks/:id/assign.
// Wireframe v1 design system.

import { useEffect, useState } from "react";
import { useBismuthStore } from "../lib/store";
import type { Task } from "../lib/types";

export const TASK_DRAG_MIME = "application/x-bismuth-task";

const STATUS_COLOR: Record<Task["status"], string> = {
  open: "#888",
  assigned: "#3B82F6",
  in_progress: "#EAB308",
  review: "#A855F7",
  done: "#22C55E",
  failed: "#EF4444",
  cancelled: "#555",
};

// Terminal statuses are display-only; everything else can be re-assigned.
const DRAGGABLE = new Set<Task["status"]>(["open", "assigned", "in_progress", "review"]);

export default function Tasks() {
  const tasks = useBismuthStore((s) => s.tasks);
  const fetchTasks = useBismuthStore((s) => s.fetchTasks);
  const [draggingId, setDraggingId] = useState<string | null>(null);

  useEffect(() => {
    fetchTasks();
    const id = setInterval(fetchTasks, 5000);
    return () => clearInterval(id);
  }, [fetchTasks]);

  const open = tasks.filter((t) => DRAGGABLE.has(t.status));
  const closed = tasks.length - open.length;

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* Section header */}
      <div
        className="flex items-center justify-between px-3 py-2 border-b shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <h2 className="text-xs font-semibold text-[#ededed] uppercase tracking-wider">
          Tasks
        </h2>
        <span className="text-[10px] text-[#555]">
          {open.length} open{closed > 0 ? ` · ${closed} closed` : ""}
        </span>
      </div>

      {/* Task rows */}
      <ul className="flex-1 overflow-y-auto p-2 space-y-1 min-h-0">
        {tasks.length === 0 && (
          <li className="text-[10px] text-[#555] py-4 text-center">
            — no tasks —
          </li>
        )}
        {tasks.map((t) => {
          const draggable = DRAGGABLE.has(t.status);
          const isDragging = draggingId === t.id;
          const color = STATUS_COLOR[t.status] || "#888";
          return (
            <li
              key={t.id}
              draggable={draggable}
              onDragStart={(e) => {
                e.dataTransfer.setData(TASK_DRAG_MIME, t.id);
                e.dataTransfer.setData("text/plain", t.id);
                e.dataTransfer.effectAllowed = "move";
                setDraggingId(t.id);
              }}
              onDragEnd={() => setDraggingId(null)}
              className={`group rounded border p-2 transition-all select-none ${
                draggable ? "cursor-grab active:cursor-grabbing" : "opacity-60"
              }`}
              style={{
                background: isDragging ? "#161718" : "#0f1011",
                borderColor: isDragging ? "#00D4AA" : "rgba(255,255,255,0.06)",
                opacity: isDragging ? 0.5 : undefined,
              }}
              title={draggable ? "Drag onto an agent card to assign" : t.status}
            >
              <div className="flex items-center gap-2 min-w-0">
                {/* Drag handle */}
                <span
                  className={`shrink-0 text-[11px] leading-none ${
                    draggable
                      ? "text-[#555] group-hover:text-[#888]"
                      : "text-[#333]"
                  }`}
                  style={{ letterSpacing: "-1px" }}
                  aria-hidden
                >
                  ⠿
                </span>
                <span className="text-[11px] text-[#ededed] truncate flex-1">
                  {t.title || t.id}
                </span>
                <span
                  className="flex items-center gap-1 text-[9px] px-1.5 py-0.5 rounded bg-[#161718] shrink-0"
                  style={{ color }}
                >
                  <span
                    className="inline-block w-1 h-1 rounded-full"
                    style={{ background: color }}
                  />
                  {t.status}
                </span>
              </div>
              <div
                className="flex items-center gap-2 mt-1 text-[9px] text-[#555] truncate"
                style={{ fontFamily: "'JetBrains Mono', monospace" }}
              >
                <span>{t.id.slice(0, 12)}</span>
                {t.assignee_agent_id && (
                  <span className="text-[#3B82F6] truncate">
                    → {t.assignee_agent_id.slice(0, 14)}
                  </span>
                )}
                <span className="ml-auto shrink-0">p{t.priority}</span>
              </div>
            </li>
          );
        })}
      </ul>

      {/* Hint footer */}
      <div
        className="px-3 py-1 border-t text-[9px] text-[#555] shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        drag ⠿ onto an agent to assign
      </div>
    </div>
  );
}
