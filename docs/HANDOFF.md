# bismuth — Handoff

> For the next session (Bremes o qualsiasi altro agent). Leggi questo
> file PRIMA di toccare codice. Poi leggi `ARCHITECTURE.md`, `ROLES.md`,
> `DECISIONS.md`. Poi le `docs/ricognizione/` per contesto esteso.

## Quick Reference

- Build: `GOTMPDIR=/home/lisergico25/.tmp go build -o bin/bismuth ./cmd/bismuth`
- Test: `GOTMPDIR=/home/lisergico25/.tmp go test -count=1 ./internal/...` (17 test su 8 package)
- Web: `cd web/ && npm run build` (tsc strict + vite, 0 errori)
- TUI: `bismuth tui` (bubbletea v1, agent list + event feed, 3s refresh)
- Server: `bismuth serve --config config.yaml` (porta 9000)
- GOTMPDIR=/home/lisergico25/.tmp OBBLIGATORIO (/tmp piena al 100%)
- Tailnet: `bismuth.biodoia.ts.net` via aigoproxy (:80 → localhost:9000, auth tailscale)
- NOTA: la porta 9000 è solo localhost. aigoproxy espone solo :80 senza porta.
- `go test ./...` senza GOTMPDIR può fallire — usare sempre il path esplicito

## P0-P5 status — ALL DONE

- P0: scaffold, build, roles, pane, bus, db, API REST, WebSocket
- P1: web (React+xterm+VAD+audit), prompts (12 ruoli), pane coalesce, MCP (8 tools)
- P2: CI, Dockerfile, systemd, cost guardrail, TUI client bubbletea
- P3: Prometheus metrics, shared memory FTS5, OpenAPI 3.1, Litestream, aigoproxy route
- P4: LLM dispatch, MCP memory_post, Grafana dashboard, provider config
- P5: Structured logging slog, LiveKit stub, alertmanager rules, meta-dev skill update

## P6 status — DONE

| # | Item | Status | Commit |
|---|------|--------|--------|
| P6-a | Fix Dependabot vulns | DONE | `4c8919b` |
| P6-b | E2E test suite | DONE | `e704dbf` |
| P6-c | Auth middleware + RBAC | DONE | `6b660e7` |
| P6-d | Web UI polish | DONE | HEAD |

### P6-a: Dependabot fix
- vitest → 4.1.8 (critical CVE)
- vite → 8.0.16 (medium CVE)
- chi/v5 → 5.3.0 (medium CVE)
- esbuild upgraded via vite
- 0 npm vulnerabilities after fix

### P6-b: E2E test suite
- `internal/api/api_test.go` — 11 test httptest.Server
- `Handler()` method estrae chi.Router per testabilità
- Covers: healthz, agents CRUD, roles, shared memory, voice rooms, metrics, 404, 400
- Fixed: Go empty slice → JSON null (test accetta nil come lista vuota)
- Tutti 17 test passano su 8 package

### P6-c: Auth middleware + RBAC
- `security.User` struct con Email/Name/Role
- `UserFromHeaders` parse Tailscale-User-Login + Tailscale-User-Name
- Context injection via `UserFromContext`/`ContextWithUser`
- RBAC: admin/operator/viewer con CanSpawn/CanKill/CanRead
- authMiddleware estrae user prima del IP check
- V1: tutti gli utenti Tailscale sono admin
- 6 test RBAC in `internal/security/rbac_test.go`

### P6-d: Web UI polish
- `Agents.tsx`: agent list live, spawn/kill, role icons, state badges color-coded
- `Header.tsx`: branding, WS status, link events/metrics
- Layout 4-zone desktop: agents|terminal|voice|feed
- Mobile: tabs con agents come default

## P7 backlog (prossima sessione)

| # | Item | Note |
|---|------|------|
| P7-a | LiveKit SDK reale | `go get github.com/livekit/server-sdk-go` |
| P7-b | Wake-word detection | Porcupine/OpenWakeWord |
| P7-c | Cognee/Mem0 memory | Graph+vector, sostituire FTS5 |
| P7-d | Telegram/Discord bridge | Bot per notifiche + comando remoto |
| P7-e | Multi-tenant | Namespace isolation, per-team DB |
| P7-f | Web code-splitting | Dynamic import, ridurre chunk size |
| P7-g | User-based RBAC enforcement | enforce CanSpawn/CanKill negli handler |
| P7-h | Audit trail UI | Visualizzare audit log nel web |
| P7-i | Task drag-drop assignment | Assegnare task ad agenti via drag |
| P7-j | Streaming agent output | SSE per agent output in real-time |

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
internal/security/      → Policy + RBAC (User, CanSpawn/Kill/Read)
internal/costguard/     → Cost ceiling enforcement per task
internal/audit/         → Audit log with salted hashing
internal/tui/           → Bubbletea TUI client
web/src/                → React 4-zone (Agents|Terminal|Voice|Feed) + Header
prompts/                → 12 role prompts (implementer, reviewer, architect, etc.)
docs/                   → Grafana dashboard, alertmanager rules, OpenAPI spec
```

## Decisioni chiave

- `json.Marshal` + `w.Write` al posto di `json.NewEncoder` (encoder silenzioso su errori)
- Custom `MarshalJSON` su struct con `sql.Null*` (null quando Valid=false)
- `writeJSON` con error logging via slog
- bubbletea v1.2.4 + lipgloss v1.0.0 (v2 breaking)
- PWA disabilitata in V1 (ENOSPC su /tmp)
- VAD con `@ricky0123/vad-web` (WebAssembly offline)
- Provider API keys iniettate per CLI tool via `cli_env` config
- FTS5 con fallback LIKE per shared memory query
- slog per structured logging
- LiveKit come V2 voice path, V1 HTTP voice default
- chi/v5 5.3.0, vitest 4.1.8, vite 8.0.16
- Handler() method per testabilità (chi.Router pubblico)
- security.User + RBAC con Tailscale headers
- V1 RBAC: tutti Tailscale users = admin
- Go empty slice serializza come JSON null (non [])
- `go test ./...` richiede GOTMPDIR esplicito

## Commit count: 36 su main
