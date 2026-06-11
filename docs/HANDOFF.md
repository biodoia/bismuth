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

## Sessione spawn-fix (PR #3) — DONE

Lo spawn degli agenti creava il record ma il worker non lavorava mai
(scrollback sempre vuoto). Quattro root cause corrette in `internal/pane`
+ `internal/api`:

1. pane-id disallineato: l'API salvava un pane_id sull'agente ma Spawn ne
   generava un altro → /read /send /kill non trovavano mai il pane
2. env azzerato: il worker partiva senza PATH/HOME (ora os.Environ + overlay)
3. worktree ignorata: il worker girava in cfg.Workdir, non nella worktree
4. task mai consegnato: ora scritto nella PTY come primo input (+Enter)

Più hardening emerso da CI/review: data race in persistChunk (snapshot
sotto lock), PTY chiusa se exec fallisce, context.Background() per lo
stato terminale, spawn fallito → 500 + agente `failed`.

Web CI (rosso da bf4c691) riparato: ERESOLVE su vite 8 (plugin-react ^6,
vite-plugin-pwa ^1.3) + Tailwind 4 mai cablato (aggiunto @tailwindcss/vite
in vite.config.ts — prima la dashboard usciva SENZA stili).

V1 completata (i TODO "sessione+1" di pane.go):
- MCP installer: `.mcp.json` scritto nella workdir prima dello spawn
  (bismuth-team → questo binario `mcp`, stesso DB via BISMUTH_MCP_DB)
- Read ultime-N-righe (`pane.LastLines`, split su \n, ANSI intatte) +
  `?n=` rispettato in /read
- Stati agente: euristica V1 output→working, silenzio→idle (2s),
  exit→exited (reap via c.Wait — il master PTY non vede mai EOF);
  persistiti su panes+agents, evento `agent_state` sul bus
- P7-g RBAC enforcement: viewer → 403 su spawn/send/kill (user nil =
  CLI localhost → permesso, V1)
- Test ermetici: repoRoot dei test API in t.TempDir() — prima ogni run
  registrava worktree + branch `bismuth/tsk-*` NEL REPO REALE

## P7 — ALL DONE (sessione ultrawork, PR #4)

| # | Item | Stato | Note |
|---|------|-------|------|
| P7-a | LiveKit reale | DONE | `internal/livekit`: JWT HS256 stdlib (claim identici a protocol/auth) + RoomService via client Twirp-JSON generato di `protocol/livekit` (stesso wire di lksdk). Config `livekit.{url,api_key,api_secret}` con ${VAR}; senza config → stub V1 |
| P7-b | Wake-word | DONE | Gate server-side su `/v1/voice/command` (`continuous:true` → richiede prefisso `voice.wake_word`, default "bismuth", risposta `ignored:true` senza wake) + toggle Continuous nel client su flusso VAD→STT. Porcupine scartato: licenza+lib native |
| P7-c | Mem0 memory | DONE | `sharedmem.Provider` interface; `Mem0` client REST (auth Token, decode version-tolerant) + `NewFallback(mem0, fts5)`; provenance `Memory.Source` (fts5/mem0). Config `memory.{mem0_base_url,mem0_api_key}`; senza config → FTS5 puro |
| P7-d | Telegram/Discord bridge | DONE | `internal/bridge`: notifier (poll events: spawned/killed/assigned/approval/exited) → sendMessage + webhook Discord; comandi Telegram long-poll `/status /agents /tasks /kill` via API REST, gate sul chat_id. Config `bridge.*` |
| P7-e | Multi-tenant | DONE (namespace V1) | Migration 002: colonna `tenant` su agents/tasks + indici; header `X-Bismuth-Tenant` (default "default") taggato su spawn/create e filtrato su list. Runner migration ora traccia `schema_migrations`. Per-team DB resta V2 |
| P7-f | Web code-splitting | DONE | React.lazy su Terminal (xterm) e Voice (vad/onnx) + manualChunks (react-vendor/xterm/vad) |
| P7-g | RBAC enforcement | DONE (PR #3) | viewer → 403 su spawn/send/kill |
| P7-h | Audit trail UI | DONE | `GET /api/v1/audit` (newest-first, limit/offset) + vista Audit nel web; `audit.Recent()` |
| P7-i | Task drag-drop | DONE | DnD HTML5 nativo task→agent card → `POST /tasks/{id}/assign` |
| P7-j | SSE streaming | DONE | `GET /api/v1/agents/{id}/stream` (event: output `chunk_b64` / state) via fanout per-pane in `pane.Manager.Attach`; ping 15s; `statusRecorder.Flush` per attraversare i middleware. Client: EventSource→xterm con fallback polling |

Note integrazione: i servizi esterni (LiveKit SFU, bot Telegram, Mem0)
sono config-gated — senza chiavi il sistema degrada ai path V1 (stub
room, bridge inerte, FTS5). `server-sdk-go` NON è in go.mod: il client
Twirp generato in `protocol/livekit` è lo stesso codice che lksdk
avvolge.

## Backlog futuro (V2)

- LiveKit: agente publisher audio nel room (oggi: token + room admin)
- Wake-word on-device (Porcupine/OpenWakeWord, richiede licenza/native)
- Cognee graph memory (Mem0 già pluggable via Provider)
- Multi-tenant: per-team DB + auth per-tenant (oggi: namespace header)
- Web: virtualized feed, audit pagination UI

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
