// Package api is the HTTP/WebSocket server for bismuth.
//
// Endpoints (V1):
//
//   GET  /healthz                       -- liveness
//   GET  /api/v1/agents                 -- list all agents
//   POST /api/v1/agents                 -- spawn agent  { role, name, cli, args, task }
//   GET  /api/v1/agents/:id             -- agent detail
//   POST /api/v1/agents/:id/send        -- send bytes to pane     { data_b64 }
//   GET  /api/v1/agents/:id/read        -- read last N lines     ?n=200
//   POST /api/v1/agents/:id/kill        -- terminate agent
//   GET  /api/v1/tasks                  -- list tasks
//   POST /api/v1/tasks                  -- create task  { title, description, priority }
//   GET  /api/v1/tasks/:id              -- task detail
//   POST /api/v1/tasks/:id/assign       -- assign to agent
//   POST /api/v1/tasks/:id/merge        -- merge branch (human OK)
//   GET  /api/v1/roles                  -- catalog of role definitions
//   GET  /api/v1/events                 -- last N events        ?types=a,b&agent_id=x&limit=200
//   GET  /api/v1/ws                     -- WebSocket subscribe   ?types=&agent_id=
//   POST /v1/voice/stt                  -- multipart audio in  -> { text }
//   POST /v1/voice/speak                -- { text } -> { audio_b64, format }
//   POST /v1/voice/command              -- { text } -> command parsed + executed
//
// See internal/bus/bus.go for the Event wire format.
package api

import (
	"context"
	"database/sql"
	"net/http"
	"time"

	"github.com/biodoia/bismuth/internal/audit"
	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/pane"
	"github.com/biodoia/bismuth/internal/voice"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
)

// Server bundles the dependencies.
type Server struct {
	cfg   config.APICfg
	db    *sql.DB
	bus   *bus.Bus
	pane  *pane.Manager
	voice *voice.Gateway
	audit *audit.Log

	upgrader websocket.Upgrader
}

// NewServer wires the HTTP routes.
func NewServer(cfg config.APICfg, db *sql.DB, b *bus.Bus, pm *pane.Manager, v *voice.Gateway, a *audit.Log) *Server {
	s := &Server{
		cfg:   cfg,
		db:    db,
		bus:   b,
		pane:  pm,
		voice: v,
		audit: a,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin:     func(r *http.Request) bool { return true }, // tailscale-only assumed
		},
	}
	return s
}

// Run starts the HTTP server until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})

	// TODO(sessione+1): register all routes listed in package doc.
	// For V1 skeleton we expose /api/v1/agents, /api/v1/tasks,
	// /api/v1/events, /api/v1/ws, /v1/voice/*.

	srv := &http.Server{
		Addr:              ":9000",
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	return srv.ListenAndServe()
}
