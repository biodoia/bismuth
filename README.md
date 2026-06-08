# bismuth

> Autonomous multi-agent coding team multiplexer with live voice control and web UI.
> Multi-agent coding team specialized by role. Voice-first control. Web-accessible from anywhere.

## What is bismuth

bismuth is a Go server that orchestrates a team of AI coding agents
(omx, omc, omo, omp, claude, codex, opencode, etc.) as a coordinated
swarm. It exposes:

- a **WebSocket + HTTP API** for control and live event streaming
- an **MCP server** (`bismuth-team`) installed on each worker agent to
  give them team-awareness (status, peers, mailbox, shared memory)
- a **Hermes Agent skill** (`bismuth-control`) so the lead agent can
  spawn, monitor, dispatch, merge
- a **Web PWA** with **live voice control** (push-to-talk, VAD, STT via
  9router, TTS via 9router) — accessible from desktop and smartphone
- a **terminal remote** (xterm.js in browser) attached to any worker
  pane
- a **realtime feed** of agent state, messages between agents, and
  pane output

It reuses what you already have:
`herdr` (agent detector), `omc team` (CLI-team runtime), `oh-my-openagent`
(plugin), `9router` (AI gateway), `openclaw` (notify/channels), `Cognee`/
`Mem0` (shared memory), `aigoproxy` (reverse proxy on tailscale).

## Quick start

```bash
# 1. Build
cd bismuth
go mod tidy
go build -o bin/bismuth ./cmd/bismuth

# 2. Configure
cp config.example.yaml config.yaml
# edit config.yaml: set NINEROUTER_URL, NINEROUTER_KEY, MCP allowlist

# 3. Run
./bin/bismuth serve --config config.yaml

# 4. Open
# Local TUI:    ./bin/bismuth tui
# Web PWA:      open http://localhost:9000 in browser
# Smartphone:   open https://bismuth.<your-tailnet>.ts.net (via aigoproxy)
```

## Architecture (5-minute tour)

```
   [Browser / Smartphone PWA]   [Hermes Agent (lead)]
            \                          /
             \  WebSocket + HTTPS     / MCP / HTTP REST
              \                      /
               +----[ bismuth ]-----+
               |   Go single binary |
               |   port 9000        |
               +---------+----------+
                         |
       +--------+--------+--------+--------+
       |        |        |        |        |
   [SQLite]  [WS Bus] [Voice GW] [MCP svr] [PTY pane]
                         |
                         +--> 9router (STT/TTS/LLM)
                         +--> worker CLI: omx/omc/omo/omp/claude/codex
```

See `docs/ARCHITECTURE.md` for the full picture.

## Repository layout

```
bismuth/
├── cmd/bismuth/            # Main entry point
├── internal/
│   ├── db/                 # SQLite schema + queries (modernc.org/sqlite, cgo-free)
│   ├── bus/                # WebSocket pub/sub event bus
│   ├── pane/               # PTY spawn + pane state + I/O proxy
│   ├── voice/              # STT/TTS gateway (HTTP streaming via 9router)
│   ├── api/                # HTTP REST API (chi router)
│   ├── mcp/                # bismuth-team MCP server (stdio)
│   ├── hermes/             # skill manifest generator
│   ├── roles/              # 8-12 specialized role definitions
│   ├── worktree/           # git worktree-per-task manager
│   ├── security/           # command allowlist, cost guardrail
│   ├── audit/              # append-only audit log with hash chain
│   └── config/             # YAML config loader
├── web/                    # PWA: voice + terminal remote + feed
├── skill/                  # Hermes skill: SKILL.md + bismuth-cli.sh
├── migrations/             # SQLite schema migrations
├── docs/
│   ├── ARCHITECTURE.md
│   ├── ROLES.md
│   ├── RICOGNIZIONE_*.md  # source material from sessione 1
│   └── HANDOFF.md         # how to continue the work
├── scripts/                # dev/install/run scripts
└── test/                   # integration tests
```

## Status (8 giu 2026)

**V1 in design + initial code.** This commit contains:

- full directory scaffold
- design docs (ARCHITECTURE, ROLES, HANDOFF, RICOGNIZIONE)
- SQLite schema migration
- Go module skeleton with package stubs
- Web PWA skeleton (Vite + React + Tailwind)
- Hermes skill manifest
- decision log (DECISIONS.md)

**NOT YET IMPLEMENTED (V1 backlog):**

- [ ] F1: Go server entry + HTTP routing
- [ ] F2: SQLite queries + bus publish
- [ ] F3: spawn 1 worker pane (omx pilot)
- [ ] F4: WebSocket sub/unsub
- [ ] F5: MCP server bismuth-team with 7 tools
- [ ] F6: Hermes skill install script
- [ ] F7: Web PWA vocale push-to-talk
- [ ] F8: STT/TTS via 9router
- [ ] F9: auth tailscale-only
- [ ] F10: 1 ruolo specializzato funzionante (implementer)

See `docs/HANDOFF.md` for the next-session checklist.

## License

MIT. Use, modify, redistribute.

## Acknowledgements

Built by `biodoia` + Hermes Agent (Bremes). Reuses code/concepts from
`herdr`, `tuios`, `tmuxai`, `oh-my-codex`, `oh-my-claudecode`,
`oh-my-openagent`, `oh-my-pi`, `openclaw`, `9router`, `aigoproxy`.

Anthropic multi-agent research patterns:
https://www.anthropic.com/engineering/multi-agent-research-system

OWASP Agentic Top 10 2026:
https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/
