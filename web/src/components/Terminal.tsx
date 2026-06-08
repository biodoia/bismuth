// components/Terminal.tsx — remote terminal panel (center column of App).
//
// Uses xterm.js with a WebSocket attachment. V1: read-only display of
// the selected agent's pane scrollback. V2: bidirectional with stdin.

import { useEffect, useRef, useState } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import type { Agent } from "../lib/types";
import { listAgents, readAgent } from "../lib/api";

export default function Terminal() {
  const ref = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  const [agents, setAgents] = useState<Agent[]>([]);
  const [selected, setSelected] = useState<string>("");
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
      disableStdin: true,
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
      } catch (e: any) {
        setError(e.message);
      }
    })();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Load scrollback when selected changes
  useEffect(() => {
    if (!termRef.current || !selected) return;
    termRef.current.clear();
    (async () => {
      try {
        const data = await readAgent(selected, 200);
        const text = (data.scrollback || "").toString();
        if (!text) {
          termRef.current?.writeln("\x1b[2m— pane vuota —\x1b[0m");
          return;
        }
        termRef.current?.write(text);
      } catch (e: any) {
        setError(e.message);
      }
    })();
  }, [selected]);

  // TODO(V2): live tail via WebSocket pane_output subscription
  useEffect(() => {
    const t = setInterval(async () => {
      if (!termRef.current || !selected) return;
      try {
        const data = await readAgent(selected, 50);
        const text = (data.scrollback || "").toString();
        termRef.current.clear();
        termRef.current.write(text || "\x1b[2m— pane vuota —\x1b[0m");
      } catch {}
    }, 3000);
    return () => clearInterval(t);
  }, [selected]);

  return (
    <div className="flex flex-col h-full gap-2">
      <header className="flex items-center justify-between">
        <h2 className="text-sm font-semibold">Terminale</h2>
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
      </header>
      {error && <p className="text-xs text-rose-400">{error}</p>}
      <div ref={ref} className="flex-1 min-h-0 bg-zinc-950 rounded border border-zinc-800" />
      <p className="text-[10px] text-zinc-700">
        read-only V1 · polling 3s · V2 = WS attach + bidirezionale
      </p>
    </div>
  );
}
