# bismuth — Handoff

> For the next session (Bremes o qualsiasi altro agent). Leggi questo
> file PRIMA di toccare codice. Poi leggi `ARCHITECTURE.md`, `ROLES.md`,
> `DECISIONS.md`. Poi le `docs/ricognizione/` per contesto esteso.

## Quick Reference

- Build: `GOTMPDIR=/home/lisergico25/.tmp go build -o bin/bismuth ./cmd/bismuth`
- Test: `GOTMPDIR=/home/lisergico25/.tmp go test ./...` (15 test su 6 package)
- Web: `cd web/ && npm run build` (tsc strict + vite, 0 errori)
- TUI: `bismuth tui` (bubbletea v1, agent list + event feed, 3s refresh)
- Server: `bismuth serve --config config.yaml` (porta 9000)
- GOTMPDIR=/home/lisergico25/.tmp OBBLIGATORIO (la partizione /tmp è piena al 100%)
- Tailnet: `bismuth.biodoia.ts.net` via aigoproxy (:80 → localhost:9000, auth tailscale)
- NOTA: la porta 9000 è solo localhost. aigoproxy espone solo :80 senza porta.

## P0-P3 status — ALL DONE

- P0: scaffold, build, roles, pane, bus, db, API REST, WebSocket
- P1: web (React+xterm+VAD+audit), prompts (12 ruoli), pane coalesce, MCP (8 tools)
- P2: CI, Dockerfile, systemd, cost guardrail, TUI client bubbletea
- P3: Prometheus metrics, shared memory FTS5, OpenAPI 3.1, Litestream, aigoproxy route

## P4 status — DONE

- P4-a: LLM dispatch (providers + cli_env + ${VAR} resolution)
- P4-b: MCP memory_post (FTS5 query + write, 8 tools totali)
- P4-c: Grafana dashboard (8 panels)
- P4-d: aigoproxy verify (bismuth.biodoia.ts.net → localhost:9000)

## P5 status — DONE (meta-dev)

| # | Item | Status | Commit |
|---|------|--------|--------|
| P5-1 | Structured logging slog | DONE | `49f4a7d` |
| P5-2 | Git worktree isolation | DONE | già integrato in P0 |
| P5-3 | LiveKit voice stub | DONE | `0c42af8` |
| P5-4 | Alertmanager rules | DONE | `3dab861` |
| P5-5 | meta-dev skill update | DONE | lessons bismuth |

## P6 backlog (prossima sessione)

| # | Item | Note |
|---|------|------|
| P6-a | LiveKit SDK reale | `go get github.com/livekit/server-sdk-go`, SFU connect |
| P6-b | Wake-word detection | Porcupine/OpenWakeWord |
| P6-c | Cognee/Mem0 memory | Graph+vector, sostituire FTS5 |
| P6-d | Telegram/Discord bridge | Bot per notifiche + comando remoto |
| P6-e | Multi-tenant | Namespace isolation, per-team DB |
| P6-f | Web UI polish | Drag-drop task assignment, agent status badges |
| P6-g | Auth middleware reale | Tailscale-User header parsing + RBAC |
| P6-h | E2E test suite | httptest.Server based, no real network |

## Architettura chiave

```
cmd/bismuth/main.go     → cobra CLI (serve, tui, mcp)
internal/api/           → HTTP REST + WebSocket + Prometheus /metrics
internal/bus/           → Event bus (SQLite-backed, safePayload)
internal/config/        → YAML config + ${VAR} resolution + EnvForCLI
internal/db/            → SQLite store (agents, tasks, events, messages, memories)
internal/logger/        → slog wrapper (Debug/Info/Warn/Error + With)
internal/livekit/       → LiveKit room manager stub (V2 upgrade path)
internal/mcp/           → MCP server (8 tools: team_* + shared_memory + memory_post)
internal/metrics/       → Prometheus counters/histograms (agents, API, LLM cost)
internal/pane/          → PTY manager (coalesced scrollback 256B/500ms)
internal/sharedmem/     → FTS5 shared memory (POST/QUERY/LIST/DELETE)
internal/voice/         → STT/TTS gateway (ninerouter)
internal/worktree/      → Git worktree isolation (branch + .bismuth/<task-id>)
internal/costguard/     → Cost ceiling enforcement per task
internal/tui/           → Bubbletea TUI client
web/                    → React + xterm.js + VAD push-to-talk + audit timeline
prompts/                → 12 role prompts (implementer, reviewer, architect, etc.)
```

## Decisioni chiave

- `json.Marshal` + `w.Write` al posto di `json.NewEncoder` (encoder silenzioso su errori)
- Custom `MarshalJSON` su struct con `sql.Null*` (null quando Valid=false)
- `safePayload()` nel bus garantisce `"{}"` per payload vuoti
- `writeJSON` con error logging via slog
- bubbletea v1 (v2 ha breaking changes con lipgloss)
- PWA disabilitata in V1 (ENOSPC su /tmp)
- VAD con `@ricky0123/vad-web` (WebAssembly offline)
- Provider API keys iniettate per CLI tool via `cli_env` config
- FTS5 con fallback LIKE per shared memory query
- aigoproxy route: `bismuth.biodoia.ts.net` SOLO porta 80, localhost:9000 è interno
- slog per structured logging (sostituito fmt.Fprintf + log.Printf)
- LiveKit come V2 voice path, V1 HTTP voice rimane default
- Alerting: 9 rules in 3 groups (agents, api, cost)

## Blob count: 30 commits su main

Ultimo: `3dab861 ops: Prometheus alertmanager rules for bismuth`
