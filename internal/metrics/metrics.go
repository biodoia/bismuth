// Package metrics provides Prometheus instrumentation for bismuth.
//
// Exposes counters, gauges, and histograms for:
//   - Agent lifecycle (spawned, killed, active)
//   - Task lifecycle (created, claimed, finished, failed)
//   - LLM usage (tokens, cost, latency)
//   - API request rates and latencies
//   - MCP tool call rates
//   - Event bus throughput
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const namespace = "bismuth"

var (
	// --- Agents ---

	AgentsSpawned = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "agents_spawned_total",
		Help:      "Total number of agents spawned",
	}, []string{"role", "cli"})

	AgentsKilled = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "agents_killed_total",
		Help:      "Total number of agents killed",
	}, []string{"role"})

	AgentsActive = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "agents_active",
		Help:      "Currently active agents",
	}, []string{"role", "state"})

	// --- Tasks ---

	TasksCreated = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tasks_created_total",
		Help:      "Total number of tasks created",
	})

	TasksClaimed = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tasks_claimed_total",
		Help:      "Total number of tasks claimed",
	}, []string{"agent_id"})

	TasksFinished = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tasks_finished_total",
		Help:      "Total number of tasks finished",
	}, []string{"status"})

	// --- LLM ---

	LLMCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "llm_calls_total",
		Help:      "Total LLM API calls",
	}, []string{"model", "provider"})

	LLMTokensIn = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "llm_tokens_in_total",
		Help:      "Total input tokens consumed",
	}, []string{"model"})

	LLMTokensOut = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "llm_tokens_out_total",
		Help:      "Total output tokens generated",
	}, []string{"model"})

	LLMCostUSD = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "llm_cost_usd_total",
		Help:      "Total LLM cost in USD",
	}, []string{"model"})

	LLMLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "llm_latency_seconds",
		Help:      "LLM API call latency",
		Buckets:   []float64{.1, .25, .5, 1, 2.5, 5, 10, 30, 60},
	}, []string{"model"})

	// --- API ---

	APIRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "api_requests_total",
		Help:      "Total HTTP API requests",
	}, []string{"method", "path", "status"})

	APILatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "api_latency_seconds",
		Help:      "HTTP API request latency",
		Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1},
	}, []string{"method", "path"})

	// --- MCP ---

	MCPToolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "mcp_tool_calls_total",
		Help:      "Total MCP tool invocations",
	}, []string{"tool"})

	MCPToolLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "mcp_tool_latency_seconds",
		Help:      "MCP tool call latency",
		Buckets:   []float64{.001, .005, .01, .05, .1, .5, 1},
	}, []string{"tool"})

	// --- Events ---

	EventsPublished = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_published_total",
		Help:      "Total events published to the bus",
	}, []string{"type"})

	EventsDelivered = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "events_delivered_total",
		Help:      "Total events delivered to subscribers",
	})

	// --- Voice ---

	VoiceSTTCalls = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "voice_stt_calls_total",
		Help:      "Total speech-to-text calls",
	})

	VoiceTTSCalls = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "voice_tts_calls_total",
		Help:      "Total text-to-speech calls",
	})

	VoiceSTTLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "voice_stt_latency_seconds",
		Help:      "STT processing latency",
		Buckets:   []float64{.1, .25, .5, 1, 2.5, 5},
	})

	VoiceTTSLatency = promauto.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "voice_tts_latency_seconds",
		Help:      "TTS processing latency",
		Buckets:   []float64{.1, .25, .5, 1, 2.5, 5},
	})

	// --- DB ---

	DBQueries = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "db_queries_total",
		Help:      "Total database queries",
	}, []string{"operation"})

	DBLatency = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "db_latency_seconds",
		Help:      "Database query latency",
		Buckets:   []float64{.0001, .0005, .001, .005, .01, .05, .1},
	}, []string{"operation"})
)

// IncAgentSpawned increments the spawn counter.
func IncAgentSpawned(role, cli string) {
	AgentsSpawned.WithLabelValues(role, cli).Inc()
}

// IncAgentKilled increments the kill counter.
func IncAgentKilled(role string) {
	AgentsKilled.WithLabelValues(role).Inc()
}

// SetActiveAgents updates the active agent gauge.
func SetActiveAgents(role, state string, count float64) {
	AgentsActive.WithLabelValues(role, state).Set(count)
}

// RecordLLMCall records an LLM call with tokens and cost.
func RecordLLMCall(model, provider string, tokensIn, tokensOut int64, costUSD float64, latencySeconds float64) {
	LLMCalls.WithLabelValues(model, provider).Inc()
	LLMTokensIn.WithLabelValues(model).Add(float64(tokensIn))
	LLMTokensOut.WithLabelValues(model).Add(float64(tokensOut))
	LLMCostUSD.WithLabelValues(model).Add(costUSD)
	LLMLatency.WithLabelValues(model).Observe(latencySeconds)
}

// IncMCPTool records an MCP tool invocation.
func IncMCPTool(tool string, latencySeconds float64) {
	MCPToolCalls.WithLabelValues(tool).Inc()
	MCPToolLatency.WithLabelValues(tool).Observe(latencySeconds)
}

// IncEvent records an event publication.
func IncEvent(eventType string) {
	EventsPublished.WithLabelValues(eventType).Inc()
}
