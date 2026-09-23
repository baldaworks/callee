package workflow

import (
	"strings"
	"sync"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
)

// RunMetrics accumulates provider usage for one resolved agent run.
type RunMetrics struct {
	mu    sync.Mutex
	usage runtime.UsageMetrics
}

// Usage returns a snapshot of the run's provider-reported token usage.
func (m *RunMetrics) Usage() runtime.UsageMetrics {
	if m == nil {
		return runtime.UsageMetrics{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.usage
}

func (m *RunMetrics) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.usage = runtime.UsageMetrics{}
}

func (m *RunMetrics) add(usage runtime.UsageMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.usage.Add(usage)
}

type roleMetrics struct {
	usage            runtime.UsageMetrics
	duration         time.Duration
	wait             time.Duration
	turnStarted      bool
	resolved         bool
	redactSelections bool
	provider         string
	model            string
	reasoning        string
}

func newRoleMetrics(provider *agent.Provider) roleMetrics {
	if provider == nil {
		return roleMetrics{}
	}

	return roleMetrics{
		resolved:  true,
		provider:  strings.TrimSpace(provider.Type),
		model:     strings.TrimSpace(provider.Model),
		reasoning: strings.TrimSpace(provider.Reasoning),
	}
}

func (m *roleMetrics) applySessionConfiguration(session runtime.AgentSession) {
	reporter, ok := session.(interface {
		Configuration() runtime.SessionConfiguration
	})
	if !ok {
		return
	}

	configuration := reporter.Configuration()
	m.model = strings.TrimSpace(configuration.Model)
	m.reasoning = strings.TrimSpace(configuration.Reasoning)
}

func roleConfigurationValue(value string) string {
	if value == "" {
		return "backend-default"
	}

	return value
}
