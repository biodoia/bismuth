// Package worktree manages git worktrees for task isolation.
//
// V1 rules:
//   - Every task gets its own branch and worktree at <repo>/.bismuth/<task-id>
//   - The multiplexer spawns worker panes inside that worktree
//   - PR is opened on task finish, human approves before merge
//   - The lead agent (Hermes) is the only writer to main
package worktree

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Manager creates / removes / inspects worktrees.
type Manager struct {
	RepoRoot string
}

// New returns a manager rooted at the given git repository.
func New(repoRoot string) *Manager {
	return &Manager{RepoRoot: repoRoot}
}

// Create makes a new branch + worktree for the task.
func (m *Manager) Create(ctx context.Context, taskID, baseBranch string) (worktreePath, branch string, err error) {
	branch = "bismuth/" + taskID
	worktreePath = m.RepoRoot + "/.bismuth/" + taskID

	if err := m.run(ctx, "worktree", "add", "-b", branch, worktreePath, baseBranch); err != nil {
		return "", "", fmt.Errorf("worktree add: %w", err)
	}
	return worktreePath, branch, nil
}

// Remove deletes the worktree and branch.
func (m *Manager) Remove(ctx context.Context, taskID string) error {
	wt := m.RepoRoot + "/.bismuth/" + taskID
	branch := "bismuth/" + taskID
	if err := m.run(ctx, "worktree", "remove", "--force", wt); err != nil {
		return err
	}
	return m.run(ctx, "branch", "-D", branch)
}

// Commit commits staged changes with the given message.
func (m *Manager) Commit(ctx context.Context, worktreePath, message string) error {
	return m.runIn(ctx, worktreePath, "commit", "-am", message)
}

// Push pushes the branch to origin.
func (m *Manager) Push(ctx context.Context, worktreePath, branch string) error {
	return m.runIn(ctx, worktreePath, "push", "-u", "origin", branch)
}

func (m *Manager) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = m.RepoRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}

func (m *Manager) runIn(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), string(out), err)
	}
	return nil
}
