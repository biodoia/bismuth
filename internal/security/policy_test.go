// Package security — smoke test for the policy.
package security

import (
	"testing"

	"github.com/biodoia/bismuth/internal/config"
)

func TestPolicyAllowList(t *testing.T) {
	cfg := config.SecurityCfg{
		AllowedCommands:       []string{"ls", "cat", "git"},
		DeniedCommands:        []string{"rm", "sudo"},
		CostCeilingPerTaskUSD: 2.0,
		WorktreeRequired:      true,
		HumanApprovalForPush:  true,
	}
	p := New(cfg)
	if !p.AllowsCommand("ls -la") {
		t.Error("ls should be allowed")
	}
	if !p.AllowsCommand("cat foo") {
		t.Error("cat should be allowed")
	}
	if p.AllowsCommand("rm -rf /") {
		t.Error("rm must be denied")
	}
	if p.AllowsCommand("sudo something") {
		t.Error("sudo must be denied")
	}
	if p.AllowsCommand("unknown_command") {
		t.Error("unknown must be denied by default")
	}
}

func TestRequiresHumanApproval(t *testing.T) {
	p := New(config.SecurityCfg{HumanApprovalForPush: true})
	if !p.RequiresHumanApproval("git_push") {
		t.Error("git_push should require approval when configured")
	}
	if p.RequiresHumanApproval("git_commit") {
		t.Error("git_commit should NOT require approval by default")
	}
}
