// App.tsx — 4-zone layout with header (V2).
// Desktop: header + [agents | terminal | voice | feed].
// Mobile (< 768px): header + vertical stack with tabs.

import { useState } from "react";
import Header from "../components/Header";
import Agents from "../components/Agents";
import Voice from "../components/Voice";
import Terminal from "../components/Terminal";
import Feed from "../components/Feed";

type Tab = "agents" | "voice" | "terminal" | "feed";

const TABS: { key: Tab; label: string }[] = [
  { key: "agents", label: "agents" },
  { key: "voice", label: "voice" },
  { key: "terminal", label: "terminal" },
  { key: "feed", label: "feed" },
];

export default function App() {
  const [tab, setTab] = useState<Tab>("agents");

  return (
    <div className="flex flex-col h-full w-full bg-zinc-900 text-zinc-100">
      <Header />
      <DesktopLayout />
      <MobileLayout tab={tab} setTab={setTab} />
    </div>
  );
}

function DesktopLayout() {
  return (
    <div
      className="hidden md:grid flex-1 min-h-0"
      style={{ gridTemplateColumns: "260px 1fr 280px 320px", gap: "4px", padding: "4px" }}
    >
      <section className="panel p-2 min-h-0 flex flex-col" aria-label="agents">
        <Agents />
      </section>
      <section className="panel p-2 min-h-0 flex flex-col" aria-label="terminal">
        <Terminal />
      </section>
      <section className="panel p-2 min-h-0 flex flex-col" aria-label="voice">
        <Voice />
      </section>
      <section className="panel p-2 min-h-0 flex flex-col" aria-label="feed">
        <Feed />
      </section>
    </div>
  );
}

function MobileLayout({ tab, setTab }: { tab: Tab; setTab: (t: Tab) => void }) {
  return (
    <div className="flex flex-col flex-1 min-h-0 md:hidden">
      <nav className="flex border-b border-zinc-800 text-xs">
        {TABS.map((t) => (
          <button
            key={t.key}
            onClick={() => setTab(t.key)}
            className={
              "flex-1 py-2 " +
              (tab === t.key ? "bg-zinc-800 text-zinc-100" : "text-zinc-500")
            }
          >
            {t.label}
          </button>
        ))}
      </nav>
      <main className="flex-1 min-h-0 p-2">
        {tab === "agents" && <Agents />}
        {tab === "voice" && <Voice />}
        {tab === "terminal" && <Terminal />}
        {tab === "feed" && <Feed />}
      </main>
    </div>
  );
}
