// Package costguard enforces per-task cost ceilings.
//
// Before every LLM call, the guard checks whether the task's
// cumulative cost (tokens_in * price_in + tokens_out * price_out)
// exceeds the task's cost_ceil_usd. If so, it marks the agent
// as "paused_cost" and prevents further dispatch.
package costguard

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/biodoia/bismuth/internal/db"
)

// Guard checks cost ceilings before allowing dispatch.
type Guard struct {
	store *db.Store
	mu    sync.RWMutex
	// rates per model: model -> {in, out} price per 1M tokens
	rates map[modelKey]modelRate
}

type modelKey struct {
	provider string
	model    string
}

type modelRate struct {
	inPer1M  float64 // USD per 1M input tokens
	outPer1M float64 // USD per 1M output tokens
}

// DefaultRates are conservative estimates for common models.
var DefaultRates = map[modelKey]modelRate{
	{provider: "openai", model: "gpt-5.5"}:    {inPer1M: 15, outPer1M: 60},
	{provider: "anthropic", model: "claude-opus-4"}: {inPer1M: 15, outPer1M: 75},
	{provider: "anthropic", model: "claude-sonnet-4"}: {inPer1M: 3, outPer1M: 15},
	{provider: "google", model: "gemini-3.5-flash"}: {inPer1M: 0.15, outPer1M: 0.60},
}

// NewGuard creates a guard with default pricing.
func NewGuard(store *db.Store) *Guard {
	return &Guard{
		store: store,
		rates: DefaultRates,
	}
}

// Check returns nil if the agent can proceed, or an error if
// the cost ceiling has been exceeded.
func (g *Guard) Check(ctx context.Context, agentID string) error {
	agent, err := g.store.GetAgent(ctx, agentID)
	if err != nil {
		return fmt.Errorf("costguard: agent not found: %w", err)
	}
	if !agent.TaskID.Valid {
		return nil // no task → no ceiling
	}
	task, err := g.store.GetTask(ctx, agent.TaskID.String)
	if err != nil {
		return nil // no task → allow
	}
	if task.CostCeilUSD <= 0 {
		return nil // no ceiling set → allow
	}
	if task.CostUsedUSD >= task.CostCeilUSD {
		// Over ceiling — pause agent
		_ = g.store.UpdateAgentState(ctx, agentID, "paused_cost")
		return fmt.Errorf("costguard: task %s cost $%.2f exceeds ceiling $%.2f",
			task.ID, task.CostUsedUSD, task.CostCeilUSD)
	}
	return nil
}

// RecordUsage updates the task's cost after an LLM call.
func (g *Guard) RecordUsage(ctx context.Context, agentID string, tokensIn, tokensOut int64) error {
	agent, err := g.store.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if !agent.TaskID.Valid {
		return nil
	}

	// Find rate
	rate := modelRate{inPer1M: 3, outPer1M: 15} // default sonnet pricing
	if agent.Model.Valid {
		rate = g.rates[modelKey{model: agent.Model.String}]
	}

	cost := (float64(tokensIn)/1_000_000)*rate.inPer1M +
		(float64(tokensOut)/1_000_000)*rate.outPer1M

	task, err := g.store.GetTask(ctx, agent.TaskID.String)
	if err != nil {
		return err
	}
	task.CostUsedUSD += cost
	// Update via raw SQL since we only need one field
	_, err = g.store.DB().ExecContext(ctx,
		`UPDATE tasks SET cost_used_usd = ?, updated_at = ? WHERE id = ?`,
		task.CostUsedUSD, time.Now().UTC().Format(time.RFC3339), task.ID)
	return err
}

// SetRate overrides the pricing for a specific model.
func (g *Guard) SetRate(provider, model string, inPer1M, outPer1M float64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rates[modelKey{provider: provider, model: model}] = modelRate{
		inPer1M:  inPer1M,
		outPer1M: outPer1M,
	}
}
