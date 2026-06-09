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

## P0-P3 status — ALL DONE

- P0: scaffold, build, roles, pane, bus, db, API REST, WebSocket
- P1: web (React+xterm+VAD+audit), prompts (12 ruoli), pane coalesce, MCP (8 tools)
- P2: CI, Dockerfile, systemd, cost guardrail, TUI client bubbletea
- P3: Prometheus metrics, shared memory FTS5, OpenAPI 3.1, Litestream, aigoproxy route

## P4 status — DONE

| # | Item | Status |
|---|------|--------|
| P4-a | LLM dispatch reale | DONE — providers config + CLIEnv + ${VAR} resolution |
| P4-b | MCP memory_post | DONE — FTS5 query + write shared memory from agents |
| P4-c | Grafana dashboard | DONE — docs/grafana-dashboard.json, 8 panels |
| P4-d | aigoproxy verify | DONE — bismuth.biodoia.ts.net → localhost:9000 |

## P5 backlog (prossima sessione)

| # | Item | Note |
|---|------|------|
| P5-a | LiveKit voice gateway | Sostituire edge/groq con LiveKit SFU |
| P5-b | Wake-word detection | Porcupine/PV porcupine o OpenWakeWord |
| P5-c | Cognee/Mem0 memory | Sostituire FTS5 con graph+vector memory |
| P5-d | Telegram/Discord bridge | Bot per notifiche + comando remoto |
| P5-e | Multi-tenant | Namespace isolation, per-team DB |
| P5-f | Git worktree isolation | Ogni agente in worktree separato |
| P5-g | Prometheus alertmanager rules | Soglie su cost/latency/error rate |
| P5-h | Structured logging | Slog/zap al posto di log.Printf |

## Architettura chiave

```
cmd/bismuth/main.go     → cobra CLI (serve, tui, mcp)
internal/api/           → HTTP REST + WebSocket + Prometheus /metrics
internal/bus/           → Event bus (SQLite-backed, safePayload)
internal/db/            → SQLite store (agents, tasks, events, messages, memories)
internal/pane/          → PTY manager (coalesced scrollback 256B/500ms)
internal/voice/         → STT/TTS gateway (ninerouter)
internal/mcp/           → MCP server (8 tools: team_* + shared_memory + memory_post)
internal/sharedmem/     → FTS5 shared memory (POST/QUERY/LIST/DELETE)
internal/costguard/     → Cost ceiling enforcement per task
internal/metrics/       → Prometheus counters/histograms
internal/tui/           → Bubbletea TUI client
internal/config/        → YAML config + ${VAR} resolution + EnvForCLI
web/                    → React + xterm.js + VAD push-to-talk + audit timeline
prompts/                → 12 role prompts (implementer, reviewer, architect, etc.)
```

## Decisioni chiave

- `json.Marshal` + `w.Write` al posto di `json.NewEncoder` (encoder falliva silenziosamente)
- Custom `MarshalJSON` su struct con `sql.Null*` invece di cambiare 22+ riferimenti
- `safePayload()` nel bus garantisce `"{}"` per payload vuoti
- `writeJSON` con error logging su stderr
- bubbletea v1 (v2 ha breaking changes con lipgloss)
- PWA disabilitata in V1 (ENOSPC su /tmp)
- VAD con `@ricky0123/vad-web` (WebAssembly offline)
- Provider API keys iniettate per CLI tool via `cli_env` config
- FTS5 con fallback LIKE per shared memory query
- aigoproxy route: `bismuth.biodoia.ts.net` (porta 80, no :9000) → localhost:9000

## Blob count: 24 commits su main

Ultimo: `8d51f25 feat(P4): LLM dispatch, MCP memory_post, Grafana dashboard, provider config`
