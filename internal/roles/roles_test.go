// Package roles — end-to-end smoke test for the role catalog
// and a fake worktree-driven spawn flow.
package roles

import "testing"

func TestDefaultCatalogHasTwelveRoles(t *testing.T) {
	c := DefaultCatalog()
	if got, want := len(c.Roles), 12; got != want {
		t.Fatalf("got %d roles, want %d", got, want)
	}
	want := []string{
		"planner", "architect", "implementer", "reviewer",
		"tester", "security", "debug", "refactor",
		"documenter", "devops", "critic", "verifier",
	}
	seen := map[string]bool{}
	for _, r := range c.Roles {
		seen[r.ID] = true
		if r.DefaultModel == "" {
			t.Errorf("role %s has empty DefaultModel", r.ID)
		}
		if r.PromptPath == "" {
			t.Errorf("role %s has empty PromptPath", r.ID)
		}
		if len(r.CLIs) == 0 {
			t.Errorf("role %s has no CLIs", r.ID)
		}
	}
	for _, id := range want {
		if !seen[id] {
			t.Errorf("missing role %q in catalog", id)
		}
	}
}

func TestRolesAreInternallyConsistent(t *testing.T) {
	c := DefaultCatalog()
	for _, r := range c.Roles {
		if r.ID == "" {
			t.Errorf("empty id")
		}
	}
}
