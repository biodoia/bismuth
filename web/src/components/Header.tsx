// components/Header.tsx — top bar 40px with bismuth branding, ws-status pulsing dot.
// Uses wireframe v1 design system colors.

import { useBismuthWS } from "../hooks/useWebSocket";

export default function Header() {
  const { connected } = useBismuthWS({});

  return (
    <header
      className="flex items-center justify-between px-4 border-b bg-[#0f1011]"
      style={{ height: 40, borderColor: "rgba(255,255,255,0.06)" }}
    >
      {/* Left: brand */}
      <div className="flex items-center gap-3">
        <span className="text-sm font-semibold tracking-tight text-[#ededed]">
          ◈ bismuth
        </span>
        <span className="text-[10px] text-[#555]">multi-agent multiplexer</span>
      </div>

      {/* Right: ws status + links */}
      <div className="flex items-center gap-4 text-xs">
        {/* WS status with pulsing dot */}
        <span className="flex items-center gap-1.5">
          <span
            className={`inline-block w-1.5 h-1.5 rounded-full ${
              connected
                ? "bg-[#22C55E] ws-pulse"
                : "bg-[#EF4444]"
            }`}
          />
          <span className={connected ? "text-[#22C55E]" : "text-[#EF4444]"}>
            {connected ? "live" : "offline"}
          </span>
        </span>

        <a
          href="/api/v1/events"
          className="text-[#555] hover:text-[#ededed] transition-colors"
          title="Event stream (SSE)"
        >
          events
        </a>
        <a
          href="/metrics"
          className="text-[#555] hover:text-[#ededed] transition-colors"
          title="Prometheus metrics"
        >
          metrics
        </a>
      </div>
    </header>
  );
}
