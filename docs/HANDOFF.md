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
