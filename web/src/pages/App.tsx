// App.tsx — single page, 3 zone layout.
// TODO(sessione+1): implementare completamente.
//   - Colonna sinistra: Voice (push-to-talk + transcript + TTS playback)
//   - Colonna centrale: Terminal remote (xterm.js + WS attachment)
//   - Colonna destra: Feed real-time (event list)

export default function App() {
  return (
    <div className="grid h-full w-full grid-cols-1 md:grid-cols-[300px_1fr_360px] gap-2 p-2">
      <section className="panel p-3 overflow-hidden flex flex-col" aria-label="voice">
        <h2 className="text-sm font-semibold mb-2">Voce</h2>
        <p className="text-xs text-zinc-500">Push-to-talk (TODO)</p>
      </section>
      <section className="panel p-2 overflow-hidden flex flex-col" aria-label="terminal">
        <h2 className="text-sm font-semibold mb-2">Terminale</h2>
        <p className="text-xs text-zinc-500">xterm.js (TODO)</p>
      </section>
      <section className="panel p-3 overflow-hidden flex flex-col" aria-label="feed">
        <h2 className="text-sm font-semibold mb-2">Feed</h2>
        <p className="text-xs text-zinc-500">Event bus (TODO)</p>
      </section>
    </div>
  );
}
