// mcp package: smoke test for tool list shape.
package mcp

import "testing"

func TestToolListShape(t *testing.T) {
	tools := toolList()
	if len(tools) != 7 {
		t.Fatalf("expected 7 tools, got %d", len(tools))
	}
	want := map[string]bool{
		"team_status":     false,
		"team_peers":      false,
		"team_post":       false,
		"team_read_inbox": false,
		"team_claim":      false,
		"team_finish":     false,
		"shared_memory":   false,
	}
	for _, tool := range tools {
		name, ok := tool["name"].(string)
		if !ok {
			t.Errorf("tool missing name: %v", tool)
			continue
		}
		if _, expected := want[name]; !expected {
			t.Errorf("unexpected tool: %s", name)
		}
		want[name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("missing tool: %s", name)
		}
	}
}
