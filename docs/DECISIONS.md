# bismuth — Decision Log

Tutte le decisioni architetturali in un posto, con motivazione e data.
Append-only.

## 2026-06-08 — Sessione 1 (iniziale)

### D-001: Linguaggio server = Go
- Motivazione: allineato a tuios/tmuxai/aigoproxy/memogo/9router (tutta
  la tua infra). MIT pulito. Niente Rust per evitare AGPL-taint (herdr
  è AGPL, lo usiamo come dipendenza esterna, non lo importiamo).
- Alternativa scartata: TS+Bun (peso enorme, 368M solo per omp).
- Alternativa scartata: Rust (AGPL se importato, pesante).

### D-002: SQLite via modernc.org/sqlite (cgo-free)
- Motivazione: zero dipendenze C, build riproducibile, niente CGO.
- WAL mode + busy_timeout + foreign_keys.
- Pattern: store wrapper + migrations embedded in FS.

### D-003: WebSocket pub/sub via gorilla/websocket
- Motivazione: standard de facto, maturo, MIT.
- Pattern: in-memory fan-out + persist su SQLite per replay.

### D-004: PTY via charmbracelet/x/xpty
- Motivazione: Charm ecosystem, MIT, cross-platform.
- API: `xpty.NewPty(w, h)` + `pty.Start(cmd *exec.Cmd)`.
- Alternativa: libghostty-vt (AGPL, scartata).

### D-005: Voice gateway V1 = HTTP streaming via 9router
- Motivazione: zero deploy extra (9router già attivo). STT Groq/Whisper
  turbo (30× realtime) o Deepgram Nova-3 (streaming). TTS Edge TTS
  (free IT) o ElevenLabs (best quality, sub già attiva).
- V2: LiveKit (real-turn, VAD, barge-in).

### D-006: 12 ruoli specializzati (non 19, non 5)
- Pianificazione: planner, architect, implementer, reviewer, tester,
  security, debug, refactor, documenter, devops, critic, verifier.
- Modello per ruolo scelto dal catalogo provider omo (opus-4-7 per
  i "thinking" roles, gpt-5.5/sonnet-4-6 per gli "execute" roles).
- Ogni ruolo = 1 struct in `internal/roles/catalog.go`.

### D-007: OWASP Agentic Top 10 mitigations = built-in
- Ogni ASI01-10 ha contromisura concreta nel codice (security package).
- Audit log con hash chain (tamper-evident).
- Command allowlist + risk scoring.

### D-008: MCP "bismuth-team" = stdio JSON-RPC 2.0
- Standard MCP ufficiale. Tool set: team_status, team_peers, team_post,
  team_read_inbox, team_claim, team_finish, shared_memory.
- Installato su ciascun worker via .mcp.json.

### D-009: Web UI = Vite + React + Tailwind + xterm.js
- PWA manifest per install su smartphone.
- 3 zone: vocale (push-to-talk), terminal remote (xterm.js), feed
  realtime (eventi).

### D-010: Hermes skill = bismuth-control
- YAML + script bismuth-cli.sh che parla REST al server.
- 8 comandi: list-agents, list-tasks, spawn, send, read, assign,
  kill, merge, status.

### D-011: Flusso utente = DIALOGO PRIMA, poi AUTONOMIA
(Sergio OOB 8 giu 2026, 18:43)
- Idea → dialogo con Hermes (botta e risposta) → research automatica
  → ragionamento + immaginazione concreta → spec presentate → interfacce
  grafiche proposte (mockup, scelte) → "ok" → build autonoma team →
  commit/push frequenti → test come un umano → updates periodici.
- HITL solo su push/force/operazioni distruttive.
- Ogni fase completata = commit + push + notifica.
- Questo è il comportamento della skill bismuth-control V1, non un
  workflow separato.

### D-012: Massimo livello di automazione (Sergio OOB)
- Worktree-per-task automatico
- PR aperto da implementer, reviewer e verifier interagiscono da soli,
  critic attacca la plan, devops fa CI
- Tu guardi e basta nella V2. Nella V1, approvi i checkpoint chiave
  (spec, interfacce, merge finale).

## Backlog (decisioni da prendere nelle prossime sessioni)

- D-013: 9router come AI gateway unico o multi-gateway (9router +
  direct provider)?
- D-014: Memory backend = Cognee (graph) o Mem0 (vector) o entrambi?
- D-015: V2 voice = LiveKit (self-host) o cloud (Daily.co)?
- D-016: Multi-tenancy: solo Sergio o anche altri utenti? (impatta auth)
- D-017: Pricing model per V1: gratis MIT, o commerciale con closed
  features?
