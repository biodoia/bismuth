# bismuth — Handoff

> For the next session (Bremes o qualsiasi altro agent). Leggi questo
> file PRIMA di toccare codice. Poi leggi `ARCHITECTURE.md`, `ROLES.md`,
> `DECISIONS.md`. Poi le `docs/ricognizione/*.md` per il contesto
> completo della sessione 1.

## Stato attuale (8 giu 2026, fine sessione 1)

- Repo creato: https://github.com/biodoia/bismuth
- Branch: `main`
- Commit: 1 iniziale con tutto il design + scaffold
- Build: `go build ./...` OK, binary a `bin/bismuth` (18 MB)
- Working tree: pulito

## Cosa funziona

- `bismuth --help` → stampa help
- `bismuth serve` → legge config, apre SQLite, init bus + pane + voice
- Tutti i package `internal/*` compilano
- Migration `001_init.sql` viene applicata all'avvio
- Tutto è MIT, niente copyleft, niente hardcoded secrets

## Cosa NON funziona (V1 backlog, in ordine di priorità)

### P0 — must per "V1 funzionante"

- [ ] **F1**: registrare le route HTTP in `internal/api/api.go`
      (al momento solo `/healthz`). Vedi il doc-comment del package per
      l'elenco. ~300 LOC, uso `chi` router.

- [ ] **F2**: implementare le query SQLite in `internal/db/queries.go`
      (nuovo file) per `agents`, `tasks`, `events`, `messages`, `panes`.
      Il package ha solo lo schema, non le query CRUD. ~400 LOC.

- [ ] **F3**: cablare `pane.Spawn` con il flusso completo:
      1. `worktree.Manager.Create(ctx, taskID, "main")`
      2. scrivere `.mcp.json` con `bismuth-team` MCP server
      3. spawnare `omx` (o altro CLI) dentro il worktree
      4. pubblicare eventi pane_spawned
      ~200 LOC.

- [ ] **F4**: WebSocket subscribe con read pump + write pump
      gorilla/websocket standard. Vedi esempio in gorilla docs.
      ~150 LOC.

- [ ] **F5**: implementare i 7 tool MCP in `internal/mcp/mcp.go`
      (al momento solo stub). `team_status`, `team_peers`, `team_post`,
      `team_read_inbox`, `team_claim`, `team_finish`, `shared_memory`.
      ~400 LOC.

- [ ] **F6**: script `bismuth cli skill-install` che chiama
      `hermes.Install("~/.claude/skills")`. Aggiungere subcommand
      `cli` al main cobra. ~50 LOC.

- [ ] **F7-F8**: Web PWA vocale. Vedi `web/src/pages/Voice.tsx` (vuoto).
      Implementare: MediaRecorder → POST /v1/voice/stt → play TTS.
      ~500 LOC TypeScript/React.

- [ ] **F9-F10**: auth tailscale-only + 1 ruolo specializzato
      (implementer) end-to-end funzionante.

### P1 — dopo che V1 gira

- [ ] V2-1: 8-12 ruoli specializzati
- [ ] V2-2: worktree-per-task automatico (già iniziato)
- [ ] V2-3: cost guardrail enforcement (contatore per task)
- [ ] V2-4: command allowlist enforcement
- [ ] V2-5: PR obbligatorio per merge
- [ ] V2-6: multi-worker parallelo (4-8)
- [ ] V2-7: LiveKit server per voce real-time
- [ ] V2-8: wake-word "bismuth"
- [ ] V2-9: notifiche Telegram/Discord via openclaw
- [ ] V2-10: TUI client (charm.land/bubbletea/v2)

### P2 — polishing

- [ ] CI: GitHub Actions con `go test`, `go vet`, build matrix
- [ ] Dockerfile multi-stage
- [ ] systemd unit file (vedi `scripts/bismuth.service`)
- [ ] OpenAPI spec autogenerata da chi routes
- [ ] Prometheus metrics endpoint
- [ ] Backup automatico della SQLite (Litestream o rclone)
- [ ] Grafana dashboard per team status

## Convenzioni di coding

- Go 1.23, niente generics se non necessario, niente panic in prod
- Error sempre wrappato con `fmt.Errorf("ctx: %w", err)`
- Log strutturato con `slog` (no `fmt.Printf` fuori da main/CLI)
- Ogni package ha il suo `*_test.go`
- Commit: conventional commits (`feat:`, `fix:`, `docs:`, `refactor:`)
- Branch: `feature/<short-name>`, PR a main
- Niente secrets in repo. `config.example.yaml` per template.

## Comandi utili per la prossima sessione

```bash
# Clone
git clone git@github.com:biodoia/bismuth.git
cd bismuth

# Build
GOTMPDIR=/home/lisergico25/.tmp go build -o bin/bismuth ./cmd/bismuth

# Config
cp config.example.yaml config.yaml
# edit: set NINEROUTER_URL, NINEROUTER_KEY, audit.salt (random)

# Test
GOTMPDIR=/home/lisergico25/.tmp go test ./...

# Vet
go vet ./...

# Run
./bin/bismuth serve --config config.yaml

# Web
open http://localhost:9000

# TUI
./bin/bismuth tui

# Install skill
./bin/bismuth cli skill-install
```

## Gotchas scoperti

1. **/tmp pieno**: la macchina ha /tmp come tmpfs da 32G spesso al 100%.
   Usa `GOTMPDIR=/home/lisergico25/.tmp` per i build Go.

2. **xpty API**: `xpty.NewPty(w, h)` non `xpty.New()`. Ritorna
   `(Pty, error)`. `pty.Start(cmd *exec.Cmd)` non `xpty.Command(...)`.

3. **modernc/sqlite**: no CGO, build più lento la prima volta (~30s)
   ma poi cache. SetMaxOpenConns(1) per WAL.

4. **owasp-agentic top 10**: studio da Anthropic + OWASP. Vedi
   `docs/ricognizione/03_best_practices.md` e `04_owasp.md`.

5. **herdr è AGPL**: usalo come client, mai importare il codice.
   Stessa regola per Aperant.

6. **9router obbligatorio per voce**: senza NINEROUTER_URL il voice
   gateway fallisce al boot. Config fallback possibile (V2).

## Decision log

Vedi `docs/DECISIONS.md`. Append-only. Ogni nuova decisione va
aggiunta in fondo con data e motivazione.

## Domande ancora aperte

- D-013: 9router unico o multi-gateway?
- D-014: Cognee o Mem0 per memoria?
- D-015: V2 voce LiveKit self-host o Daily.co cloud?
- D-016: multi-tenant (altri utenti) o solo Sergio?
- D-017: pricing model per V1?

Se stai leggendo questo e hai dubbi, chiedi a Sergio o guarda
session_search per conversazioni passate su "bismuth".

## Contatti

- Repo: https://github.com/biodoia/bismuth
- Owner: biodoia (Sergio Martinelli)
- Lead agent: Hermes (Bremes)
- Lavori in corso: `git log --oneline -20`

---

## P0 status (aggiornato fine sessione 1)

| F# | item | status | commit |
|-----|------|--------|--------|
| F1  | all HTTP routes + WS | DONE | 61bd099 |
| F2  | SQLite CRUD queries | DONE | 9788824 |
| F3  | pane.Spawn + worktree | DONE (partial) | dcdd086 |
| F4  | WS read/write pumps + client | DONE | a779866 |
| F5  | 7 MCP tools bismuth-team | DONE | 916fce6 |
| F6  | smoke tests db + mcp | DONE | 53b7bbe |
| F7  | web PWA voice | DONE | c17a03d |
| F8  | web PWA terminal + feed | DONE | c17a03d |
| F9  | auth tailscale-only | DONE | 61bd099 |
| F10 | end-to-end smoke (spawn+kill) | DONE | 1addc41 |

**Tutto P0 done.** Server funziona, 12 test verdi, MCP funziona,
end-to-end passa.

## P1 backlog (sessione 2+)

| P1# | item | effort |
|-----|------|--------|
| P1-1 | `cd web && npm install && npm run build` (LSP cleanup) | 5min |
| P1-2 | prompts/*.md per gli 8 ruoli mancanti (oggi: solo planner/implementer/reviewer/verifier) | 30min |
| P1-3 | throttling persist scrollback (coalescer 256B/500ms) | 1h |
| P1-4 | WS live attach su terminal (sostituire polling 3s) | 2h |
| P1-5 | bidirezionale terminal (write pane from web) | 1h |
| P1-6 | bidirezionale voce (push-to-talk + barge-in) | 2h |
| P1-7 | audit log rendering in UI (event timeline con verifica SHA) | 1h |
| P1-8 | installare bismuth-team MCP sui 4 worker target (omx/omc/omo/omp) | 1h |
| P1-9 | mcp_test: aggiungere test dispatch reale (mock stdio) | 1h |
| P1-10 | end-to-end con worker reale (omx o claude invece di bash) | 2h |

## P2 backlog (V2 features)

- LiveKit per voce real-time (sostituisce HTTP streaming)
- Wake-word "bismuth" italiano
- Cognee graph + vector memory al posto di LIKE fallback
- TUI client (Cobra + bubbletea)
- openclaw channels integration (Telegram/Discord notifications)
- aigoproxy routes per bismuth
- Cost guardrail enforcement attivo (blocca agent quando supera ceiling)
- Multi-tenant (più utenti sullo stesso server)

## Quick reference (per riprendere)

```bash
cd ~/projects/bismuth
export GOTMPDIR=/home/lisergico25/.tmp   # /tmp è 100% pieno
go mod tidy
go build -o bin/bismuth ./cmd/bismuth

# run server (con 9router stub)
NINEROUTER_URL=http://127.0.0.1:9999 NINEROUTER_KEY=x \
  ./bin/bismuth serve --config config.yaml

# smoke test (richiede server NON attivo)
./scripts/smoke.sh

# tests
go test ./...

# install Hermes skill
./bin/bismuth cli skill-install

# operator CLI (richiede server attivo)
./bin/bismuth cli list-agents
./bin/bismuth cli spawn --role implementer --cli bash --task "echo ciao"
./bin/bismuth cli status
```

## Per il prossimo agent

1. Sei in P1. Vedi la tabella sopra per priorita.
2. Niente in P0 è bloccante — l'infrastruttura regge.
3. Worktree-per-task è già attivo ma la creazione richiede che il
   repo git di destinazione esista. Per smoke test bash viene spawnato
   senza worktree.
4. Il frontend (web/) richiede `npm install` per buildare. I TS errors
   di LSP sono attesi senza dipendenze installate.
5. Se vuoi aggiungere un worker CLI nuovo (es. droid, vibe), basta
   una riga in `internal/roles/catalog.go` e una riga nei 4
   `prompts/*.md` esistenti.
