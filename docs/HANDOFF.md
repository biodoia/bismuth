# bismuth — Handoff

> For the next session (Bremes o qualsiasi altro agent). Leggi questo
> file PRIMA di toccare codice. Poi leggi `ARCHITECTURE.md`, `ROLES.md`,
> `DECISIONS.md`. Poi le `docs/ricognizione/*.md` per il contesto
> completo della sessione 1.

## Stato attuale (9 giu 2026, dopo sessione 2)

- Repo: https://github.com/biodoia/bismuth (branch main, 18 commits)
- Binario: `bin/bismuth` (~20 MB, MIT, cgo-free)
- Build: `go build ./...` OK, `go test ./...` OK (14 test verdi)
- Web: `web/` build OK (tsc strict + vite, 0 errori)
- TUI: `bismuth tui` funziona (bubbletea, agent list + event feed)

## P0 status — DONE (sessione 1)

Tutte le 10 feature (F1-F10) completate. Vedi git log.

## P1 status — DONE (sessione 2)

| P1# | item | commit |
|-----|------|--------|
| P1-1 | web build (tsc + vite) | bd22324 |
| P1-2 | 8 prompt ruoli mancanti | 22c063c |
| P1-3 | coalesced scrollback 256B/500ms | 918e0ff |
| P1-4 | WS live attach terminal | f2102c3 |
| P1-5 | bidirezionale terminal | a875a24 |
| P1-6 | voice VAD push-to-talk + barge-in | dadcec2 |
| P1-7 | audit log timeline UI | cd65d3b |
| P1-8 | MCP install su omx/omc/omo/omp | 32ae35f |
| P1-9 | MCP smoke test 7 tools | b648223 |
| P1-10 | e2e con omc + JSON fix | efdccd9 |

## P2 status — DONE (sessione 2, batch)

| P2# | item | status |
|-----|------|--------|
| CI | GitHub Actions (go test + node build) | DONE |
| Dockerfile | 3-stage multi-build | DONE |
| systemd | unit file con hardening | DONE |
| costguard | cost ceiling enforcement | DONE |
| TUI | bubbletea v1 client | DONE |

## P3 backlog (prossima sessione)

- LiveKit per voce real-time (sostituisce HTTP streaming)
- Wake-word "bismuth" italiano
- Cognee/Mem0 graph + vector memory
- openclaw channels (Telegram/Discord)
- aigoproxy routes per bismuth
- Multi-tenant
- Prometheus metrics endpoint
- Grafana dashboard
- SQLite backup (Litestream/rclone)
- OpenAPI spec autogenerata

## Bug fixati (sessione 2)

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
```

## Gotchas

1. **/tmp pieno** — usa GOTMPDIR=/home/lisergico25/.tmp
2. **bubbletea v1** — lipgloss v1.0.0, bubbletea v1.2.4. Non upgradare a v2.
3. **modernc/sqlite** — no CGO, SetMaxOpenConns(1) per WAL.
4. **9router** — obbligatorio per voice. Senza NINEROUTER_URL, voice fallisce.
5. **herdr AGPL** — usare come client, mai importare.

## Contatti

- Repo: https://github.com/biodoia/bismuth
- Owner: biodoia (Sergio Martinelli)
- Lead agent: Hermes (Bremes)
