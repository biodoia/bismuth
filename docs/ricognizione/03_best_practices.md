# Ricognizione 03 — best practices enterprise per AI coding team

Source: 3 fonti gold standard consumate integrali.

## Fonti

1. **Anthropic** — "How we built our multi-agent research system"
   (giu 2025, ancora la reference). Orchestrator-worker pattern.
   https://www.anthropic.com/engineering/multi-agent-research-system

2. **Virtido** — "Agentic Workflow Patterns & Best Practices
   Enterprise" (mar 2026). Tre livelli di agentic AI + pattern core.
   https://virtido.com/blog/agentic-workflows-patterns-best-practices-enterprise

3. **OWASP** — "Top 10 for Agentic Applications 2026" (dic 2025).
   Framework di sicurezza di riferimento.
   https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/

## Pattern architetturali — cosa ha funzionato (Anthropic)

[1] ORCHESTRATOR-WORKER
    Lead + N subagent parallelo. Multi-agent 15× token di chat.
    → bismuth: Hermes = lead, 4 worker (omx/omc/omo/omp).

[2] SUBAGENT SEPARATION OF CONCERNS
    Ogni subagent: prompt dedicato, toolset dedicato, trajectory isolata.
    → bismuth: 12 ruoli distinti in internal/roles/catalog.go.

[3] EFFORT SCALING
    Agents non sanno giudicare l'effort. Prompt con regole esplicite:
    simple=1 agent 3-10 calls, medium=2-4 10-15 calls, complex=10+.
    → bismuth: planner prompt codifica queste regole.

[4] START WIDE, NARROW DOWN
    Agents default-ano a query troppo specifiche. Forzare broad→narrow.
    → bismuth: prompt iniziale "esplora prima, identifica 3 punti".

[5] PARALLEL TOOL CALLING
    Speed ×10. Workers devono lanciare N tool in parallelo.
    → bismuth: nessun lock seriale nel multiplexer.

[6] EXTENDED THINKING COME SCRATCHPAD
    Lead planning, subagent interleaved thinking dopo tool result.
    → bismuth: tutti i worker scrivono in thinking block (Claude/Codex).

[7] ARTIFACT PERSISTENCE
    Subagent output to filesystem, passano reference, no copia context.
    → bismuth: Cognee (graph+vector) È il filesystem.

[8] END-STATE EVALUATION
    Valuta RISULTATO FINALE, non passi. Checkpoint su state change.
    → bismuth: dashboard mostra end-state (file/test/PR) non "turns".

[9] LLM-AS-JUDGE
    Judge con rubric (accuracy, completeness, source quality, tool
    efficiency). Score 0-1.
    → bismuth: skill bismuth-control ha "review_peer(worker, output)".

[10] REFLECTION PATTERN (Virtido)
     Agent genera, poi critiche, poi rifinisce. Per output critici.
     → bismuth: ogni worker fa reflection prima di postare "task done".

[11] PLANNING PATTERNS (Virtido)
     Plan-Act per stabili, Plan-Act-Reflect per incerti.
     → bismuth: skill forza 3 fasi (plan → execute → reflect).

[12] CONTEXT BUDGET & SUMMARIZATION
     Agent summarize completed phases, store in external memory.
     → bismuth: quando context si avvicina al limite, scarica su
       Cognee (episodic memory) e chiede fresh context.

[13] CHECKPOINTING & DURABLE EXECUTION
     Restart = expensive. Sistema riprende da dove era.
     → bismuth: multiplexer tiene checkpoint per pane (last N linee,
       cursor pos, agent state).

[14] RAINBOW DEPLOYMENTS
     Update agent → vecchi e nuovi insieme, shift graduale.
     → bismuth: versionamento skill/MCP, v1 e v2 coesistenti su
       worker diversi per test.

## OWASP Agentic Top 10 2026 — applicato a bismuth

| ASI | Risk | Mitigation in bismuth |
|-----|------|------------------------|
| 01 | Agent Goal Hijack | sanitize input esterno, prompt che rifiuta injection |
| 02 | Tool Misuse | command allowlist (internal/security/policy.go) |
| 03 | Identity/Privilege Abuse | per-agent scope, audit log su ogni action |
| 04 | Supply Chain | MCP pinned a versione, skill SHA pinned |
| 05 | Unexpected Code Execution | dry-run + confirm per distruttivi, worktree isolation |
| 06 | Memory Poisoning | Cognee: solo Hermes write, worker solo append |
| 07 | Inter-Agent Comms | signed messages, nonce, TTL corto |
| 08 | Cascading Failures | independent verification, monitora end-state |
| 09 | Human-Agent Trust | diff sempre visibile, no "fidati", no urgent emoji |
| 10 | Rogue Agents | kill switch, audit log, alerts, cost ceiling |

## Virtido — 3 livelli di agentic

- Level 1: AI Workflows (output decisions only)
- Level 2: Router Workflows (agent sceglie tasks/tools in set definito)
- Level 3: Autonomous Agents (crea nuovi tasks/tools per goal)

bismuth V1 = Level 2 (router con team). V2 = Level 3 (Hermes crea
nuovi task al volo).

## Checklist governance enterprise (Virtido adattata)

READINESS
  [x] Data quality = codice leggibile (no file 5000 righe, no TODO)
  [x] Tool integration = 9router + aigoproxy + Cognee
  [ ] Compliance = EU AI Act risk classification (V2)

HUMAN-IN-THE-LOOP CHECKPOINT
  [ ] PRIMA del team start: Sergio approva task decomposition
  [ ] DOPO ogni worker done: Sergio vede diff, ok o reject
  [ ] PRIMA del merge: Sergio approva PR (o Hermes con doppio check)
  [ ] NO silent: niente agent che fa push senza OK

SECURITY
  [x] Worktree per worker (mai main)
  [x] Command allowlist per worker
  [x] MCP firmato e pinned
  [x] Skill bismuth-control firmata
  [x] Log immutabile con provenance
  [x] Cost guardrail (max token per task)

OBSERVABILITY
  [ ] OpenTelemetry trace per ogni agent invocation
  [x] Dashboard bismuth: stato N worker, end-state, token used
  [x] Cognee history ispezionabile
