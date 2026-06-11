// components/Audit.tsx — tamper-evident audit trail view (P7-h).
//
// Renders GET /api/v1/audit?limit=100&offset=0 (newest first) as a
// table: ts | actor | action | target | payload (truncated, click to
// expand) | row_hash (short prefix, full hash in tooltip).
// Manual refresh button + auto-refresh every 10s.
// Wireframe v1 design system.

import { useCallback, useEffect, useState } from "react";
import { getAudit } from "../lib/api";
import type { AuditEntry } from "../lib/types";

const ACTION_COLOR: Record<string, string> = {
  spawn_agent: "#22C55E",
  kill_agent: "#EF4444",
  denied_kill: "#EF4444",
  assign_task: "#3B82F6",
  create_task: "#3B82F6",
  merge_task: "#A855F7",
  voice_stt: "#A855F7",
  voice_command: "#A855F7",
  send: "#06B6D4",
};

// payloadText normalizes the payload (JSON string server-side, but be
// liberal) into a displayable string.
function payloadText(p: unknown): string {
  if (p == null || p === "") return "";
  if (typeof p === "string") {
    try {
      return JSON.stringify(JSON.parse(p));
    } catch {
      return p;
    }
  }
  try {
    return JSON.stringify(p);
  } catch {
    return String(p);
  }
}

function payloadPretty(p: unknown): string {
  if (p == null || p === "") return "—";
  if (typeof p === "string") {
    try {
      return JSON.stringify(JSON.parse(p), null, 2);
    } catch {
      return p;
    }
  }
  try {
    return JSON.stringify(p, null, 2);
  } catch {
    return String(p);
  }
}

function fmtTS(ts: string): string {
  // 2026-06-10T12:34:56.789Z → "06-10 12:34:56"
  if (ts.length >= 19) return `${ts.slice(5, 10)} ${ts.slice(11, 19)}`;
  return ts;
}

export default function Audit() {
  const [entries, setEntries] = useState<AuditEntry[]>([]);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [lastFetch, setLastFetch] = useState<string>("");

  const refresh = useCallback(async () => {
    setLoading(true);
    try {
      const body = await getAudit(100, 0);
      setEntries(body.entries || []);
      setError(null);
      setLastFetch(new Date().toLocaleTimeString());
    } catch (e: unknown) {
      setError((e as Error)?.message || "audit load failed");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    refresh();
    const id = setInterval(refresh, 10_000);
    return () => clearInterval(id);
  }, [refresh]);

  return (
    <div className="flex flex-col h-full min-h-0">
      {/* Section header */}
      <div
        className="flex items-center justify-between px-3 py-2 border-b shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <div className="flex items-center gap-3">
          <h2 className="text-xs font-semibold text-[#ededed] uppercase tracking-wider">
            Audit
          </h2>
          <span className="text-[10px] text-[#555]">
            tamper-evident trail · last 100 · newest first
          </span>
        </div>
        <div className="flex items-center gap-3">
          {lastFetch && (
            <span className="text-[10px] text-[#555]">
              {lastFetch} · auto 10s
            </span>
          )}
          <button
            onClick={refresh}
            disabled={loading}
            className="text-[10px] px-2.5 py-1 rounded font-medium bg-[rgba(0,212,170,0.15)] text-[#00D4AA] border border-[rgba(0,212,170,0.25)] hover:bg-[rgba(0,212,170,0.25)] disabled:opacity-40 transition-colors"
          >
            {loading ? "…" : "↻ refresh"}
          </button>
        </div>
      </div>

      {error && (
        <div className="px-3 py-1 text-[10px] text-[#EF4444] bg-[rgba(239,68,68,0.08)] shrink-0">
          {error}
        </div>
      )}

      {/* Table */}
      <div className="flex-1 overflow-auto min-h-0">
        <table className="w-full text-[11px]" style={{ borderCollapse: "collapse" }}>
          <thead>
            <tr
              className="text-left text-[10px] uppercase tracking-wider text-[#555] sticky top-0"
              style={{ background: "#0f1011" }}
            >
              <th className="px-3 py-2 font-medium w-28">ts</th>
              <th className="px-3 py-2 font-medium w-32">actor</th>
              <th className="px-3 py-2 font-medium w-28">action</th>
              <th className="px-3 py-2 font-medium w-40">target</th>
              <th className="px-3 py-2 font-medium">payload</th>
              <th className="px-3 py-2 font-medium w-24">hash</th>
            </tr>
          </thead>
          <tbody>
            {entries.length === 0 && !error && (
              <tr>
                <td colSpan={6} className="px-3 py-8 text-center text-[10px] text-[#555]">
                  — no audit entries —
                </td>
              </tr>
            )}
            {entries.map((e) => {
              const isExpanded = expanded === e.seq;
              const ptext = payloadText(e.payload);
              const truncated = ptext.length > 96 ? `${ptext.slice(0, 96)}…` : ptext;
              const actionColor = ACTION_COLOR[e.action] || "#ededed";
              return (
                <tr
                  key={e.seq}
                  onClick={() => setExpanded(isExpanded ? null : e.seq)}
                  className="cursor-pointer align-top transition-colors border-b hover:bg-[#161718]"
                  style={{
                    borderColor: "rgba(255,255,255,0.04)",
                    background: isExpanded ? "#161718" : "transparent",
                  }}
                >
                  <td
                    className="px-3 py-1.5 text-[#888] whitespace-nowrap"
                    style={{ fontFamily: "'JetBrains Mono', monospace" }}
                    title={e.ts}
                  >
                    {fmtTS(e.ts)}
                  </td>
                  <td className="px-3 py-1.5 text-[#888] truncate max-w-32" title={e.actor}>
                    {e.actor || "—"}
                  </td>
                  <td className="px-3 py-1.5 font-medium" style={{ color: actionColor }}>
                    {e.action}
                  </td>
                  <td
                    className="px-3 py-1.5 text-[#888] truncate max-w-40"
                    style={{ fontFamily: "'JetBrains Mono', monospace" }}
                    title={e.target}
                  >
                    {e.target || "—"}
                  </td>
                  <td className="px-3 py-1.5 text-[#888]">
                    {isExpanded ? (
                      <pre
                        className="whitespace-pre-wrap break-all text-[10px] text-[#aaa] max-h-64 overflow-y-auto"
                        style={{ fontFamily: "'JetBrains Mono', monospace" }}
                      >
                        {payloadPretty(e.payload)}
                      </pre>
                    ) : (
                      <span
                        className="text-[10px] break-all"
                        style={{ fontFamily: "'JetBrains Mono', monospace" }}
                      >
                        {truncated || "—"}
                        {ptext.length > 96 && (
                          <span className="text-[#555] ml-1">▸</span>
                        )}
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-1.5">
                    <span
                      className="text-[10px] text-[#555] px-1.5 py-0.5 rounded bg-[#161718] cursor-help"
                      style={{ fontFamily: "'JetBrains Mono', monospace" }}
                      title={e.row_hash}
                    >
                      {e.row_hash ? e.row_hash.slice(0, 8) : "—"}
                    </span>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>

      {/* Footer */}
      <div
        className="flex items-center justify-between px-3 py-1.5 border-t text-[10px] text-[#555] shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <span>{entries.length} entries</span>
        <span>
          seq #{entries.length > 0 ? entries[0].seq : 0} · hash chain sha256
        </span>
      </div>
    </div>
  );
}
