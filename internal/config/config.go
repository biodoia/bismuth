// Package config loads the bismuth YAML config.
//
// Example config.yaml:
//
//	server:
//	  host: 0.0.0.0
//	  port: 9000
//	db:
//	  path: ./data/bismuth.db
//	voice:
//	  stt_provider: groq          # groq | deepgram | openai | gemini
//	  stt_model: whisper-large-v3-turbo
//	  tts_provider: edge          # edge | openai | elevenlabs | google
//	  tts_voice: it-IT-IsabellaNeural
//	security:
//	  allowed_commands: [...]
//	  cost_ceiling_per_task_usd: 2.0
//	audit:
//	  salt: changeme
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server    ServerCfg                    `yaml:"server"`
	DB        DBCfg                        `yaml:"db"`
	Pane      PaneCfg                      `yaml:"pane"`
	Voice     VoiceCfg                     `yaml:"voice"`
	Security  SecurityCfg                  `yaml:"security"`
	Audit     AuditCfg                     `yaml:"audit"`
	NineR     NineRCfg                     `yaml:"ninerouter"`
	API       APICfg                       `yaml:"api"`
	LiveKit   LiveKitCfg                   `yaml:"livekit"`
	Bridge    BridgeCfg                    `yaml:"bridge"`
	Memory    MemoryCfg                    `yaml:"memory"`
	Providers map[string]ProviderCfg       `yaml:"providers"`
	CLIEnv    map[string]map[string]string `yaml:"cli_env"`
}

// LiveKitCfg configures the real LiveKit voice path (P7-a). All three
// fields set => enabled; otherwise the HTTP voice-room stubs answer.
type LiveKitCfg struct {
	URL       string `yaml:"url"`
	APIKey    string `yaml:"api_key"`
	APISecret string `yaml:"api_secret"`
}

// BridgeCfg configures the Telegram/Discord notification + remote
// command bridge (P7-d). Values support ${VAR} env expansion.
type BridgeCfg struct {
	TelegramToken     string `yaml:"telegram_token"`
	TelegramChatID    string `yaml:"telegram_chat_id"`
	DiscordWebhookURL string `yaml:"discord_webhook_url"`
	APIBase           string `yaml:"api_base"`
	PollIntervalMS    int    `yaml:"poll_interval_ms"`
}

// MemoryCfg selects the shared-memory backend (P7-c). With Mem0
// configured the FTS5 store remains as fallback.
type MemoryCfg struct {
	Mem0BaseURL string `yaml:"mem0_base_url"`
	Mem0APIKey  string `yaml:"mem0_api_key"`
}

type ServerCfg struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DBCfg struct {
	Path string `yaml:"path"`
}

type PaneCfg struct {
	DefaultShell string `yaml:"default_shell"`
	Workdir      string `yaml:"workdir"`
}

type VoiceCfg struct {
	STTProvider string `yaml:"stt_provider"`
	STTModel    string `yaml:"stt_model"`
	TTSProvider string `yaml:"tts_provider"`
	TTSVoice    string `yaml:"tts_voice"`
	Language    string `yaml:"language"`
	WakeWord    string `yaml:"wake_word"`
	VADMs       int    `yaml:"vad_silence_ms"`
}

type SecurityCfg struct {
	AllowedCommands       []string `yaml:"allowed_commands"`
	DeniedCommands        []string `yaml:"denied_commands"`
	CostCeilingPerTaskUSD float64  `yaml:"cost_ceiling_per_task_usd"`
	WorktreeRequired      bool     `yaml:"worktree_required"`
	HumanApprovalForPush  bool     `yaml:"human_approval_for_push"`
}

type AuditCfg struct {
	Salt string `yaml:"salt"`
}

type NineRCfg struct {
	URL string `yaml:"url"`
	Key string `yaml:"key"`
}

type APICfg struct {
	TailscaleOnly bool     `yaml:"tailscale_only"`
	AllowedCIDRs  []string `yaml:"allowed_cidrs"`
}

type ProviderCfg struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	c.applyDefaults()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Host == "" {
		c.Server.Host = "0.0.0.0"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 9000
	}
	if c.DB.Path == "" {
		c.DB.Path = "./data/bismuth.db"
	}
	if c.Pane.DefaultShell == "" {
		c.Pane.DefaultShell = "bash"
	}
	if c.Pane.Workdir == "" {
		c.Pane.Workdir = "."
	}
	if c.Voice.STTProvider == "" {
		c.Voice.STTProvider = "groq"
		c.Voice.STTModel = "whisper-large-v3-turbo"
	}
	if c.Voice.TTSProvider == "" {
		c.Voice.TTSProvider = "edge"
		c.Voice.TTSVoice = "it-IT-IsabellaNeural"
	}
	if c.Voice.Language == "" {
		c.Voice.Language = "it"
	}
	if c.Voice.WakeWord == "" {
		c.Voice.WakeWord = "bismuth"
	}
	if c.Voice.VADMs == 0 {
		c.Voice.VADMs = 1500
	}
	// Secrets support ${VAR} env references.
	c.LiveKit.URL = resolveEnv(c.LiveKit.URL)
	c.LiveKit.APIKey = resolveEnv(c.LiveKit.APIKey)
	c.LiveKit.APISecret = resolveEnv(c.LiveKit.APISecret)
	c.Bridge.TelegramToken = resolveEnv(c.Bridge.TelegramToken)
	c.Bridge.TelegramChatID = resolveEnv(c.Bridge.TelegramChatID)
	c.Bridge.DiscordWebhookURL = resolveEnv(c.Bridge.DiscordWebhookURL)
	c.Bridge.APIBase = resolveEnv(c.Bridge.APIBase)
	c.Memory.Mem0BaseURL = resolveEnv(c.Memory.Mem0BaseURL)
	c.Memory.Mem0APIKey = resolveEnv(c.Memory.Mem0APIKey)
	if c.Security.CostCeilingPerTaskUSD == 0 {
		c.Security.CostCeilingPerTaskUSD = 2.0
	}
	if c.Security.AllowedCommands == nil {
		c.Security.AllowedCommands = []string{
			"ls", "cat", "grep", "find", "git", "go", "npm", "node",
			"python", "pytest", "cargo", "rustc", "make", "echo",
			"pwd", "cd", "head", "tail", "wc", "tree", "curl",
		}
	}
	if c.Security.DeniedCommands == nil {
		c.Security.DeniedCommands = []string{
			"rm", "rmdir", "sudo", "su", "dd", "mkfs", "fdisk",
			"shutdown", "reboot", "halt", "poweroff", "kill -9",
		}
	}
	c.Security.WorktreeRequired = true
	c.Security.HumanApprovalForPush = true
}

func (c *Config) validate() error {
	if c.Audit.Salt == "" || c.Audit.Salt == "changeme" {
		return fmt.Errorf("audit.salt must be set to a random string")
	}
	return nil
}

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// resolveEnv replaces ${VAR} patterns with the value of the
// environment variable VAR. If VAR is not set, the pattern is
// replaced with an empty string.
func resolveEnv(s string) string {
	return envVarRe.ReplaceAllStringFunc(s, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(name)
	})
}

// EnvForCLI returns the environment variables that should be injected
// when spawning an agent with the given CLI tool (omc, omx, etc.).
// It resolves ${VAR} references from the cli_env config section.
func (c *Config) EnvForCLI(cli string) []string {
	if c.CLIEnv == nil {
		return nil
	}
	envMap, ok := c.CLIEnv[cli]
	if !ok {
		return nil
	}
	var env []string
	for k, v := range envMap {
		resolved := resolveEnv(v)
		if resolved != "" {
			env = append(env, k+"="+resolved)
		}
	}
	return env
}
