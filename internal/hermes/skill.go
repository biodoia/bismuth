// Package hermes contains the generator for the bismuth-control skill
// manifest installed into ~/.claude/skills/.
//
// The skill is a thin YAML wrapper that points Hermes at a `bismuth`
// CLI (this same binary, subcommand `cli`) which talks REST to the
// running bismuth server.
package hermes

import (
	"fmt"
	"os"
	"path/filepath"
)

const skillYAML = `---
name: bismuth-control
description: |
  Control a running bismuth multi-agent team: spawn workers by role,
  send prompts, read pane output, assign tasks, monitor state, merge PRs.
  Use when the user says "delegate to the team", "spawn an implementer",
  "what are the agents doing", "send to the reviewer", "merge task X", etc.
level: 3
---

# bismuth-control

bismuth is a local multi-agent coding team multiplexer. This skill
lets Hermes (the lead agent) act as orchestrator.

All commands go through the local bismuth CLI which talks to the server
over HTTP at $BISMUTH_URL (default http://127.0.0.1:9000).

## Commands

- ` + "`bismuth cli list-agents`" + `             # who is running, what state
- ` + "`bismuth cli list-tasks`" + `              # the bacheca
- ` + "`bismuth cli spawn --role implementer --cli omx --task \"implement auth\"`" + `
- ` + "`bismuth cli send --agent omx-1 --data \"implementa la funzione foo\"`" + `
- ` + "`bismuth cli read --agent omx-1 --n 200`" + `  # last 200 lines of pane
- ` + "`bismuth cli assign --task T-123 --agent omx-1`" + `
- ` + "`bismuth cli kill --agent omx-1`" + `
- ` + "`bismuth cli merge --task T-123`" + `        # opens PR (human approves)
- ` + "`bismuth cli status`" + `                   # global team status

## Conventions

- 8-12 specialized roles. Pick the SMALLEST role that fits.
- ALWAYS include a verifier pass before declaring done.
- For "complex" tasks, spawn: planner → implementer → reviewer → tester
  → verifier, in that order.
- Use ` + "`task --description`" + ` to give the worker full context.
- Read pane output with ` + "`read`" + ` before sending follow-up prompts.

## Cost

Every spawn has a cost ceiling (default $2/task). If the worker hits
it, bismuth kills it and notifies. Use opus-4-7 only for planner/
architect/security/critic, sonnet-4-6 / gpt-5.5 for implementer.
`

// Install writes the SKILL.md into dest. dest is the .claude/skills
// root (e.g. ~/.claude/skills).
func Install(dest string) error {
	dir := filepath.Join(dest, "bismuth-control")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(skillYAML), 0o644)
}

// SkillYAML returns the SKILL.md content (for embedding/tests).
func SkillYAML() string { return skillYAML }

// PrintSkill writes the skill to stdout (for `bismuth cli skill-install`).
func PrintSkill() { fmt.Print(skillYAML) }
