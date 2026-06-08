// App.tsx — single page, 3 zone layout (V1 complete).
// On desktop: 3 columns. On mobile (< 768px): vertical stack with tabs.

import { useState } from "react";
import Voice from "../components/Voice";
import Terminal from "../components/Terminal";
import Feed from "../components/Feed";

type Tab = "voice" | "terminal" | "feed";

export default function App() {
  const [tab, setTab] = useState<Tab>("voice");
  const isMobile = typeof window !== "undefined" && window.innerWidth < 768;

  if (isMobile) {
    return (
      <div className="flex flex-col h-full">
        <nav className="flex border-b border-zinc-800 text-xs">
          {(["voice", "terminal", "feed"] as Tab[]).map((t) => (
            <button
              key={t}
              onClick={() => setTab(t)}
              className={
                "flex-1 py-2 " +
                (tab === t ? "bg-zinc-800 text-zinc-100" : "text-zinc-500")
              }
            >
              {t}
            </button>
          ))}
        </nav>
        <main className="flex-1 min-h-0 p-2">
          {tab === "voice" && <Voice />}
          {tab === "terminal" && <Terminal />}
          {tab === "feed" && <Feed />}
        </main>
      </div>
    );
  }

  return (
    <div className="grid h-full w-full grid-cols-[300px_1fr_360px] gap-2 p-2">
      <section className="panel p-3 min-h-0 flex flex-col" aria-label="voice">
        <Voice />
      </section>
      <section className="panel p-2 min-h-0 flex flex-col" aria-label="terminal">
        <Terminal />
      </section>
      <section className="panel p-3 min-h-0 flex flex-col" aria-label="feed">
        <Feed />
      </section>
    </div>
  );
}
