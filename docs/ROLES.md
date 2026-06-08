# bismuth — Role Catalog (V1)

12 specialized roles. Each role = 1 struct in `internal/roles/catalog.go`.
The mapping is informed by the union of:
- omx 21 agent TOML (analyst, architect, build-fixer, code-reviewer,
  code-simplifier, critic, debugger, dependency-expert, designer,
  executor, explore, git-master, planner, researcher, security-reviewer,
  team-executor, test-engineer, verifier, vision, writer)
- omo 11 agent (sisyphus=lead, hephaestus=implementer, oracle=advisor,
  librarian=search, explore=scout, multimodal-looker, prometheus=planner,
  metis=critic, momus=reviewer, atlas=worker, sisyphus-junior=apprentice)
- omc 19 agent prompt set

The V1 catalog is the distillation that minimizes overlap and maximizes
the chance that a "swarm" of N workers actually feels like a team.

## Table

| # | Role | CLI pref | Model | Effort | $/task | Used for |
|---|------|----------|-------|--------|--------|----------|
| 1 | planner | omx | anthropic/claude-opus-4-7 | max | $1.0 | high-level plan, decompose, depends_on graph |
| 2 | architect | omx | anthropic/claude-opus-4-7 | max | $1.5 | system design, library pick, ADRs |
| 3 | implementer | omx, omc | openai/gpt-5.5 | medium | $2.0 | write code, smallest viable diff, verify |
| 4 | reviewer | omc | anthropic/claude-opus-4-7 | high | $1.0 | review diffs, style, security, never writes prod code |
| 5 | tester | omx | anthropic/claude-sonnet-4-6 | medium | $1.0 | TDD, run tests, report coverage |
| 6 | security | omc | anthropic/claude-opus-4-7 | max | $1.5 | OWASP ASI01-10 audit, threat model |
| 7 | debug | omc | anthropic/claude-opus-4-7 | max | $1.5 | repro bug, root-cause, propose fix |
| 8 | refactor | omc | anthropic/claude-sonnet-4-6 | medium | $1.0 | behavior-preserving cleanup |
| 9 | documenter | omx | google/gemini-3-flash | medium | $0.5 | README, API docs, ADRs, CHANGELOG |
| 10 | devops | omx | openai/gpt-5.4 | high | $1.0 | CI/CD, infra, worktree hygiene, deploy |
| 11 | critic | omc | anthropic/claude-opus-4-7 | max | $0.8 | devil's advocate, attacks the plan |
| 12 | verifier | omc | openai/gpt-5.4 | xhigh | $0.5 | end-state check: tests/build/lint/security OK |

## Effort levels

We map reasoning_effort (Anthropic) / variant (OpenAI) / temperature
(others) to a 4-step scale:

- **low**: trivially easy, no risk, no multi-file
- **medium**: standard work, 1-3 files, reversible
- **high**: complex, multi-file, risk-aware
- **max / xhigh**: critical, irreversible, security-sensitive

## CLI preference order

For each role we list preferred CLIs in order. The multiplexer picks
the first one available in the user's PATH. Fallback: omc → omx → omo
→ omp → claude → codex → opencode.

## When to spawn which role

| Goal | Spawn pattern (in order) |
|------|--------------------------|
| New app from scratch | planner → architect → implementer → reviewer → tester → verifier |
| Add feature | planner → implementer → reviewer → tester → verifier |
| Fix bug | debug → implementer → tester → verifier |
| Security audit | security (alone) → critic |
| Refactor | refactor (alone) → reviewer → verifier |
| Docs | documenter (alone) |
| Deploy | devops (alone) → verifier |
| Debate a design | critic (after planner/architect) |
| Merge review | reviewer → verifier → devops |

## Scaling rules (Anthropic pattern)

From `Anthropic — How we built our multi-agent research system`:

- Simple fact: 1 worker, 3-10 tool calls
- Medium: 2-4 workers, 10-15 calls each
- Complex: 10+ workers,职责 divise

bismuth encodes this in the planner prompt: a planner that produces
a plan with 1 task spawns 1 worker; with 5+ tasks spawns 5+ workers
in parallel.

## Adding a new role (V1 contribution guide)

1. Add a `Role` struct to `internal/roles/catalog.go` `DefaultCatalog()`.
2. Add a prompt file under `prompts/<role>.md` (system prompt fragment).
3. Update `docs/ROLES.md` table.
4. Add tests in `internal/roles/catalog_test.go`.
5. Open a PR. CI runs `go test ./...`.

That's it. No other file needs to change.
