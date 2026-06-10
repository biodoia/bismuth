// pages/App.tsx — 3-column grid layout: 240px sidebar | 1fr terminal | 300px feed.
// Header 40px on top. No Voice tab on desktop. Mobile: vertical stack with tabs.
// Wireframe v1 design system.

import { useState, useEffect, useCallback } from "react";
import Header from "../components/Header";
import Agents from "../components/Agents";
import Terminal from "../components/Terminal";
import Feed from "../components/Feed";
import type { Agent } from "../lib/types";

type Tab = "agents" | "terminal" | "feed";

const TABS: { key: Tab; label: string }[] = [
  { key: "agents", label: "agents" },
  { key: "terminal", label: "terminal" },
  { key: "feed", label: "feed" },
];

export default function App() {
  const [tab, setTab] = useState<Tab>("agents");
  const [selectedAgentId, setSelectedAgentId] = useState<string>("");
  const [agents, setAgents] = useState<Agent[]>([]);

  // Fetch agents for terminal tabs
  const fetchAgents = useCallback(async () => {
    try {
      const r = await fetch("/api/v1/agents");
      if (!r.ok) return;
      const body = await r.json();
      const list: Agent[] = body.agents || [];
      setAgents(list);
      // Auto-select first active agent if none selected
      if (!selectedAgentId && list.length > 0) {
        const active = list.find(
          (a) => a.state === "working" || a.state === "idle"
        );
        if (active) setSelectedAgentId(active.id);
      }
    } catch {
      /* ignore */
    }
  }, [selectedAgentId]);

  useEffect(() => {
    fetchAgents();
    const id = setInterval(fetchAgents, 3000);
    return () => clearInterval(id);
  }, [fetchAgents]);

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
      <Header />

      {/* Desktop: 3-column grid */}
      <div
        className="hidden md:grid flex-1 min-h-0"
        style={{
          gridTemplateColumns: "240px 1fr 300px",
        }}
      >
        {/* Sidebar: Agents */}
        <section
          className="min-h-0 flex flex-col border-r"
          style={{ borderColor: "rgba(255,255,255,0.06)" }}
        >
          <Agents
            selectedAgentId={selectedAgentId}
            onSelectAgent={handleSelectAgent}
          />
        </section>

        {/* Center: Terminal */}
        <section
          className="min-h-0 flex flex-col border-r"
          style={{ borderColor: "rgba(255,255,255,0.06)" }}
        >
          <Terminal
            selectedAgentId={selectedAgentId}
            onSelectAgent={handleSelectAgent}
            agents={agents}
          />
        </section>

        {/* Right: Feed */}
        <section className="min-h-0 flex flex-col">
          <Feed />
        </section>
      </div>

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
              className="flex-1 py-2.5 text-xs font-medium transition-colors border-b-2"
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
        <main className="flex-1 min-h-0">
          {tab === "agents" && (
            <Agents
              selectedAgentId={selectedAgentId}
              onSelectAgent={handleSelectAgent}
            />
          )}
          {tab === "terminal" && (
            <Terminal
              selectedAgentId={selectedAgentId}
              onSelectAgent={handleSelectAgent}
              agents={agents}
            />
          )}
          {tab === "feed" && <Feed />}
        </main>
      </div>
    </div>
  );
}
