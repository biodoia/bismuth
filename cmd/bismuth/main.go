// Package main is the bismuth server entry point.
//
// bismuth is an autonomous multi-agent coding team multiplexer with live
// voice control and web UI. It orchestrates a team of AI coding agent
// CLIs (omx, omc, omo, omp, claude, codex, opencode, ...) as a
// coordinated swarm.
//
// Architecture (V1):
//   - SQLite (modernc.org/sqlite, cgo-free) for persistence
//   - WebSocket pub/sub for realtime event streaming
//   - HTTP REST API for control
//   - PTY pane management via charmbracelet/x/xpty
//   - STT/TTS via 9router (HTTP streaming)
//   - MCP server "bismuth-team" installable on each worker
//   - Web PWA for voice + terminal remote + realtime feed
//
// See docs/ARCHITECTURE.md for the full picture.
package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/biodoia/bismuth/internal/api"
	"github.com/biodoia/bismuth/internal/audit"
	"github.com/biodoia/bismuth/internal/bus"
	"github.com/biodoia/bismuth/internal/config"
	"github.com/biodoia/bismuth/internal/db"
	"github.com/biodoia/bismuth/internal/hermes"
	"github.com/biodoia/bismuth/internal/mcp"
	"github.com/biodoia/bismuth/internal/tui"
	"github.com/biodoia/bismuth/internal/pane"
	"github.com/biodoia/bismuth/internal/roles"
	"github.com/biodoia/bismuth/internal/security"
	"github.com/biodoia/bismuth/internal/voice"
	"github.com/spf13/cobra"
)

var (
	cfgFile string
	rootCmd = &cobra.Command{
		Use:   "bismuth",
		Short: "Autonomous multi-agent coding team multiplexer",
		Long:  "bismuth orchestrates a team of AI coding agents with live voice control and web UI.",
	}
	serveCmd = &cobra.Command{
		Use:   "serve",
		Short: "Run the bismuth server (HTTP + WebSocket + MCP stdio when invoked as 'bismuth-mcp')",
		RunE:  runServe,
	}
	tuiCmd = &cobra.Command{
		Use:   "tui",
		Short: "Run the local TUI client attached to a running bismuth server",
		RunE:  runTUI,
	}
	mcpCmd = &cobra.Command{
		Use:   "mcp",
		Short: "Run the bismuth-team MCP server on stdio (installed on worker agents)",
		RunE:  runMCP,
	}
	cliCmd = &cobra.Command{
		Use:   "cli",
		Short: "Operator CLI: list-agents, list-tasks, spawn, send, read, assign, kill, merge, status, skill-install",
	}
	cliListAgentsCmd  = &cobra.Command{Use: "list-agents", RunE: runCLIListAgents}
	cliListTasksCmd   = &cobra.Command{Use: "list-tasks", RunE: runCLIListTasks}
	cliSpawnCmd       = &cobra.Command{Use: "spawn", RunE: runCLISpawn}
	cliSendCmd        = &cobra.Command{Use: "send", RunE: runCLISend}
	cliReadCmd        = &cobra.Command{Use: "read", RunE: runCLIRead}
	cliAssignCmd      = &cobra.Command{Use: "assign", RunE: runCLIAssign}
	cliKillCmd        = &cobra.Command{Use: "kill", RunE: runCLIKill}
	cliMergeCmd       = &cobra.Command{Use: "merge", RunE: runCLIMerge}
	cliStatusCmd      = &cobra.Command{Use: "status", RunE: runCLIStatus}
	cliSkillInstallCmd = &cobra.Command{Use: "skill-install", RunE: runCLISkillInstall}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "config.yaml", "path to config file")
	rootCmd.AddCommand(serveCmd, tuiCmd, mcpCmd, cliCmd)
	cliCmd.AddCommand(cliListAgentsCmd, cliListTasksCmd, cliSpawnCmd, cliSendCmd, cliReadCmd, cliAssignCmd, cliKillCmd, cliMergeCmd, cliStatusCmd, cliSkillInstallCmd)
	cliSpawnCmd.Flags().String("role", "", "role id (planner, implementer, ...)")
	cliSpawnCmd.Flags().String("cli", "", "worker CLI (omx, omc, omo, omp, ...)")
	cliSpawnCmd.Flags().String("task", "", "task description / initial prompt")
	cliSendCmd.Flags().String("agent", "", "agent id")
	cliSendCmd.Flags().String("data", "", "text to send to the pane")
	cliReadCmd.Flags().String("agent", "", "agent id")
	cliReadCmd.Flags().Int("n", 200, "lines to read")
	cliAssignCmd.Flags().String("task", "", "task id")
	cliAssignCmd.Flags().String("agent", "", "agent id")
	cliKillCmd.Flags().String("agent", "", "agent id")
	cliMergeCmd.Flags().String("task", "", "task id")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServe(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(cfgFile)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := db.Open(ctx, cfg.DB.Path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer store.Close()
	sqlDB := store.DB()

	auditLog, err := audit.New(sqlDB, cfg.Audit.Salt)
	if err != nil {
		return fmt.Errorf("init audit: %w", err)
	}

	busServer := bus.New(sqlDB)
	defer busServer.Close()

	paneMgr := pane.NewManager(sqlDB, busServer, cfg.Pane)
	defer paneMgr.Close()

	voiceGW, err := voice.NewGateway(ctx, cfg.Voice, sqlDB, busServer)
	if err != nil {
		return fmt.Errorf("init voice: %w", err)
	}
	defer voiceGW.Close()

	sec := security.New(cfg.Security)
	catalog := roles.DefaultCatalog()
	repoRoot := cfg.Pane.Workdir
	if repoRoot == "" || repoRoot == "." {
		repoRoot = "."
	}

	srv := api.NewServer(cfg, store, busServer, paneMgr, voiceGW, auditLog, sec, catalog, repoRoot)
	return srv.Run(ctx)
}

func runTUI(cmd *cobra.Command, args []string) error {
	baseURL := os.Getenv("BISMUTH_URL")
	if baseURL == "" {
		baseURL = "http://127.0.0.1:9000"
	}
	return tui.Run(baseURL)
}

func runMCP(cmd *cobra.Command, args []string) error {
	// /tmp is often 100% full on this host. Use a writable per-call
	// path under the user's home.
	dbPath := os.Getenv("BISMUTH_MCP_DB")
	if dbPath == "" {
		dbPath = os.Getenv("HOME") + "/.cache/bismuth/mcp.db"
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	store, err := db.Open(context.Background(), dbPath)
	if err != nil {
		return err
	}
	defer store.Close()
	srv := mcp.NewServer(store.DB())
	return srv.Run(context.Background())
}

func defaultDBPath() string {
	if cfgFile == "" {
		return "./data/bismuth.db"
	}
	// best-effort: try to load config and use its db path; if that
	// fails, fall back to a sane default.
	if cfg, err := config.Load(cfgFile); err == nil {
		return cfg.DB.Path
	}
	return "./data/bismuth.db"
}

// --- CLI subcommands (operator surface) -------------------------------------
//
// These talk to a running bismuth server over HTTP. If no server is
// running, they print a helpful message. V1: simple HTTP-only client.

func apiBase() string { return "http://127.0.0.1:9000" }

func runCLIListAgents(cmd *cobra.Command, args []string) error {
	out, _ := httpGET(apiBase() + "/api/v1/agents")
	fmt.Println(out)
	return nil
}

func runCLIListTasks(cmd *cobra.Command, args []string) error {
	out, _ := httpGET(apiBase() + "/api/v1/tasks")
	fmt.Println(out)
	return nil
}

func runCLISpawn(cmd *cobra.Command, args []string) error {
	role, _ := cmd.Flags().GetString("role")
	cli, _ := cmd.Flags().GetString("cli")
	task, _ := cmd.Flags().GetString("task")
	if role == "" || cli == "" || task == "" {
		return fmt.Errorf("--role, --cli, --task are required")
	}
	out, _ := httpPOST(apiBase()+"/api/v1/agents", map[string]any{
		"role": role, "cli": cli, "task": task,
	})
	fmt.Println(out)
	return nil
}

func runCLISend(cmd *cobra.Command, args []string) error {
	agent, _ := cmd.Flags().GetString("agent")
	data, _ := cmd.Flags().GetString("data")
	if agent == "" || data == "" {
		return fmt.Errorf("--agent and --data are required")
	}
	out, _ := httpPOST(apiBase()+"/api/v1/agents/"+agent+"/send", map[string]any{
		"data_b64": base64.StdEncoding.EncodeToString([]byte(data)),
	})
	fmt.Println(out)
	return nil
}

func runCLIRead(cmd *cobra.Command, args []string) error {
	agent, _ := cmd.Flags().GetString("agent")
	n, _ := cmd.Flags().GetInt("n")
	if agent == "" {
		return fmt.Errorf("--agent is required")
	}
	out, _ := httpGET(fmt.Sprintf("%s/api/v1/agents/%s/read?n=%d", apiBase(), agent, n))
	fmt.Println(out)
	return nil
}

func runCLIAssign(cmd *cobra.Command, args []string) error {
	task, _ := cmd.Flags().GetString("task")
	agent, _ := cmd.Flags().GetString("agent")
	if task == "" || agent == "" {
		return fmt.Errorf("--task and --agent are required")
	}
	out, _ := httpPOST(apiBase()+"/api/v1/tasks/"+task+"/assign", map[string]any{"agent_id": agent})
	fmt.Println(out)
	return nil
}

func runCLIKill(cmd *cobra.Command, args []string) error {
	agent, _ := cmd.Flags().GetString("agent")
	if agent == "" {
		return fmt.Errorf("--agent is required")
	}
	out, _ := httpPOST(apiBase()+"/api/v1/agents/"+agent+"/kill", nil)
	fmt.Println(out)
	return nil
}

func runCLIMerge(cmd *cobra.Command, args []string) error {
	task, _ := cmd.Flags().GetString("task")
	if task == "" {
		return fmt.Errorf("--task is required")
	}
	out, _ := httpPOST(apiBase()+"/api/v1/tasks/"+task+"/merge", nil)
	fmt.Println(out)
	return nil
}

func runCLIStatus(cmd *cobra.Command, args []string) error {
	out, _ := httpGET(apiBase() + "/api/v1/agents")
	fmt.Println("agents:", out)
	out, _ = httpGET(apiBase() + "/api/v1/tasks")
	fmt.Println("tasks:", out)
	return nil
}

func runCLISkillInstall(cmd *cobra.Command, args []string) error {
	dest := os.Getenv("HOME") + "/.claude/skills"
	if err := hermes.Install(dest); err != nil {
		return err
	}
	fmt.Println("installed at", dest+"/bismuth-control/SKILL.md")
	return nil
}

func httpGET(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}

func httpPOST(url string, body any) (string, error) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, _ := http.NewRequest("POST", url, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b), nil
}
