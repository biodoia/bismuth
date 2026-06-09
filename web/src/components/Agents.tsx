// components/Agents.tsx — agent list with status badges and controls.
//
// Fetches /api/v1/agents on interval, shows cards with role/CLI/state,
// spawn button, kill button. Status badges color-coded by state.

import { useState, useEffect, useCallback } from "react";

interface Agent {
  id: string;
  role: string;
  cli: string;
  state: string;
  task_id: string;
  created_at: string;
}

const STATE_STYLE: Record<string, string> = {
  running: "bg-emerald-500/20 text-emerald-400 border-emerald-500/40",
  idle:    "bg-amber-500/20 text-amber-400 border-amber-500/40",
  killed:  "bg-rose-500/20 text-rose-400 border-rose-500/40",
  error:   "bg-red-500/20 text-red-400 border-red-500/40",
  done:    "bg-sky-500/20 text-sky-400 border-sky-500/40",
};

const ROLE_ICON: Record<string, string> = {
  implementer: "⌨",
  reviewer:    "👁",
  architect:   "🏗",
  tester:      "🧪",
  planner:     "📋",
  orchestrator: "🎼",
  hermes:      "🕊",
  researcher:  "🔍",
};

export default function Agents() {
  const [agents, setAgents] = useState<Agent[]>([]);
  const [spawning, setSpawning] = useState(false);
  const [spawnRole, setSpawnRole] = useState("implementer");
  const [error, setError] = useState("");

  const fetchAgents = useCallback(async () => {
    try {
      const r = await fetch("/api/v1/agents");
      const body = await r.json();
      setAgents(body.agents || []);
    } catch { /* ignore */ }
  }, []);

  useEffect(() => {
    fetchAgents();
    const id = setInterval(fetchAgents, 3000);
    return () => clearInterval(id);
  }, [fetchAgents]);

  const handleSpawn = async () => {
    setSpawning(true);
    setError("");
    try {
      const r = await fetch("/api/v1/agents", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ role: spawnRole, cli: "bash", task: "awaiting instructions" }),
      });
      if (!r.ok) {
        const b = await r.json();
        setError(b.error || "spawn failed");
      } else {
        setTimeout(fetchAgents, 500);
      }
    } catch (e: any) {
      setError(e.message);
    } finally {
      setSpawning(false);
    }
  };

  const handleKill = async (id: string) => {
    await fetch(`/api/v1/agents/${id}/kill`, { method: "POST" });
    setTimeout(fetchAgents, 500);
  };

  const active = agents.filter((a) => a.state === "running" || a.state === "idle");
  const done = agents.filter((a) => a.state !== "running" && a.state !== "idle");

  return (
    <div className="flex flex-col h-full gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Agents</h2>
        <span className="text-xs text-zinc-500">{active.length} attivi</span>
      </header>

      {/* spawn bar */}
      <div className="flex gap-1.5">
        <select
          className="flex-1 bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-xs"
          value={spawnRole}
          onChange={(e) => setSpawnRole(e.target.value)}
        >
          {Object.keys(ROLE_ICON).map((r) => (
            <option key={r} value={r}>{ROLE_ICON[r]} {r}</option>
          ))}
        </select>
        <button
          onClick={handleSpawn}
          disabled={spawning}
          className="bg-zinc-800 hover:bg-zinc-700 disabled:opacity-50 text-xs px-3 py-1 rounded border border-zinc-700"
        >
          {spawning ? "..." : "+ spawn"}
        </button>
      </div>
      {error && <p className="text-xs text-rose-400">{error}</p>}

      {/* active agents */}
      <ul className="flex-1 overflow-y-auto space-y-1.5">
        {active.length === 0 && (
          <li className="text-xs text-zinc-600 py-4 text-center">— nessun agente attivo —</li>
        )}
        {active.map((a) => (
          <li
            key={a.id}
            className="bg-zinc-950 border border-zinc-800 rounded p-2"
          >
            <div className="flex items-center justify-between gap-2">
              <div className="flex items-center gap-2 min-w-0">
                <span className="text-base">{ROLE_ICON[a.role] || "🤖"}</span>
                <div className="min-w-0">
                  <div className="text-xs font-mono truncate">{a.id.slice(0, 16)}</div>
                  <div className="text-[10px] text-zinc-500">{a.role} · {a.cli}</div>
                </div>
              </div>
              <div className="flex items-center gap-1.5">
                <span className={
                  "text-[10px] px-1.5 py-0.5 rounded border " +
                  (STATE_STYLE[a.state] || "bg-zinc-800 text-zinc-400 border-zinc-700")
                }>
                  {a.state}
                </span>
                <button
                  onClick={() => handleKill(a.id)}
                  className="text-[10px] text-rose-500 hover:text-rose-400 px-1"
                  title="kill"
                >
                  ✕
                </button>
              </div>
            </div>
            {a.task_id && (
              <div className="text-[10px] text-zinc-600 mt-1 font-mono truncate">
                task: {a.task_id}
              </div>
            )}
          </li>
        ))}
      </ul>

      {/* completed agents (collapsed) */}
      {done.length > 0 && (
        <details className="text-xs">
          <summary className="text-zinc-500 cursor-pointer">{done.length} completati</summary>
          <ul className="mt-1 space-y-1">
            {done.slice(0, 20).map((a) => (
              <li key={a.id} className="text-zinc-600 font-mono text-[10px] flex gap-2">
                <span className={
                  "px-1 rounded " + (STATE_STYLE[a.state] || "")
                }>
                  {a.state}
                </span>
                <span className="truncate">{a.id.slice(0, 20)}</span>
              </li>
            ))}
          </ul>
        </details>
      )}
    </div>
  );
}
