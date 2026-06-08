# Web PWA — bismuth

Stack: Vite + React 19 + TailwindCSS v4 + xterm.js + vite-plugin-pwa.

## Struttura

```
web/
├── public/                 # PWA manifest, icon, fallback
├── src/
│   ├── components/         # Voice, Terminal, Feed, Sidebar
│   ├── hooks/              # useWebSocket, useRecorder, useFetch
│   ├── lib/                # api client, event types
│   ├── pages/              # App.tsx (single page, 3 zone)
│   ├── main.tsx            # React entry
│   └── index.css           # Tailwind imports
├── index.html
├── package.json
├── tsconfig.json
├── tailwind.config.js
└── vite.config.ts
```

## Zone (single page, 3 column)

1. **VOCE** (sinistra) — push-to-talk, trascrizione live, audio TTS playback
2. **TERMINALE** (centro) — xterm.js connesso via WebSocket a una pane scelta
3. **FEED** (destra) — timeline eventi real-time (agent_spawned, pane_output, agent_message, task_done, ...)

## TODO sessione +1

- [ ] Vite project init (`npm create vite@latest` con react-ts template)
- [ ] tailwind setup
- [ ] vite-plugin-pwa config
- [ ] xterm.js + ws attachment
- [ ] MediaRecorder hook
- [ ] STT/TTS roundtrip
- [ ] Event bus sub via WS
- [ ] Layout responsive (3-col desktop, tab mobile)

Questo commit contiene SOLO lo skeleton, il package.json e i TODO.
Vedi docs/HANDOFF.md per la priorità.
