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
	"context"
	"strings"

	"github.com/biodoia/bismuth/internal/config"
)

// contextKey is used for storing the user in context.
type contextKey string

const userKey contextKey = "bismuth_user"

// User represents an authenticated user extracted from Tailscale headers.
type User struct {
	Email   string
	Name    string
	Role    string // admin, operator, viewer
	Tailscale bool
}

// Roles
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

// UserFromContext extracts the authenticated user from the request context.
func UserFromContext(ctx context.Context) *User {
	u, _ := ctx.Value(userKey).(*User)
	return u
}

// ContextWithUser injects a user into the context.
func ContextWithUser(ctx context.Context, u *User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// UserFromHeaders extracts a User from Tailscale HTTP headers.
//
// Tailscale sets these headers when TailscaleServe or TailscaleFunnel is used:
//   - Tailscale-User-Login: email (e.g. "sergio@example.com")
//   - Tailscale-User-Name:  display name (e.g. "Sergio")
//
// If no headers are present, returns nil.
func UserFromHeaders(email, name string) *User {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}
	return &User{
		Email:     email,
		Name:      strings.TrimSpace(name),
		Role:      RoleAdmin, // V1: all tailscale users are admins
		Tailscale: true,
	}
}

// CanSpawn returns true if the user can spawn agents.
func (u *User) CanSpawn() bool {
	if u == nil {
		return false
	}
	return u.Role == RoleAdmin || u.Role == RoleOperator
}

// CanKill returns true if the user can kill agents.
func (u *User) CanKill() bool {
	if u == nil {
		return false
	}
	return u.Role == RoleAdmin || u.Role == RoleOperator
}

// CanRead returns true if the user can read agents/tasks/events.
func (u *User) CanRead() bool {
	if u == nil {
		return false
	}
	return true // all authenticated users can read
}

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
