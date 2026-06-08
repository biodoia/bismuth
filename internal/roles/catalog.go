// Package roles defines the 8-12 specialized agent roles in bismuth.
//
// A Role is a named bundle of:
//   - the preferred CLI binary (omx, omc, omo, omp, claude, codex, ...)
//   - the default LLM model (resolved through 9router)
//   - the default reasoning_effort / variant
//   - a system-prompt fragment (loaded from prompts/<role>.md)
//   - MCP servers the role should have installed
//   - tools the role is allowed to use (allowlist)
//   - cost ceiling per task
//
// The catalog is intentionally explicit (one struct per role) so that
// tweaking a role is a one-line PR.
package roles

// Role is a named specialized agent configuration.
type Role struct {
	ID            string   `yaml:"id"`
	Name          string   `yaml:"name"`
	Description   string   `yaml:"description"`
	CLIs          []string `yaml:"clis"`           // preferred worker CLI in order
	DefaultModel  string   `yaml:"default_model"`  // e.g. "anthropic/claude-opus-4-7"
	Effort        string   `yaml:"effort"`         // low | medium | high | max
	MCPServers    []string `yaml:"mcp_servers"`    // to install
	AllowedTools  []string `yaml:"allowed_tools"`  // tool allowlist
	CostCeilingUSD float64 `yaml:"cost_ceiling_usd"`
	PromptPath    string   `yaml:"prompt_path"`    // path under /prompts/
}

// Catalog is the full set of roles.
type Catalog struct {
	Roles []Role `yaml:"roles"`
}

// DefaultCatalog returns the V1 8-role catalog, specialized from the
// 21 omx agent TOML + 11 omo agents + 19 omc prompt set.
func DefaultCatalog() Catalog {
	return Catalog{
		Roles: []Role{
			{ID: "planner", Name: "Planner", Description: "Turns high-level goals into actionable task plans. Does not implement.", CLIs: []string{"omx"}, DefaultModel: "anthropic/claude-opus-4-7", Effort: "max", CostCeilingUSD: 1.0, PromptPath: "prompts/planner.md"},
			{ID: "architect", Name: "Architect", Description: "Designs system structure, picks libraries, draws boundaries. Writes ADRs.", CLIs: []string{"omx"}, DefaultModel: "anthropic/claude-opus-4-7", Effort: "max", CostCeilingUSD: 1.5, PromptPath: "prompts/architect.md"},
			{ID: "implementer", Name: "Implementer", Description: "Writes the code. Smallest viable diff. Always verifies with tests.", CLIs: []string{"omx", "omc"}, DefaultModel: "openai/gpt-5.5", Effort: "medium", CostCeilingUSD: 2.0, PromptPath: "prompts/implementer.md"},
			{ID: "reviewer", Name: "Reviewer", Description: "Reviews diffs for correctness, style, security. Never writes production code.", CLIs: []string{"omc"}, DefaultModel: "anthropic/claude-opus-4-7", Effort: "high", CostCeilingUSD: 1.0, PromptPath: "prompts/reviewer.md"},
			{ID: "tester", Name: "Tester", Description: "Writes and runs tests. TDD where it makes sense. Reports coverage.", CLIs: []string{"omx"}, DefaultModel: "anthropic/claude-sonnet-4-6", Effort: "medium", CostCeilingUSD: 1.0, PromptPath: "prompts/tester.md"},
			{ID: "security", Name: "Security Reviewer", Description: "Audits for OWASP Top 10 + agentic ASI01-10. Threat model.", CLIs: []string{"omc"}, DefaultModel: "anthropic/claude-opus-4-7", Effort: "max", CostCeilingUSD: 1.5, PromptPath: "prompts/security.md"},
			{ID: "debug", Name: "Debugger", Description: "Reproduces bugs, isolates root cause, proposes fix. Logs evidence.", CLIs: []string{"omc"}, DefaultModel: "anthropic/claude-opus-4-7", Effort: "max", CostCeilingUSD: 1.5, PromptPath: "prompts/debug.md"},
			{ID: "refactor", Name: "Refactorer", Description: "Cleans up code without changing behavior. Behavior-preserving diffs only.", CLIs: []string{"omc"}, DefaultModel: "anthropic/claude-sonnet-4-6", Effort: "medium", CostCeilingUSD: 1.0, PromptPath: "prompts/refactor.md"},
			{ID: "documenter", Name: "Documenter", Description: "Writes READMEs, API docs, ADRs, CHANGELOG. Pulls from code, not from memory.", CLIs: []string{"omx"}, DefaultModel: "google/gemini-3-flash", Effort: "medium", CostCeilingUSD: 0.5, PromptPath: "prompts/documenter.md"},
			{ID: "devops", Name: "DevOps", Description: "CI/CD, infra, deploys, observability. Worktree + branch hygiene.", CLIs: []string{"omx"}, DefaultModel: "openai/gpt-5.4", Effort: "high", CostCeilingUSD: 1.0, PromptPath: "prompts/devops.md"},
			{ID: "critic", Name: "Critic", Description: "Devil's advocate. Attacks the plan, finds gaps, demands evidence.", CLIs: []string{"omc"}, DefaultModel: "anthropic/claude-opus-4-7", Effort: "max", CostCeilingUSD: 0.8, PromptPath: "prompts/critic.md"},
			{ID: "verifier", Name: "Verifier", Description: "End-state check: tests pass, build green, lint clean, security clean. Issues OK/NOT-OK.", CLIs: []string{"omc"}, DefaultModel: "openai/gpt-5.4", Effort: "xhigh", CostCeilingUSD: 0.5, PromptPath: "prompts/verifier.md"},
		},
	}
}
