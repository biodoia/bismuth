// components/Terminal.tsx — remote terminal panel (center column of App).
//
// Uses xterm.js with a WebSocket attachment that streams
// pane_output events in real time. No polling. Bidirectional stdin
// is V2 (currently disabledStdin=true on xterm).
//
// On agent selection change, the WS is resubscribed with the new
// agent_id filter so only that agent's output streams in.

import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import type { Agent, Event } from "../lib/types";
import { listAgents, sendToAgent } from "../lib/api";

export default function Terminal() {
  const ref = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const bufferRef = useRef<string>("");
  const selectedRef = useRef<string>("");
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState<string>("");
  const [status, setStatus] = useState<"connecting" | "live" | "offline">("offline");
  const [error, setError] = useState<string | null>(null);

  // Init xterm once
  useEffect(() => {
    if (!ref.current || termRef.current) return;
    const term = new XTerm({
      theme: { background: "#09090b", foreground: "#d4d4d8", cursor: "#22c55e" },
      fontFamily: "ui-monospace, monospace",
      fontSize: 12,
      convertEol: true,
      cursorBlink: true,
      // V1.1: stdin enabled for writeback. xterm.js requires the
      // terminal to be focused for onData to fire. We enable it
      // here and forward keystrokes to the worker via sendToAgent.
      disableStdin: false,
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(ref.current);
    fit.fit();
    termRef.current = term;
    fitRef.current = fit;
    term.writeln("\x1b[2m— nessun agent selezionato —\x1b[0m");
    const onResize = () => fit.fit();
    window.addEventListener("resize", onResize);
    // onData fires for every keypress; forward to selected agent
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
    };
  }, []);

  // Fetch agents on mount
  useEffect(() => {
    (async () => {
      try {
        const { agents } = await listAgents();
        setAgents(agents || []);
        if (agents?.length && !selected) setSelected(agents[0].id);
      } catch (e: unknown) {
        setError((e as Error)?.message || "load failed");
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Live WS attach whenever selected changes
  useEffect(() => {
    if (!selected) return;
    setStatus("connecting");
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const url = `${proto}//${window.location.host}/api/v1/ws?types=pane_output&agent_id=${encodeURIComponent(selected)}`;

    const term = termRef.current;
    if (!term) return;
    term.clear();
    term.writeln(`\x1b[2m— subscribing to ${selected}…\x1b[0m`);

    const ws = new WebSocket(url);
    ws.onopen = () => {
      setStatus("live");
      setError(null);
      // Stash the selected agent in a ref so onData can read it
      selectedRef.current = selected;
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
        // V1 WS doesn't carry raw bytes; the server only sends
        // byte counts. To get the real bytes we'd need a separate
        // /v1/pane/:id/stream endpoint (V2). For now, show a tick
        // so the user knows new output arrived.
        if (e.type === "pane_output") {
          const bytes = (e.payload as { bytes?: number })?.bytes ?? 0;
          bufferRef.current += `[pane_output +${bytes}B] `;
          // truncate buffer display to last 200 chars
          if (bufferRef.current.length > 200) {
            bufferRef.current = bufferRef.current.slice(-200);
          }
          term.writeln(`\x1b[2m${bufferRef.current.trim()}\x1b[0m`);
        }
      } catch {
        // ignore malformed
      }
    };
    return () => {
      ws.close();
    };
  }, [selected]);

  return (
    <div className="flex flex-col h-full gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Terminale</h2>
        <div className="flex items-center gap-2">
          <span
            className={
              "text-[10px] " +
              (status === "live" ? "text-emerald-400" : status === "connecting" ? "text-amber-300" : "text-rose-400")
            }
          >
            ● {status}
          </span>
          <select
            className="bg-zinc-950 border border-zinc-800 rounded px-2 py-1 text-xs"
            value={selected}
            onChange={(e) => setSelected(e.target.value)}
          >
            <option value="">— seleziona agent —</option>
            {agents.map((a) => (
              <option key={a.id} value={a.id}>
                {a.name} ({a.role}, {a.state})
              </option>
            ))}
          </select>
        </div>
      </header>
      {error && <p className="text-xs text-rose-400">{error}</p>}
      <div ref={ref} className="flex-1 min-h-0 bg-zinc-950 rounded border border-zinc-800" />
      <p className="text-[10px] text-zinc-700">
        WS live attach (V1 shows pane_output events; V2 streams raw bytes)
      </p>
    </div>
  );
}
