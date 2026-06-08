# bismuth — Architecture

> Last updated: 8 giu 2026, sessione 1 (initial design + scaffold)

## High-level

bismuth is a Go single-binary server that orchestrates a team of AI
coding agents. The user drives the swarm from a browser/smartphone
(via the Web PWA) or via voice. Hermes Agent (the lead) drives the
swarm via the `bismuth-control` skill.

```
   [Browser / Smartphone PWA]            [Hermes Agent (lead)]
            \                                  /
             \  WebSocket + HTTPS              / MCP / HTTP REST
              \                              /
               +--------[ bismuth ]---------+
               |     Go single binary       |
               |     port 9000              |
               +-----+----------+-----------+
                     |          |
                     |          +--> 9router (STT/TTS/LLM gateway)
                     |
       +-------------+----------------+
       |             |                |
   [SQLite]   [WS Bus]         [PTY panes]
   agents,    pub/sub +        worker CLIs
   tasks,     persistent       (omx/omc/omo/omp/
   events,    log              claude/codex/...)
   audit_log
   panes
   messages
```

## Components (V1)

### 1. Core multiplexer (Go)
- SQLite for state
- WebSocket bus for realtime events
- HTTP REST + WS API
- PTY pane manager (charmbracelet/x/xpty)
- 12-role catalog
- Worktree manager (git shell-out)
- Command allowlist + cost guardrail
- Hash-chained audit log

### 2. Voice gateway (Go)
- STT: 9router (groq whisper-large-v3-turbo, fallback deepgram nova-3)
- TTS: 9router (edge-tts it-IT-IsabellaNeural, fallback elevenlabs)
- Command parser: maps natural language to bismuth API calls
- Wake-word optional ("bismuth" in IT)

### 3. MCP server `bismuth-team` (Go, stdio)
- 7 tools: team_status, team_peers, team_post, team_read_inbox,
  team_claim, team_finish, shared_memory
- Installed on each worker via .mcp.json

### 4. Hermes skill `bismuth-control` (YAML + bash)
- 8 commands: list-agents, list-tasks, spawn, send, read, assign,
  kill, merge, status
- Talks REST to bismuth server

### 5. Web PWA (TypeScript, Vite + React + Tailwind)
- Zone vocale: push-to-talk, MediaRecorder → /v1/voice/stt,
  transcript live, TTS playback
- Zone terminal remote: xterm.js attached to any pane
- Zone feed realtime: eventi agent_spawned, agent_state, pane_output,
  agent_message, task_assigned, task_done

## Data model

7 tables in SQLite (see `migrations/001_init.sql`):
- **agents** — worker pane instances, state, model, cost
- **tasks** — bacheca (open / assigned / in_progress / review /
  done / failed / cancelled)
- **events** — append-only bus log
- **audit_log** — tamper-evident hash chain
- **panes** — PTY state + last scrollback tail
- **messages** — inter-agent mailbox
- **settings** — runtime k/v overrides

## User flow (D-011)

```
+------------------------------------------------------------+
| 1. IDEA     user says "voglio un'app che fa X"             |
+------------------------------------------------------------+
| 2. DIALOGUE bismuth (Hermes) talks back, asks questions,   |
|             explores tradeoffs in botta-e-risposta          |
+------------------------------------------------------------+
| 3. RESEARCH bismuth spawns researcher worker (omx+explore)  |
|             reads docs, scans similar apps, uses tavily     |
+------------------------------------------------------------+
| 4. REASON   bismuth planner writes concrete spec           |
|             with user stories, acceptance criteria          |
+------------------------------------------------------------+
| 5. SPECS    bismuth presents specs to user for review       |
|             "vuoi cambiare qualcosa?"                       |
+------------------------------------------------------------+
| 6. MOCKUPS  bismuth shows 3-5 UI mockups (via appuntaigo)  |
|             user picks one (or describes changes)           |
+------------------------------------------------------------+
| 7. OK       user says "ok" / "vai"                          |
+------------------------------------------------------------+
| 8. AUTONOMOUS BUILD                                          |
|    - worktree per task                                       |
|    - planner → architect → implementer → reviewer →         |
|      tester → security → critic → verifier → devops         |
|    - commit + push at every task boundary                   |
|    - HITL only on push to main                              |
|    - updates via WebSocket feed + Telegram/Discord          |
|    - tests run by tester after every implementer commit     |
+------------------------------------------------------------+
| 9. DELIVER  PR opened, human approves, merge to main         |
+------------------------------------------------------------+
```

## Security model (OWASP Agentic Top 10 2026)

| ASI | Risk | Mitigation in bismuth |
|-----|------|------------------------|
| 01 | Agent Goal Hijack | input sanitization, no auto-execute of untrusted strings |
| 02 | Tool Misuse | command allowlist (`internal/security/policy.go`) |
| 03 | Identity/Privilege Abuse | per-agent scope, all actions go through audit log |
| 04 | Supply Chain | pinned versions, MCP stdio, skill SHA-pinned |
| 05 | Unexpected Code Execution | dry-run + confirm prompt for destructive ops, worktree isolation |
| 06 | Memory Poisoning | Cognee write scope (only Hermes writes), provenance tracking |
| 07 | Inter-Agent Comms | signed messages, non-replayable nonce |
| 08 | Cascading Failures | independent verification per worker, kill switch |
| 09 | Human-Agent Trust | always show diff, never auto-merge, "explain in 1 line" for risky ops |
| 10 | Rogue Agents | audit log, kill switch, alerts, cost ceiling |

## Voice flow

```
PWA:    MediaRecorder (16kHz WebM) --chunked POST--> bismuth /v1/voice/stt
bismuth:                                                 |
                                                         v
                                                  9router STT
                                                         |
                                                         v
                                            transcript (it/en)
                                                         |
                                                         v
                                          command parser (Hermes-style)
                                                         |
                                                         v
                                    bismuth API call (spawn/send/status/...)
                                                         |
                                                         v
                                          response text from Hermes
                                                         |
                                                         v
                                                  9router TTS
                                                         |
                                                         v
PWA:    <-chunked audio (mp3/opus)-- bismuth /v1/voice/speak
        Audio plays via Web Audio API
```

## Integration with existing infra

- **herdr** (Rust, AGPL): we use it as a CLIENT to bismuth (it sends
  agent state events to our WS). We do NOT import its code (avoid AGPL).
- **omc team** (Node): `bismuth cli spawn` wraps `omc team` for the
  V1 pilot.
- **oh-my-openagent**: each omo agent gets bismuth-team MCP installed
  in its config; bismuth reads omo's `~/.omo/teams/<run>/` for state.
- **9router**: STT + TTS + LLM gateway. Single source of truth for
  provider config.
- **openclaw**: channels (Telegram/Discord) for notifications.
- **aigoproxy**: reverse proxy on tailscale, exposes bismuth.
- **Cognee / Mem0**: shared memory (V2). V1 uses in-DB task board only.

## Non-goals (V1)

- IDE / code editor (use existing tools)
- Visual kanban (use terminal/Web PWA feed)
- Multi-tenant (single user: lisergico25)
- Cloud deploy (local-only, tailscale for remote access)
- Auto-merge (always human approves final merge)
- Replacing existing tools (omx/omc/omo/omp stay primary)
