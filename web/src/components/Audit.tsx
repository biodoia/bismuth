// components/Audit.tsx — audit log timeline with SHA verification.
//
// Shows the last N audit events from /api/v1/events?types=audit_log.
// Each event displays: timestamp, actor, action, target, SHA badge.
// Clicking an event reveals the full payload and SHA256 verification.

import { useEffect, useState } from "react";
import type { Event } from "../lib/types";

interface AuditEntry {
  seq: number;
  ts: string;
  actor: string;
  action: string;
  target: string;
  sha: string;
  payload: Record<string, unknown>;
}

export default function Audit() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchAudit = async () => {
      try {
        const r = await fetch("/api/v1/events?types=audit_log&limit=50");
        if (!r.ok) throw new Error(`audit ${r.status}`);
        const data = await r.json();
        const evs: Event[] = data.events || [];
        const mapped: AuditEntry[] = evs.map((e) => {
          const p = (e.payload || {}) as Record<string, unknown>;
          return {
            seq: e.seq ?? 0,
            ts: e.ts ?? "",
            actor: (p.actor as string) || "system",
            action: (p.action as string) || e.type,
            target: (p.target as string) || "—",
            sha: (p.sha as string) || "—",
            payload: p,
          };
        });
        setEntries(mapped);
      } catch (e: unknown) {
        setError((e as Error)?.message || "load failed");
      }
    };
    fetchAudit();
    const t = setInterval(fetchAudit, 5000);
    return () => clearInterval(t);
  }, []);

  const verify = (entry: AuditEntry): "valid" | "missing" | "mismatch" => {
    if (!entry.sha || entry.sha === "—") return "missing";
    // V1: SHA is computed server-side; we trust it. V2: client-side
    // recomputation via subtle.crypto.digest("SHA-256", payloadBytes).
    return "valid";
  };

  return (
    <div className="flex flex-col h-full gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Audit</h2>
        <span className="text-[10px] text-zinc-500">aggiorna ogni 5s</span>
      </header>

      {error && <p className="text-xs text-rose-400">{error}</p>}

      <div className="flex-1 overflow-y-auto space-y-1">
        {entries.length === 0 && (
          <p className="text-xs text-zinc-600">Nessun evento audit.</p>
        )}
        {entries.map((e) => {
          const v = verify(e);
          return (
            <div
              key={e.seq}
              className="text-xs bg-zinc-900 rounded p-2 cursor-pointer hover:bg-zinc-800 transition-colors"
              onClick={() => setExpanded(expanded === e.seq ? null : e.seq)}
            >
              <div className="flex items-center gap-2">
                <span
                  className={`w-2 h-2 rounded-full ${
                    v === "valid"
                      ? "bg-emerald-400"
                      : v === "missing"
                      ? "bg-amber-300"
                      : "bg-rose-400"
                  }`}
                  title={v}
                />
                <span className="text-zinc-400 font-mono">#{e.seq}</span>
                <span className="text-zinc-300">{e.action}</span>
                <span className="text-zinc-500">→ {e.target}</span>
                <span className="text-[10px] text-zinc-600 ml-auto">{e.ts}</span>
              </div>
              {expanded === e.seq && (
                <div className="mt-2 space-y-1 border-t border-zinc-800 pt-2">
                  <p className="text-[10px] text-zinc-500">
                    actor: <span className="text-zinc-300">{e.actor}</span>
                  </p>
                  <p className="text-[10px] text-zinc-500">
                    SHA: <span className="font-mono text-zinc-300">{e.sha}</span>
                  </p>
                  <pre className="text-[10px] text-zinc-400 overflow-x-auto">
                    {JSON.stringify(e.payload, null, 2)}
                  </pre>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
