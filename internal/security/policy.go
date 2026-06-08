// Package security enforces the OWASP Agentic Top 10 mitigations.
//
// Maps to OWASP ASI01-10 (https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/):
//
//   ASI01 Agent Goal Hijack       -> input sanitization
//   ASI02 Tool Misuse             -> command allowlist + risk scoring
//   ASI03 Identity/Privilege      -> per-agent scoped permissions
//   ASI04 Supply Chain            -> pinned versions, signed MCP
//   ASI05 Unexpected Code Exec    -> dry-run + confirm for destructive ops
//   ASI06 Memory Poisoning        -> Cognee write/read scope + provenance
//   ASI07 Inter-Agent Comms       -> signed messages, non-replayable nonce
//   ASI08 Cascading Failures      -> independent verification per worker
//   ASI09 Human-Agent Trust       -> always show diff, never auto-merge
//   ASI10 Rogue Agents            -> kill switch + audit log + alerts
package security

import (
	"strings"

	"github.com/biodoia/bismuth/internal/config"
)

// Policy is the runtime security policy.
type Policy struct {
	cfg config.SecurityCfg
}

// New returns the policy.
func New(cfg config.SecurityCfg) *Policy {
	return &Policy{cfg: cfg}
}

// AllowsCommand returns true if the given command line is allowed.
//
// Rule: extract the first token (binary name). If it's in the deny
// list, reject. If allowed list is non-empty and the binary is not
// in it, reject. Otherwise allow.
func (p *Policy) AllowsCommand(line string) bool {
	tok := strings.Fields(strings.TrimSpace(line))
	if len(tok) == 0 {
		return false
	}
	bin := tok[0]
	// strip path
	if i := strings.LastIndex(bin, "/"); i >= 0 {
		bin = bin[i+1:]
	}
	for _, d := range p.cfg.DeniedCommands {
		if d == bin {
			return false
		}
		// also block e.g. "rm -rf" as substring for dangerous args
		if strings.HasPrefix(d, bin+" ") && strings.Contains(line, d) {
			return false
		}
	}
	if len(p.cfg.AllowedCommands) == 0 {
		return true
	}
	for _, a := range p.cfg.AllowedCommands {
		if a == bin {
			return true
		}
	}
	return false
}

// RequiresHumanApproval returns true if the action needs human OK.
func (p *Policy) RequiresHumanApproval(action string) bool {
	switch action {
	case "git_push", "git_force_push", "rm_force", "sudo", "merge_to_main":
		return true
	}
	return p.cfg.HumanApprovalForPush && strings.HasPrefix(action, "push")
}

// CostOK returns true if the proposed spend is within the ceiling.
func (p *Policy) CostOK(spent, ceiling float64) bool {
	if ceiling <= 0 {
		ceiling = p.cfg.CostCeilingPerTaskUSD
	}
	return spent < ceiling
}
