// components/Terminal.tsx — center panel with agent tabs + xterm.js.
// Wireframe v1 design system.
//
// Live output (P7-j): on agent select we backfill scrollback once via
// GET /read?n=200, then follow the SSE stream /agents/:id/stream
// (event `output` → base64 chunk → xterm.write). If the EventSource
// errors it is closed and we fall back to polling /read every 2s.
// Keystrokes are forwarded via POST /send.

import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import type { Agent } from "../lib/types";
import { sendToAgent, readAgent } from "../lib/api";
import { useAgentStream } from "../hooks/useAgentStream";

interface TerminalProps {
  selectedAgentId: string | null;
  onSelectAgent: (id: string) => void;
  agents: Agent[];
}

export default function Terminal({ selectedAgentId, onSelectAgent, agents }: TerminalProps) {
  const ref = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const selectedRef = useRef<string>("");
  // While backfilling /read, SSE chunks are buffered here (null = live).
  const pendingRef = useRef<Uint8Array[] | null>(null);
  const lastScrollbackRef = useRef<string>("");
  const [overflowTabs, setOverflowTabs] = useState(false);
  const tabsRef = useRef<HTMLDivElement>(null);

  // SSE live stream (preferred transport). Also patches the agent's
  // state badge in the store on `state` events (inside the hook).
  const { status: sseStatus } = useAgentStream(selectedAgentId || null, {
    onOutput: (bytes) => {
      const term = termRef.current;
      if (!term) return;
      if (pendingRef.current) {
        pendingRef.current.push(bytes); // backfill in flight — keep order
      } else {
        term.write(bytes);
      }
    },
  });

  // Init xterm once
  useEffect(() => {
    if (!ref.current || termRef.current) return;
    const term = new XTerm({
      theme: {
        background: "#08090a",
        foreground: "#ededed",
        cursor: "#00D4AA",
        cursorAccent: "#08090a",
        selectionBackground: "rgba(0,212,170,0.2)",
        black: "#161718",
        red: "#EF4444",
        green: "#22C55E",
        yellow: "#EAB308",
        blue: "#3B82F6",
        magenta: "#A855F7",
        cyan: "#06B6D4",
        white: "#ededed",
        brightBlack: "#555",
        brightRed: "#F87171",
        brightGreen: "#4ADE80",
        brightYellow: "#FACC15",
        brightBlue: "#60A5FA",
        brightMagenta: "#C084FC",
        brightCyan: "#22D3EE",
        brightWhite: "#ededed",
      },
      fontFamily: "'JetBrains Mono', monospace",
      fontSize: 12,
      convertEol: true,
      cursorBlink: true,
      disableStdin: false,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(ref.current);
    fit.fit();
    termRef.current = term;
    fitRef.current = fit;

    const onResize = () => {
      fit.fit();
    };
    window.addEventListener("resize", onResize);

    // Forward keystrokes to selected agent
    term.onData((data) => {
      const ag = selectedRef.current;
      if (!ag) return;
      sendToAgent(ag, data).catch((err: unknown) => {
        term.writeln(`\x1b[31m[send error: ${(err as Error)?.message || "fail"}]\x1b[0m`);
      });
    });

    return () => {
      window.removeEventListener("resize", onResize);
      term.dispose();
      termRef.current = null;
    };
  }, []);

  // Check tab overflow
  useEffect(() => {
    if (tabsRef.current) {
      setOverflowTabs(tabsRef.current.scrollWidth > tabsRef.current.clientWidth);
    }
  }, [agents]);

  // Backfill scrollback whenever the selected agent changes, buffering
  // live SSE chunks until the snapshot has been written.
  useEffect(() => {
    const term = termRef.current;
    if (!term) return;
    if (!selectedAgentId) {
      selectedRef.current = "";
      term.reset();
      term.writeln("\x1b[2m— no agent selected —\x1b[0m");
      return;
    }
    selectedRef.current = selectedAgentId;
    pendingRef.current = [];
    lastScrollbackRef.current = "";
    term.reset();
    term.writeln(`\x1b[2m— attaching to ${selectedAgentId.slice(0, 12)}… —\x1b[0m`);

    let cancelled = false;
    readAgent(selectedAgentId, 200)
      .then((body) => {
        if (cancelled) return;
        const sb: string = body.scrollback || "";
        lastScrollbackRef.current = sb;
        term.reset();
        if (sb) term.write(sb);
      })
      .catch(() => {
        if (!cancelled) term.writeln("\x1b[31m[scrollback read failed]\x1b[0m");
      })
      .finally(() => {
        if (cancelled) return;
        // Flush chunks that arrived while backfilling, then go live.
        const pending = pendingRef.current;
        pendingRef.current = null;
        const t = termRef.current;
        if (t && pending) for (const b of pending) t.write(b);
      });

    return () => {
      cancelled = true;
      pendingRef.current = null;
    };
  }, [selectedAgentId]);

  // Fallback: SSE failed → poll /read and rewrite on change.
  useEffect(() => {
    if (!selectedAgentId || sseStatus !== "error") return;
    const poll = async () => {
      try {
        const body = await readAgent(selectedAgentId, 200);
        const sb: string = body.scrollback || "";
        if (sb === lastScrollbackRef.current) return;
        lastScrollbackRef.current = sb;
        const term = termRef.current;
        if (term && pendingRef.current === null) {
          term.reset();
          term.write(sb);
        }
      } catch {
        /* server unreachable — keep polling */
      }
    };
    poll();
    const id = setInterval(poll, 2000);
    return () => clearInterval(id);
  }, [selectedAgentId, sseStatus]);

  // Fit terminal when container resizes
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const obs = new ResizeObserver(() => {
      fitRef.current?.fit();
    });
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  const handleTabClose = useCallback(
    (e: React.MouseEvent, agentId: string) => {
      e.stopPropagation();
      if (selectedAgentId === agentId) {
        // Switch to another agent or deselect
        const remaining = agents.filter((a) => a.id !== agentId);
        if (remaining.length > 0) {
          onSelectAgent(remaining[0].id);
        } else {
          onSelectAgent("");
        }
      }
    },
    [selectedAgentId, agents, onSelectAgent]
  );

  const activeAgents = agents.filter(
    (a) => a.state === "working" || a.state === "idle" || a.state === "planning" || a.state === "reviewing"
  );

  // Transport status for the indicator: sse | polling | connecting | off.
  const transport = !selectedAgentId
    ? { label: "offline", color: "#555" }
    : sseStatus === "open"
    ? { label: "live · sse", color: "#22C55E" }
    : sseStatus === "connecting"
    ? { label: "connecting", color: "#EAB308" }
    : sseStatus === "error"
    ? { label: "polling", color: "#3B82F6" }
    : { label: "offline", color: "#555" };

  return (
    <div className="flex flex-col h-full">
      {/* Terminal header with tabs */}
      <div
        className="flex items-center border-b shrink-0"
        style={{ borderColor: "rgba(255,255,255,0.06)", height: 36 }}
      >
        {/* Agent tabs */}
        <div
          ref={tabsRef}
          className="flex-1 flex items-center overflow-x-auto min-w-0"
          style={{ scrollbarWidth: "none" }}
        >
          {activeAgents.length === 0 && (
            <span className="text-[10px] text-[#555] px-3">no agents</span>
          )}
          {activeAgents.map((a) => {
            const isActive = selectedAgentId === a.id;
            return (
              <button
                key={a.id}
                onClick={() => onSelectAgent(a.id)}
                className="flex items-center gap-1.5 px-3 text-[11px] border-b-2 shrink-0 transition-colors"
                style={{
                  height: 36,
                  borderColor: isActive ? "#00D4AA" : "transparent",
                  color: isActive ? "#ededed" : "#555",
                  background: isActive ? "#161718" : "transparent",
                }}
              >
                <span className="truncate max-w-[100px]" style={{ fontFamily: "'JetBrains Mono', monospace" }}>
                  {a.id.slice(0, 10)}
                </span>
                {isActive && (
                  <span
                    onClick={(e) => handleTabClose(e, a.id)}
                    className="text-[#555] hover:text-[#EF4444] transition-colors ml-1"
                  >
                    ×
                  </span>
                )}
              </button>
            );
          })}
        </div>

        {/* Transport status indicator */}
        <div className="flex items-center gap-2 px-3 shrink-0">
          <span
            className="inline-block w-1.5 h-1.5 rounded-full"
            style={{ background: transport.color }}
          />
          <span className="text-[10px]" style={{ color: transport.color }}>
            {transport.label}
          </span>
        </div>
      </div>

      {/* xterm container */}
      <div
        ref={ref}
        className="flex-1 min-h-0"
        style={{ background: "#08090a" }}
      />

      {/* Footer */}
      <div
        className="flex items-center justify-between px-3 py-1 border-t text-[10px] text-[#555]"
        style={{ borderColor: "rgba(255,255,255,0.06)" }}
      >
        <span>
          {sseStatus === "error"
            ? "SSE unavailable — polling /read every 2s"
            : "SSE live attach · /read backfill"}
        </span>
        {overflowTabs && <span>→ scroll tabs</span>}
      </div>
    </div>
  );
}
