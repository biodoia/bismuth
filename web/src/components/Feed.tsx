// components/Feed.tsx — right column (300px) real-time event feed.
// Wireframe v1 design system. Expandable items, filter, auto-scroll.

import { useState, useEffect, useRef, useCallback } from "react";
import { useBismuthWS } from "../hooks/useWebSocket";

const TYPE_COLOR: Record<string, string> = {
  agent_spawned: "#22C55E",
  agent_killed: "#EF4444",
  agent_state: "#EAB308",
  pane_output: "#888",
  pane_input: "#06B6D4",
  task_assigned: "#3B82F6",
  task_done: "#22C55E",
  human_approval_required: "#EF4444",
  voice_stt: "#A855F7",
  voice_speak: "#A855F7",
  audit_log: "#888",
};

export default function Feed() {
  const [filter, setFilter] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});
  const listRef = useRef<HTMLUListElement>(null);

  const { events, connected, error } = useBismuthWS({});

  useEffect(() => {
    if (autoScroll && listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
  }, [events, autoScroll]);

  const filtered = filter
    ? events.filter(
        (e) =>
          e.agent_id?.includes(filter) ||
          e.type.includes(filter)
      )
    : events;

  const toggleExpand = useCallback((i: number) => {
    setExpanded((prev) => ({ ...prev, [i]: !prev[i] }));
  }, []);

  return (
    <div className="flex flex-col h-full">
      {/* Section header */}
      <div
        className="flex items-center justify-between px-3 py-2 border-b shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <h2 className="text-xs font-semibold text-[#ededed] uppercase tracking-wider">
          Feed
        </h2>
        <span className="flex items-center gap-1.5 text-[10px]">
          <span
            className={`inline-block w-1.5 h-1.5 rounded-full ${
              connected ? "bg-[#22C55E] ws-pulse" : "bg-[#555]"
            }`}
          />
          <span className={connected ? "text-[#22C55E]" : "text-[#555]"}>
            {connected ? "live" : "offline"}
          </span>
        </span>
      </div>

      {/* Filter bar */}
      <div
        className="flex items-center gap-2 px-3 py-1.5 border-b shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <input
          className="flex-1 text-[10px] px-2 py-1 rounded bg-[#161718] text-[#ededed] border outline-none placeholder:text-[#555] hover:border-[rgba(255,255,255,0.10)] focus:border-[#00D4AA] transition-colors"
          style={{ borderColor: "rgba(255,255,255,0.06)" }}
          placeholder="filter by type or agent_id"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <label className="flex items-center gap-1 text-[10px] text-[#555] cursor-pointer select-none">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={(e) => setAutoScroll(e.target.checked)}
            className="accent-[#00D4AA]"
          />
          auto
        </label>
      </div>

      {error && (
        <div className="px-3 py-1 text-[10px] text-[#EF4444] bg-[rgba(239,68,68,0.08)] shrink-0">
          {error}
        </div>
      )}

      {/* Event list */}
      <ul ref={listRef} className="flex-1 overflow-y-auto p-2 space-y-0.5">
        {filtered.length === 0 && (
          <li className="text-[10px] text-[#555] py-8 text-center">
            — no events —
          </li>
        )}
        {filtered.map((e, i) => {
          const typeColor = TYPE_COLOR[e.type] || "#888";
          const isExpanded = expanded[i];
          return (
            <li
              key={`${e.seq}-${i}`}
              className="rounded cursor-pointer transition-colors border"
              onClick={() => toggleExpand(i)}
              style={{
                background: isExpanded ? "#161718" : "#0f1011",
                borderColor: isExpanded
                  ? "rgba(255,255,255,0.10)"
                  : "rgba(255,255,255,0.03)",
                padding: "6px 8px",
              }}
            >
              <div className="flex items-center gap-2">
                <span className="text-[10px] text-[#555] w-14 shrink-0 font-mono">
                  {e.ts?.substring(11, 19) || ""}
                </span>
                <span
                  className="text-[10px] font-medium shrink-0"
                  style={{ color: typeColor }}
                >
                  {e.type}
                </span>
                {e.agent_id && (
                  <span
                    className="text-[10px] text-[#555] truncate"
                    style={{ fontFamily: "'JetBrains Mono', monospace" }}
                  >
                    {e.agent_id.slice(0, 12)}
                  </span>
                )}
                <span className="ml-auto text-[10px] text-[#555]">
                  {isExpanded ? "▾" : "▸"}
                </span>
              </div>
              {isExpanded && (
                <pre
                  className="mt-1.5 pt-1.5 text-[10px] text-[#555] whitespace-pre-wrap break-all overflow-x-auto border-t"
                  style={{
                    fontFamily: "'JetBrains Mono', monospace",
                    borderColor: "rgba(255,255,255,0.06)",
                  }}
                >
                  {JSON.stringify(e.payload, null, 2)}
                </pre>
              )}
            </li>
          );
        })}
      </ul>

      {/* Footer */}
      <div
        className="flex items-center justify-between px-3 py-1.5 border-t text-[10px] text-[#555]"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <span>{filtered.length} events</span>
        <span>seq #{filtered.length > 0 ? filtered[filtered.length - 1].seq : 0}</span>
      </div>
    </div>
  );
}
