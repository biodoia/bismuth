// db package: smoke test for Open/Close/migrations.
package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.db")
	s, err := Open(context.Background(), p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()
	if s.DB() == nil {
		t.Fatal("nil DB handle")
	}
	// schema_version should be at 1
	var v int
	if err := s.DB().QueryRow("SELECT MAX(version) FROM schema_version").Scan(&v); err != nil {
		t.Fatalf("query: %v", err)
	}
	if v < 1 {
		t.Fatalf("schema not applied, got version %d", v)
	}
}

func TestAgentCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	a := &Agent{
		ID:   "agt-test-1",
		Role: "implementer",
		Name: "test",
		CLI:  "claude",
	}
	if err := s.InsertAgent(ctx, a); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetAgent(ctx, "agt-test-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Role != "implementer" || got.Name != "test" {
		t.Errorf("mismatch: %+v", got)
	}

	if err := s.UpdateAgentState(ctx, "agt-test-1", "working"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAgent(ctx, "agt-test-1")
	if got.State != "working" {
		t.Errorf("state not updated: %s", got.State)
	}

	if err := s.AddAgentCost(ctx, "agt-test-1", 0.42, 100, 200); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetAgent(ctx, "agt-test-1")
	if got.CostUSD != 0.42 || got.TokensIn != 100 || got.TokensOut != 200 {
		t.Errorf("cost not updated: %+v", got)
	}

	if err := s.DeleteAgent(ctx, "agt-test-1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.GetAgent(ctx, "agt-test-1")
	if err == nil {
		t.Error("expected error after delete")
	}
}

func TestTaskCRUD(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	task := &Task{
		ID:    "tsk-1",
		Title: "implementa auth JWT",
	}
	if err := s.InsertTask(ctx, task); err != nil {
		t.Fatal(err)
	}
	if err := s.AssignTask(ctx, "tsk-1", "agt-1"); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetTask(ctx, "tsk-1")
	if got.Status != "assigned" || !got.AssigneeAgent.Valid || got.AssigneeAgent.String != "agt-1" {
		t.Errorf("assign failed: %+v", got)
	}
	if err := s.MarkTaskStarted(ctx, "tsk-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTaskCost(ctx, "tsk-1", 0.5); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkTaskFinished(ctx, "tsk-1", "done"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTask(ctx, "tsk-1")
	if got.Status != "done" || got.CostUsedUSD != 0.5 {
		t.Errorf("finish failed: %+v", got)
	}
}

func TestMessageMailbox(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	m1 := &Message{ID: "m-1", FromAgent: "a", ToAgent: "b", Body: "ciao"}
	if err := s.PostMessage(ctx, m1); err != nil {
		t.Fatal(err)
	}
	in, err := s.ReadInbox(ctx, "b", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(in) != 1 || in[0].Body != "ciao" {
		t.Errorf("inbox wrong: %+v", in)
	}
	if err := s.MarkMessageRead(ctx, "m-1"); err != nil {
		t.Fatal(err)
	}
}

func TestEvents(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	_, err = s.DB().ExecContext(ctx,
		`INSERT INTO events(seq, type, agent_id, payload, ts)
		 VALUES(1, 'agent_spawned', 'a-1', '{"role":"implementer"}', '2026-06-08T00:00:00Z')`)
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.RecentEvents(ctx, []string{"agent_spawned"}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestSettings(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()

	if err := s.SetSetting(ctx, "k1", "v1"); err != nil {
		t.Fatal(err)
	}
	v, ok, err := s.GetSetting(ctx, "k1")
	if err != nil || !ok || v != "v1" {
		t.Errorf("got %q %v %v", v, ok, err)
	}
	if _, ok, _ := s.GetSetting(ctx, "missing"); ok {
		t.Error("missing key should not be present")
	}
}

// silence unused warnings if file is compiled without tests above
var _ = os.Getenv
