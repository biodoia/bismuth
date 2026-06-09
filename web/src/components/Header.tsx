// components/Header.tsx — top bar with bismuth branding, user info, connection status.

import { useBismuthWS } from "../hooks/useWebSocket";

export default function Header() {
  const { connected } = useBismuthWS({});

  return (
    <header className="flex items-center justify-between px-3 py-1.5 border-b border-zinc-800 bg-zinc-950">
      <div className="flex items-center gap-2">
        <span className="text-sm font-bold tracking-tight">⬡ bismuth</span>
        <span className="text-[10px] text-zinc-600">multi-agent multiplexer</span>
      </div>
      <div className="flex items-center gap-3 text-xs">
        <span className="text-zinc-500">
          {connected ? (
            <span className="text-emerald-400">● ws</span>
          ) : (
            <span className="text-rose-400">○ ws</span>
          )}
        </span>
        <a
          href="/api/v1/events"
          className="text-zinc-500 hover:text-zinc-300"
          title="Event stream"
        >
          events
        </a>
        <a
          href="/metrics"
          className="text-zinc-500 hover:text-zinc-300"
          title="Prometheus metrics"
        >
          metrics
        </a>
      </div>
    </header>
  );
}
