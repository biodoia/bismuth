// components/Header.tsx — top bar 40px with bismuth branding, ws-status pulsing dot.
// Uses wireframe v1 design system colors. Hosts the dashboard/audit
// view switcher (P7-h) on desktop; mobile reaches audit via its tab.

import { useBismuthWS } from "../hooks/useWebSocket";

export type View = "dashboard" | "audit";

interface HeaderProps {
  view: View;
  onViewChange: (v: View) => void;
}

const VIEWS: { key: View; label: string }[] = [
  { key: "dashboard", label: "dashboard" },
  { key: "audit", label: "audit" },
];

export default function Header({ view, onViewChange }: HeaderProps) {
  const { connected } = useBismuthWS({});

  return (
    <header
      className="flex items-center justify-between px-4 border-b bg-[#0f1011]"
      style={{ height: 40, borderColor: "rgba(255,255,255,0.06)" }}
    >
      {/* Left: brand + view switcher */}
      <div className="flex items-center gap-3">
        <span className="text-sm font-semibold tracking-tight text-[#ededed]">
          ◈ bismuth
        </span>
        <span className="text-[10px] text-[#555] hidden sm:inline">multi-agent multiplexer</span>

        {/* View switcher (desktop) */}
        <nav className="hidden md:flex items-center gap-1 ml-3">
          {VIEWS.map((v) => (
            <button
              key={v.key}
              onClick={() => onViewChange(v.key)}
              className="text-[11px] px-2 py-1 rounded transition-colors"
              style={{
                color: view === v.key ? "#ededed" : "#555",
                background: view === v.key ? "#161718" : "transparent",
              }}
            >
              {v.label}
            </button>
          ))}
        </nav>
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
