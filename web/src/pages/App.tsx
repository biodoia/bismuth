// pages/App.tsx — 3-column grid layout: 240px sidebar | 1fr terminal | 300px feed.
// Header 40px on top. Desktop zones: Agents+Voice | Terminal | Tasks+Feed,
// plus an Audit view reachable from the Header. Mobile: vertical stack
// with tabs. Wireframe v1 design system.
//
// P7-f: heavy zones are code-split — Terminal (xterm + addons), Voice
// (vad-web + onnxruntime) and Audit load lazily via React.lazy/Suspense
// so none of them ship in the entry chunk. Voice additionally mounts
// only when its drawer is opened (the VAD/onnx chunk is ~MB-scale).

import { useState, useEffect, lazy, Suspense } from "react";
import Header, { type View } from "../components/Header";
import Agents from "../components/Agents";
import Feed from "../components/Feed";
import Tasks from "../components/Tasks";
import { useBismuthStore } from "../lib/store";

const Terminal = lazy(() => import("../components/Terminal"));
const Voice = lazy(() => import("../components/Voice"));
const Audit = lazy(() => import("../components/Audit"));

type Tab = "agents" | "terminal" | "voice" | "feed" | "audit";

const TABS: { key: Tab; label: string }[] = [
  { key: "agents", label: "agents" },
  { key: "terminal", label: "terminal" },
  { key: "voice", label: "voice" },
  { key: "feed", label: "feed" },
  { key: "audit", label: "audit" },
];

function PanelFallback({ label }: { label: string }) {
  return (
    <div className="flex items-center justify-center h-full text-[10px] text-[#555]">
      loading {label}…
    </div>
  );
}

// ToastOverlay — drag-and-drop assignment feedback (P7-i).
function ToastOverlay() {
  const toast = useBismuthStore((s) => s.toast);
  if (!toast) return null;
  const color =
    toast.kind === "success" ? "#22C55E" : toast.kind === "error" ? "#EF4444" : "#3B82F6";
  return (
    <div
      className="fixed bottom-4 right-4 z-50 flex items-center gap-2 px-3 py-2 rounded border text-[11px] shadow-lg"
      style={{
        background: "#161718",
        borderColor: "rgba(255,255,255,0.10)",
        color: "#ededed",
      }}
    >
      <span className="inline-block w-1.5 h-1.5 rounded-full" style={{ background: color }} />
      <span style={{ fontFamily: "'JetBrains Mono', monospace" }}>{toast.msg}</span>
    </div>
  );
}

export default function App() {
  const [tab, setTab] = useState<Tab>("agents");
  const [view, setView] = useState<View>("dashboard");
  const [voiceOpen, setVoiceOpen] = useState(false);
  const [selectedAgentId, setSelectedAgentId] = useState<string>("");

  const agents = useBismuthStore((s) => s.agents);
  const fetchAgents = useBismuthStore((s) => s.fetchAgents);

  // Poll agents into the shared store (SSE patches states in between).
  useEffect(() => {
    fetchAgents();
    const id = setInterval(fetchAgents, 3000);
    return () => clearInterval(id);
  }, [fetchAgents]);

  // Auto-select first active agent if none selected
  useEffect(() => {
    if (selectedAgentId) return;
    const active = agents.find((a) => a.state === "working" || a.state === "idle");
    if (active) setSelectedAgentId(active.id);
  }, [agents, selectedAgentId]);

  const handleSelectAgent = (id: string) => {
    setSelectedAgentId(id);
    // On mobile, switch to terminal tab when selecting an agent
    if (window.innerWidth < 768) {
      setTab("terminal");
    }
  };

  return (
    <div className="flex flex-col h-full w-full" style={{ background: "#08090a", color: "#ededed" }}>
      {/* Header 40px */}
      <Header view={view} onViewChange={setView} />

      {/* Desktop: audit view (full width) */}
      {view === "audit" && (
        <div className="hidden md:flex flex-1 min-h-0 flex-col">
          <Suspense fallback={<PanelFallback label="audit" />}>
            <Audit />
          </Suspense>
        </div>
      )}

      {/* Desktop: 3-column grid */}
      {view === "dashboard" && (
        <div
          className="hidden md:grid flex-1 min-h-0"
          style={{
            gridTemplateColumns: "240px 1fr 300px",
          }}
        >
          {/* Sidebar: Agents + Voice drawer */}
          <section
            className="min-h-0 flex flex-col border-r"
            style={{ borderColor: "rgba(255,255,255,0.06)" }}
          >
            <div className="flex-1 min-h-0">
              <Agents
                selectedAgentId={selectedAgentId}
                onSelectAgent={handleSelectAgent}
              />
            </div>

            {/* Voice zone (lazy: vad-web/onnx load on first open) */}
            <div
              className="shrink-0 border-t"
              style={{ borderColor: "rgba(255,255,255,0.06)" }}
            >
              <button
                onClick={() => setVoiceOpen((v) => !v)}
                className="w-full flex items-center justify-between px-3 py-2 text-xs font-semibold uppercase tracking-wider text-[#ededed] hover:bg-[#0f1011] transition-colors"
              >
                <span>Voice</span>
                <span className="text-[10px] text-[#555]">{voiceOpen ? "▾" : "▸"}</span>
              </button>
              {voiceOpen && (
                <div style={{ height: 280 }} className="min-h-0">
                  <Suspense fallback={<PanelFallback label="voice" />}>
                    <Voice />
                  </Suspense>
                </div>
              )}
            </div>
          </section>

          {/* Center: Terminal */}
          <section
            className="min-h-0 flex flex-col border-r"
            style={{ borderColor: "rgba(255,255,255,0.06)" }}
          >
            <Suspense fallback={<PanelFallback label="terminal" />}>
              <Terminal
                selectedAgentId={selectedAgentId}
                onSelectAgent={handleSelectAgent}
                agents={agents}
              />
            </Suspense>
          </section>

          {/* Right: Tasks + Feed */}
          <section className="min-h-0 flex flex-col">
            <div
              className="min-h-0 border-b"
              style={{ height: "38%", borderColor: "rgba(255,255,255,0.06)" }}
            >
              <Tasks />
            </div>
            <div className="flex-1 min-h-0">
              <Feed />
            </div>
          </section>
        </div>
      )}

      {/* Mobile: tabs + single panel */}
      <div className="flex flex-col flex-1 min-h-0 md:hidden">
        {/* Mobile tab bar */}
        <nav
          className="flex shrink-0 border-b"
          style={{ borderColor: "rgba(255,255,255,0.06)" }}
        >
          {TABS.map((t) => (
            <button
              key={t.key}
              onClick={() => setTab(t.key)}
              className="flex-1 py-2.5 text-[11px] font-medium transition-colors border-b-2"
              style={{
                borderColor: tab === t.key ? "#00D4AA" : "transparent",
                color: tab === t.key ? "#ededed" : "#555",
                background: tab === t.key ? "#161718" : "transparent",
              }}
            >
              {t.label}
            </button>
          ))}
        </nav>

        {/* Mobile content */}
        <main className="flex-1 min-h-0 flex flex-col">
          {tab === "agents" && (
            <Agents
              selectedAgentId={selectedAgentId}
              onSelectAgent={handleSelectAgent}
            />
          )}
          {tab === "terminal" && (
            <Suspense fallback={<PanelFallback label="terminal" />}>
              <Terminal
                selectedAgentId={selectedAgentId}
                onSelectAgent={handleSelectAgent}
                agents={agents}
              />
            </Suspense>
          )}
          {tab === "voice" && (
            <Suspense fallback={<PanelFallback label="voice" />}>
              <Voice />
            </Suspense>
          )}
          {tab === "feed" && (
            <>
              <div
                className="min-h-0 border-b"
                style={{ height: "38%", borderColor: "rgba(255,255,255,0.06)" }}
              >
                <Tasks />
              </div>
              <div className="flex-1 min-h-0">
                <Feed />
              </div>
            </>
          )}
          {tab === "audit" && (
            <Suspense fallback={<PanelFallback label="audit" />}>
              <Audit />
            </Suspense>
          )}
        </main>
      </div>

      <ToastOverlay />
    </div>
  );
}
