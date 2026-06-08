// components/Feed.tsx — real-time event feed (right column of App).
//
// Subscribes to /api/v1/ws, displays last 200 events with color-coding
// by type, auto-scroll toggle, filter by agent_id. Click an event to
// expand its payload.

import { useState, useEffect } from "react";
import { useBismuthWS } from "../hooks/useWebSocket";

const TYPE_COLOR: Record<string, string> = {
  agent_spawned: "text-emerald-400",
  agent_killed: "text-rose-400",
  agent_state: "text-amber-300",
  pane_output: "text-zinc-400",
  pane_input: "text-cyan-300",
  task_assigned: "text-sky-400",
  task_done: "text-emerald-300",
  human_approval_required: "text-rose-300",
  voice_stt: "text-violet-300",
  voice_speak: "text-violet-400",
};

export default function Feed() {
  const [filter, setFilter] = useState("");
  const [autoScroll, setAutoScroll] = useState(true);
  const [expanded, setExpanded] = useState<Record<number, boolean>>({});

  const { events, connected, error } = useBismuthWS({});

  useEffect(() => {
    if (autoScroll) {
      const el = document.getElementById("feed-list");
      if (el) el.scrollTop = el.scrollHeight;
    }
  }, [events, autoScroll]);

  const filtered = filter
    ? events.filter(
        (e) => e.agent_id?.includes(filter) || e.type.includes(filter)
      )
    : events;

  return (
    <div className="flex flex-col h-full gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Feed</h2>
        <span className={"text-xs " + (connected ? "text-emerald-400" : "text-rose-400")}>
          {connected ? "● live" : "○ offline"}
        </span>
      </header>
      <div className="flex gap-2 text-xs">
        <input
          className="flex-1 bg-zinc-950 border border-zinc-800 rounded px-2 py-1"
          placeholder="filtra per type o agent_id"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
        />
        <label className="flex items-center gap-1 text-zinc-500">
          <input
            type="checkbox"
            checked={autoScroll}
            onChange={(e) => setAutoScroll(e.target.checked)}
          />
          auto
        </label>
      </div>
      {error && <p className="text-xs text-rose-400">{error}</p>}
      <ul id="feed-list" className="flex-1 overflow-y-auto space-y-1">
        {filtered.length === 0 && <li className="text-xs text-zinc-600">— feed vuoto —</li>}
        {filtered.map((e, i) => {
          const color = TYPE_COLOR[e.type] || "text-zinc-300";
          return (
            <li
              key={`${e.seq}-${i}`}
              className="text-xs bg-zinc-950 border border-zinc-900 rounded p-1.5 cursor-pointer"
              onClick={() => setExpanded((p) => ({ ...p, [i]: !p[i] }))}
            >
              <div className="flex gap-2">
                <span className="text-zinc-600 w-16 shrink-0">
                  {e.ts.substring(11, 19)}
                </span>
                <span className={"shrink-0 " + color}>{e.type}</span>
                {e.agent_id && <span className="text-zinc-500 truncate">{e.agent_id}</span>}
              </div>
              {expanded[i] && (
                <pre className="mt-1 text-[10px] text-zinc-500 whitespace-pre-wrap break-all">
                  {JSON.stringify(e.payload, null, 2)}
                </pre>
              )}
            </li>
          );
        })}
      </ul>
      <p className="text-[10px] text-zinc-700">{filtered.length} eventi</p>
    </div>
  );
}
