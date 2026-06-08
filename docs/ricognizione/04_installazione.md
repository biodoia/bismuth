# Ricognizione 04 — la tua installazione (mappa operativa)

Source: scansione locale di `~/.local/bin`, `~/.cargo/bin`, `npm list -g`,
`uv tool list`, `~/.config/*`, `~/.claude/skills/`, `~/.omc/`, `~/.omx/`,
`~/.omp/`, `~/.config/opencode/`, `~/.hermes/`.

## Coding agent CLI installati (19)

| agent | ver | lingua | tipo |
|-------|-----|--------|------|
| claude | 2.1.168 | node | Claude Code CLI |
| codex | 0.137.0 | rust | OpenAI Codex CLI |
| opencode | 1.16.2 | go/ts | OpenCode CLI + plugin omo |
| gemini | 0.40.1 | node | Google Gemini CLI |
| qwen-code | 0.14.5 | node | Qwen Code CLI (Alibaba) |
| kimi | 1.40.0 | node | Moonshot Kimi CLI |
| droid | 0.115.0 | node | Factory Droid |
| goose | 1.28.0 | go | Block Goose |
| amp | -err- | node | Sourcegraph Amp (broken keyring) |
| vtcode | 0.99.1 | rust | vinhnguyen2308/vtcode |
| vibe | 2.7.6 | py | Mistral Vibe |
| cursor-agent | 26.04 | node | Cursor Agent CLI |
| openhands | 1.14.0 | py | All-Hands OpenHands |
| jcode | - | ? | ? |
| auggie | 0.24.0 | node | Augment Code |
| oh-my-claudecode | 4.13.4 | node | oh-my-claude-sisyphus |
| @charmland/crush | 0.65.2 | node | Charm Crush (omx gemello) |
| happy-coder | 1.1.9 | ? | ? |
| tiny-agents | - | py | HF tiny-agents |
| deepagents | 0.0.39 | py | LangChain deepagents |
| toad | 0.6.16 | ? | batrachian-toad |
| crow-cli | 0.1.19 | ? | Crow agent |
| onecli | 1.1.0 | ? | OneCLI agent |
| command-code | 0.30.1 | node | ? |
| deepseek-tui | 0.8.47 | ? | DeepSeek TUI |
| mmx-cli | 1.0.16 | ? | ? |
| vibe-trading | - | py | ? |

## Provider LLM (dal config omo ~/.config/opencode/oh-my-openagent.json)

- **anthropic**: claude-opus-4-6, opus-4-7, sonnet-4-6, haiku-4-5
- **openai**: gpt-5.5, gpt-5.4 (-mini, -nano), gpt-5.3-codex, gpt-5-nano
- **google**: gemini-3.1-pro-preview, gemini-3-flash-preview
- **opencode**: alias per anthropic/openai/google + big-pickle (custom)
- **opencode-go**: minimax-m2.7, kimi-k2.5, glm-5 (highspeed)
- **kimi-for-coding**: k2p5 (Moonshot K2.5 coding)
- **zai-coding-plan**: glm-5, glm-4.6v (Zhipu Z.ai, key in opencode.json)

Totale: 19+ modelli distinti in rotazione automatica.

## "oh-my" mod attive

- **oh-my-claudecode (omc)** v4.13.4 — IN USO. ~/.claude/CLAUDE.md è sua.
  Comandi: omc team N:claude|codex|gemini "task"
- **oh-my-openagent (omo)** via opencode plugin — IN USO. Plugin in
  ~/.config/opencode/plugins/herdr-agent-state.js. Config con 11
  agent (sisyphus, hephaestus, oracle, librarian, explore,
  multimodal-looker, prometheus, metis, momus, atlas, sisyphus-junior)
  + 7 categorie (visual-engineering, ultrabrain, deep, artistry,
  quick, unspecified-low, unspecified-high, writing).
- **oh-my-codex (omx)** — installato ma non attivo (no eseguibile)
- **oh-my-pi (omp)** — installato ma non attivo

## ACP agent standard (6)

- hermes-acp (0.10.0, in-house)
- hf-inference-acp
- mini-agent-acp
- openhands-acp
- vibe-acp
- claude-code-acp (npm global @zed-industries/claude-code-acp)

## MCP servers disponibili

- zai-mcp-server (Z.ai, stdio, locale)
- web-search-prime (Z.ai, remote)
- web-reader (Z.ai, remote)
- zread (Z.ai, remote)
- openclaw channels (Telegram/Discord/Slack/file/HTTP)
- omo builtin (5: context7, websearch, grep-app, lsp, ast_grep)
- omx 6 (state, memory, code_intel, trace, wiki, hermes)
- omc 4 (t team, omc-tools, standalone, memory)
- nlm (notebooklm-mcp-cli)
- notebooklm-mcp

## Skill attive in ~/.claude/skills/ (43)

9router + 7 subskills, tavily + 6 subskills, omc-3 (omc-teams,
omc-doctor, omc-learned), ralph/ralplan/ultrawork/autopilot, mem0,
nia, zread, appuntaigo, dogodocu, biblaigo, find-skills, skill,
mcp-setup, cancel, iterative-quality-loop, self-improve, learner,
meta-dev, gofrontai, hud, project-session-manager, deep-dive,
deep-interview, team.

## Multiplexer già attivi

- **herdr** (Rust, agent-aware, 18 detector, plugin opencode attivo
  via herdr-agent-state.js)
- **omc team** (CLI-team runtime, spawna N claude/codex/gemini in tmux)
- **openclaw chat/tui** (OpenClaw TUI locale)

## Gateway / proxy

- **aigoproxy** (reverse proxy, 3 routes attive, su tailscale)
- **9router** (AI gateway, 30+ provider, REST + MCP)
- **openclaw gateway** (channels + ACP + cron + approvazioni)

## Note rilevanti

- Il fork privato `biodoia/tmuxai` (creato 19 dic 2025, ultimo push
  22 mag 2026) è STALE: 22 commit upstream non presenti, 0 commit
  tuoi. Non ha valore. Lavora su `~/projects/tmuxai` upstream.
- `biodoia/tuitty` (Phase 0) — non ancora iniziato, niente codice.
- `~/.omc/state/team/` ha sessioni Ralph attive (ralph-bonus-hunting,
  bonus-alpha-state, bonus-beta-state, ralph-competition) — roba
  in corso che NON va toccata.
- `~/.omp/agent/` ha DB SQLite WAL di omp (agent.db, history.db,
  models.db).

## Voci/TTS/STT (tue capability)

- **9router-tts**: 12+ provider (openai, elevenlabs, openrouter,
  edge-tts, google-tts, local-device, deepgram, nvidia, inworld,
  cartesia, playht, coqui, tortoise, hyperbolic)
- **9router-stt**: 7 provider (openai, groq, gemini, deepgram,
  assemblyai, nvidia, huggingface)
- **openclaw infer tts/audio**: wrapper unificato sopra gli stessi
- **Edge TTS** (free, no auth, decent IT)
- **ElevenLabs** (best quality, hai sub)
