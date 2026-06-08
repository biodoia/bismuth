// Package worktree — smoke test for the worktree manager.
// Creates a temp git repo, spawns a worktree, verifies path/branch.
package worktree

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func initRepo(t *testing.T, dir string) {
	for _, args := range [][]string{
		{"init", "-b", "main", dir},
		{"-C", dir, "config", "user.email", "test@bismuth"},
		{"-C", dir, "config", "user.name", "bismuth-test"},
		{"-C", dir, "commit", "--allow-empty", "-m", "init"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func TestCreate(t *testing.T) {
	root := t.TempDir()
	initRepo(t, root)

	m := New(root)
	wtPath, branch, err := m.Create(context.Background(), "test-task-1", "main")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer func() {
		_ = exec.Command("git", "-C", wtPath, "checkout", "main").Run()
		_ = exec.Command("git", "-C", root, "worktree", "remove", "--force", wtPath).Run()
		_ = exec.Command("git", "-C", root, "worktree", "prune").Run()
		_ = exec.Command("git", "-C", root, "branch", "-D", branch).Run()
	}()

	if !strings.HasPrefix(branch, "bismuth/") {
		t.Errorf("branch = %q, want prefix bismuth/", branch)
	}
	if !filepath.IsAbs(wtPath) {
		t.Errorf("path not absolute: %s", wtPath)
	}
	// verify worktree exists
	if out, err := exec.Command("git", "-C", root, "worktree", "list").CombinedOutput(); err != nil {
		t.Fatal(err)
	} else if !strings.Contains(string(out), wtPath) {
		t.Errorf("worktree not in list: %s", out)
	}
}
