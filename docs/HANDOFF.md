# bismuth — Handoff

> For the next session (Bremes o qualsiasi altro agent). Leggi questo
> file PRIMA di toccare codice. Poi leggi `ARCHITECTURE.md`, `ROLES.md`,
> `DECISIONS.md`. Poi le `docs/ricognizione/*.md` per il contesto
> completo della sessione 1.

## Stato attuale (9 giu 2026, dopo sessione 3)

- Repo: https://github.com/biodoia/bismuth (branch main, 21 commits)
- Binario: `bin/bismuth` (~20 MB, MIT, cgo-free)
- Build: `go build ./...` OK, `go test ./...` OK (14 test verdi)
- Web: `web/` build OK (tsc strict + vite, 0 errori)
- TUI: `bismuth tui` funziona (bubbletea v1, agent list + event feed)
- Tailnet: `bismuth.biodoia.ts.net` via aigoproxy (:80 → localhost:9000, auth tailscale)

## P0 status — DONE (sessione 1)

Tutte le 10 feature (F1-F10) completate. Vedi git log.

## P1 status — DONE (sessione 2)

Tutti i 10 item (P1-1 a P1-10). Vedi HANDOFF sessione 2.

## P2 status — DONE (sessione 2, batch)

CI GitHub Actions, Dockerfile 3-stage, systemd unit, cost guardrail, TUI bubbletea.

## P3 status — DONE (sessione 3)

| P3# | item | status |
|-----|------|--------|
| P3-a | Prometheus metrics (counters + histograms) | DONE |
| P3-b | aigoproxy route `bismuth.biodoia.ts.net` | DONE |
| P3-c | OpenAPI 3.1 spec (19 endpoints, 6 schemas) | DONE |
| P3-d | Litestream config template | DONE |
| P3-e | Shared memory FTS5 (POST/QUERY/LIST) | DONE |

e2e verificato: shared memory FTS query OK, Prometheus `bismuth_*` metrics OK.

## P4 backlog (prossima sessione)

- LiveKit per voce real-time (sostituisce HTTP streaming)
- Wake-word "bismuth" italiano
- Cognee/Mem0 graph + vector memory (oltre FTS5)
- openclaw channels (Telegram/Discord)
- Multi-tenant (namespace isolation)
- Grafana dashboard JSON per bismuth metrics
- LLM dispatch reale (spawn omc/omx con API key + modello)
- shared_memory MCP tool (exposing FTS5 via MCP protocol)

## Bug fixati (sessione 2-3)

1. `sql.NullString` serializzato come `{"String":"x","Valid":true}`:
   aggiunto `MarshalJSON()` custom su Agent, Task, Message.
2. `json.NewEncoder(w).Encode()` falliva silenziosamente su nil `json.RawMessage`:
   sostituito con `json.Marshal()` + `w.Write()` + error logging.
3. Eventi con payload vuoto rompevano il marshal:
   `safePayload()` in bus.go scrive `"{}"` per nil.

## Convenzioni di coding

- Go 1.23, niente generics se non necessario, niente panic in prod
- Error wrappato con `fmt.Errorf("ctx: %w", err)`
- Log strutturato con `slog`
- Commit: conventional commits
- Niente secrets in repo

## Comandi utili

```bash
cd ~/projects/bismuth
export GOTMPDIR=/home/lisergico25/.tmp   # /tmp è 100% pieno
go build -o bin/bismuth ./cmd/bismuth
go test ./...

# run server
NINEROUTER_URL=http://127.0.0.1:9999 NINEROUTER_KEY=x \
  ./bin/bismuth serve --config config.yaml

# TUI client
./bin/bismuth tui

# web
cd web && npm run build

# shared memory
curl -s -X POST localhost:9000/api/v1/memory \
  -H 'Content-Type: application/json' \
  -d '{"agent_id":"agt-1","key":"decision","value":"use FTS5","tags":"arch"}'
curl -s "localhost:9000/api/v1/memory?q=fts5"

# metrics
curl -s localhost:9000/metrics | grep bismuth_
```

## Gotchas

1. **/tmp pieno** — usa GOTMPDIR=/home/lisergico25/.tmp
2. **bubbletea v1** — lipgloss v1.0.0, bubbletea v1.2.4. Non upgradare a v2.
3. **modernc/sqlite** — no CGO, SetMaxOpenConns(1) per WAL.
4. **9router** — obbligatorio per voice. Senza NINEROUTER_URL, voice fallisce.
5. **herdr AGPL** — usare come client, mai importare.
6. **aigoproxy** — bismuth.biodoia.ts.net route registrata, port 9000 claimata.

## Contatti

- Repo: https://github.com/biodoia/bismuth
- Owner: biodoia (Sergio Martinelli)
- Lead agent: Hermes (Bremes)
