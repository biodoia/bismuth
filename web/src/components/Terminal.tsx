// components/Terminal.tsx — center panel with agent tabs + xterm.js.
// Wireframe v1 design system. Keeps xterm.js for the terminal but redesigns the wrapper.

import { useEffect, useRef, useState, useCallback } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import type { Agent, Event } from "../lib/types";
import { sendToAgent } from "../lib/api";

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
  const [status, setStatus] = useState<"connecting" | "live" | "offline">("offline");
  const [error, setError] = useState<string | null>(null);
  const [overflowTabs, setOverflowTabs] = useState(false);
  const tabsRef = useRef<HTMLDivElement>(null);

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
    term.writeln("\x1b[2m— no agent selected —\x1b[0m");

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

  // Live WS attach whenever selected changes
  useEffect(() => {
    if (!selectedAgentId) return;
    setStatus("connecting");
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${proto}//${window.location.host}/api/v1/ws?types=pane_output&agent_id=${encodeURIComponent(selectedAgentId)}`;

    const term = termRef.current;
    if (!term) return;
    term.clear();
    term.writeln(`\x1b[2m— connecting to ${selectedAgentId.slice(0, 12)}… —\x1b[0m`);

    const ws = new WebSocket(url);
    ws.onopen = () => {
      setStatus("live");
      setError(null);
      selectedRef.current = selectedAgentId;
    };
    ws.onclose = () => {
      setStatus("offline");
    };
    ws.onerror = () => {
      setError("ws error");
    };
    ws.onmessage = (ev) => {
      try {
        const e: Event = JSON.parse(ev.data);
        if (e.type === "pane_output") {
          const bytes = (e.payload as { bytes?: number })?.bytes ?? 0;
          term.writeln(`\x1b[2m[pane_output +${bytes}B]\x1b[0m`);
        }
      } catch {
        /* ignore */
      }
    };
    return () => {
      ws.close();
    };
  }, [selectedAgentId]);

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

        {/* Status indicator */}
        <div className="flex items-center gap-2 px-3 shrink-0">
          <span
            className="inline-block w-1.5 h-1.5 rounded-full"
            style={{
              background:
                status === "live"
                  ? "#22C55E"
                  : status === "connecting"
                  ? "#EAB308"
                  : "#555",
            }}
          />
          <span
            className="text-[10px]"
            style={{
              color:
                status === "live"
                  ? "#22C55E"
                  : status === "connecting"
                  ? "#EAB308"
                  : "#555",
            }}
          >
            {status}
          </span>
        </div>
      </div>

      {error && (
        <div className="px-3 py-1 text-[10px] text-[#EF4444] bg-[rgba(239,68,68,0.08)]">
          {error}
        </div>
      )}

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
        <span>WS live attach</span>
        {overflowTabs && <span>→ scroll tabs</span>}
      </div>
    </div>
  );
}
