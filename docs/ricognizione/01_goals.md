# Ricognizione 01 — bismuth goals e scope (sessione 1, 8 giu 2026)

Source: conversazione iniziale Sergio + Bremes, 8 giu 2026.

## Goal (verbatim, ricostruito)

Sergio vuole un sistema che:

1. Sia un emulatore di terminale O un programma tipo tmux/tuios che
   viva dentro un terminale (NO sostituisca tmux, ANZI ci lavora
   accanto).
2. Multiplexa N sessioni in singola finestra o più finestre.
3. Una sessione è per forza Hermes (lead, interfaccia vocale e di
   comando).
4. Le altre N-1 sessioni sono chat con agent AI (claude, codex,
   oh-my-pi/codex/claudecode/openagent, omo, omc, omo, omp, ...).
5. Hermes legge e scrive nelle altre sessioni via MCP server o
   skill installata sui worker.
6. I worker sanno di essere in un team (non sono isolati).
7. Massimo livello di automazione possibile.
8. Integrazione vocale live, anche via smartphone app.
9. Web UI minimale = SOLO vocale + remote terminal + feed real-time.
10. Ispirato ad Aperant (AndyMik90) come modello, ma VOCALE e WEB.

## Stack emerso

- Server: Go (cgo-free SQLite, MIT, allineato a infra esistente)
- Web PWA: Vite + React 19 + TailwindCSS v4 + xterm.js
- Voice gateway: 9router STT (Groq Whisper turbo) + 9router TTS
  (Edge TTS / ElevenLabs), HTTP streaming
- MCP: stdio JSON-RPC 2.0 (standard MCP)
- AI gateway: 9router (già attivo, 30+ provider)
- Reverse proxy: aigoproxy (già attivo, tailscale)
- Notifications: openclaw channels
- Memory: Cognee (V2, V1 = SQLite in-DB)
- Multiplexer: riusa herdr (già installato, 18 detector incluso
  Hermes), omc team N:agent
- LLM models: 19+ modelli configurati in omo (opus-4-7, sonnet-4-6,
  gpt-5.5, gpt-5.4, gemini-3.1-pro, kimi-k2.5, glm-5, big-pickle, ...)

## Flow utente (Sergio, OOB 8 giu, 18:43)

```
1. IDEA:  "voglio un'app che fa X"
2. DIALOGUE: Hermes botta-e-risposta, fa domande, esplora tradeoffs
3. RESEARCH: spawna researcher, legge docs, scansiona app simili
4. REASON: planner scrive spec concrete
5. SPECS: presenta all'utente per review
6. MOCKUPS: mostra 3-5 UI mockup tra cui scegliere
7. OK: utente dice "ok" / "vai"
8. AUTONOMOUS BUILD: team coordina, commit+push frequenti, test
   come un umano, updates periodici
9. DELIVER: PR aperto, human approve, merge
```

## Caratteristiche MUST

- Massimo livello di automazione (D-012)
- Commit+push frequenti (ogni 15-30 min di lavoro, ogni task boundary)
- Test come un umano (tester role + verifier)
- Updates periodici (Telegram/Discord via openclaw + WebSocket feed)
- Self-validation: agent si auto-controlla PRIMA di passare al
  prossimo step
- Team-consapevolezza: worker vedono i peer e lo stato collettivo
- Cost ceiling per task (default $2)
- Worktree-per-task automatico
- PR obbligatorio per merge (human approve finale)

## Caratteristiche NICE-TO-HAVE (V2+)

- Wake-word "bismuth" in IT
- LiveKit per voce real-time (VAD, barge-in)
- Multi-tenant
- Pricing model (se chiudi sorgente)
- Auto-merge di task "trusted" (test+lint+security pass, configurabile)
- TUI client (oltre a web PWA)
- Smartphone app nativa (wrapper PWA va bene per V1)
- Cognee/Mem0 come memoria semantica

## NON-goals (V1)

- IDE / code editor
- Visual kanban (terminal + web PWA feed bastano)
- Multi-tenant (solo Sergio)
- Cloud deploy
- Auto-merge
- Sostituire omx/omc/omo/omp (restano primari)
- Desktop Electron (web PWA invece, va su qualsiasi browser/OS)

## Original ask timeline (8 giu 2026)

- 18:13: "voglio un multiplexer ... con Hermes come orchestratore ...
  e oh my pi, oh my codex, oh my claude e oh my openagents come
  sistemi da usare per codificare"
- 18:18: "voglio il massimo livello di automazione"
- 18:25: "voglio qualcosa dotato di integrazione vocale live, anche
  via smartphone app. e che funzioni bene o male come Aperant"
- 18:32 (OOB): "però come interfaccia web voglio solo il controllo
  vocale e controllo remoto del terminale, con messaggi in real time
  sullo status dei vari agent e su quello che si dicono"
- 18:43 (OOB): "voglio un sistema che funzioni in modo che io gli
  dico una mia idea, lui conversa con me in botta e risposta, poi fa
  alcune ricerche, ragiona bene, immagina concretamente il
  risultato, mi elenca le specs, prende decisioni per migliorare
  l'app qualora io abbia chiesto o desiderato non sufficientemente,
  mi mostra alcune interfacce grafiche tra le quali scegliere ed
  infine mi chiede se voglio cambiare qualcosa, discutere di altro,
  e quando dico ook comincia lui parte totalmente in maniera
  autonoma. ogni fase completata fa commit, push, frequenti
  salvataggi, e testa tutto come se fosse un umano. si consulta
  con i vari agent, ragiona come un team di sbiluppo vero e proprio.
  e mi manda oogni tanto updates sul lavoro svolto"
- 18:51 (OOB): "voglio il massimo livello di automazione. intanto
  crea il nuovo repo e fai commit e push di tuttto quello che
  abbiamo fatto. e scrivi altra documentazione affinche sia
  confortevole per un altro o altri agents continuare il lavoro
  da dove lo hai lasciato, te compreso"
- 18:54 (OOB): "procedi per passi, trova, elimina vecchi files
  inutili temp, proccedi. perchè ora hai zero e sono cazzi"
- 18:56 (OOB): "fottitene di contare esattamente, comincia a
  liberare"
- 19:00 (OOB): "SI MA bro prima risolvi il proble dello spazio
  cristo dio"
