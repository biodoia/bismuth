# Ricognizione 02 — architettura multiplexer emersa

Source: studio di tuios, herdr, tmuxai, oh-my-pi/codex/claudecode/
openagent, Aperant, OpenClaw, 9router.

## Cosa è un multiplexer e cosa NON è

| Tipo | Esempio | Scopo | Standalone? |
|------|---------|-------|-------------|
| Emulatore di terminale | alacritty, kitty, xterm | renderizza TTY in una finestra OS | Sì, è il "padre" |
| Multiplexer TUI | tmux, zellij, tuios, herdr | N pane dentro 1 terminale | Sì, gira dentro emulatore |
| Agent CLI runner | omx, omc, omo, omp | 1 agent in 1 terminale | Sì, è un processo |
| Multiplexer agent-aware | herdr | multiplexer + detector stato agent | Sì, specializzato |
| Multi-agent framework | Aperant, CrewAI, AutoGen | orchestra N agent con UI/IDE | Sì, framework |
| MCP server | openclaw channels, omx 6 first-party | tools standardizzati per agent | No, è un provider |
| AI gateway | 9router, LiteLLM, Portkey | astrarre provider LLM | Sì, servizio |

## bismuth è un "multiplexer agent-aware" + "MCP server" + "AI orchestrator"
NB: NON un emulatore di terminale.

Va installato in aggiunta al terminale esistente (kitty/alacritty/
Sway+wayland). Gira come servizio. Espone WS+HTTP. Tu ci accedi via
browser o smartphone.

## Pattern a strati emersi

```
LAYER 0  Terminal emulation    → TUIOS internal/vt/ (Go, MIT)  [riferimento]
                                 o herdr libghostty-vt (AGPL)  [da evitare]
LAYER 1  PTY / pane lifecycle  → TUIOS internal/terminal/ (Go)
LAYER 2  Layout (tiling)       → TUIOS internal/layout/bsp.go (Go)
LAYER 3  Agent detection       → HERDR src/detect/agents/ (già hermes.rs)
LAYER 4  Multi-client IPC      → HERDR src/protocol/wire.rs (Rust, OK)
LAYER 5  Skill loading         → TMUXAI internal/skill_registry.go (Go, MIT)
LAYER 6  MCP integration       → TMUXAI internal/mcp/ (Go, MIT)
LAYER 7  Multi-LLM             → TMUXAI internal/ai_client.go (Go, MIT)
LAYER 8  Worker orchestration  → OMX/OMC/OMO/OMP 4 mod (TS/Rust)
LAYER 9  Shared memory         → Cognee + Mem0
LAYER 10 Live conversation     → LiveKit + PartyServer (V2)
```

## Stack scelto (definitivo)

bismuth riusa il più possibile invece di reinventare:

- TUIOS per ispirazione PTY/VT (NON importato, solo reference)
- HERDR come detector via socket (NON importato, AGPL-safe)
- TMUXAI per skill_registry e mcp (NON importato, MIT, pattern only)
- OMC team CLI per spawn worker (Node, riusato via shell-out)
- OMO/OMP/OMX come orchestrate layer (plugin, non import)
- 9router per AI gateway (HTTP REST, già attivo)
- OPENCLAW per notifications (canale, già attivo)
- AIGOPROXY per reverse proxy (già attivo, tailscale)
- COGNEE/MEM0 per memoria (V2, opzionale V1)

## Go dependency scelte (già in go.mod, 8 giu 2026)

```
github.com/biodoia/bismuth    (self)
github.com/charmbracelet/x/xpty v0.1.0     (PTY)
github.com/go-chi/chi/v5 v5.1.0             (HTTP router)
github.com/gorilla/websocket v1.5.3        (WS)
github.com/spf13/cobra v1.8.1               (CLI)
modernc.org/sqlite v1.34.4                  (cgo-free DB)
gopkg.in/yaml.v3                            (config)
```

Tutto MIT, tutto stabile, tutti attivamente mantenuti.

## Schema SQLite (V1)

7 tabelle, migration 001_init.sql:
- agents: istanze worker pane (state, model, cost)
- tasks: bacheca (open/assigned/in_progress/review/done/...)
- events: bus log (sequenziale, indicizzato per type/agent)
- audit_log: hash chain tamper-evident
- panes: scrollback tail + last state
- messages: mailbox inter-agent
- settings: k/v runtime

## API surface (V1)

GET  /healthz
GET  /api/v1/agents
POST /api/v1/agents                    { role, cli, task, args }
GET  /api/v1/agents/:id
POST /api/v1/agents/:id/send           { data_b64 }
GET  /api/v1/agents/:id/read           ?n=200
POST /api/v1/agents/:id/kill
GET  /api/v1/tasks
POST /api/v1/tasks                     { title, description, priority }
GET  /api/v1/tasks/:id
POST /api/v1/tasks/:id/assign
POST /api/v1/tasks/:id/merge
GET  /api/v1/roles
GET  /api/v1/events                    ?types=&agent_id=&limit=
GET  /api/v1/ws                        WebSocket
POST /v1/voice/stt                     multipart audio
POST /v1/voice/speak                   { text } -> { audio_b64, format }
POST /v1/voice/command                 { text } -> command parsed

## Comandi CLI (V1)

bismuth serve                # run server
bismuth tui                  # local TUI client
bismuth mcp                  # MCP server (stdio, installato sui worker)
bismuth cli list-agents
bismuth cli list-tasks
bismuth cli spawn --role X --cli Y --task Z
bismuth cli send --agent A --data "..."
bismuth cli read --agent A --n 200
bismuth cli assign --task T --agent A
bismuth cli kill --agent A
bismuth cli merge --task T
bismuth cli status
bismuth cli skill-install    # installa SKILL.md in ~/.claude/skills

## Hermes skill (V1)

File: ~/.claude/skills/bismuth-control/SKILL.md
Comandi: 8, tutti via `bismuth cli ...`
Aggiornabile con `bismuth cli skill-install`.
