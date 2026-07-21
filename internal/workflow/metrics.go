package workflow

import (
	"strings"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
)

// RunMetrics accumulates provider usage for one resolved agent run.
type RunMetrics struct {
	usage runtime.UsageMetrics
}

// Usage returns a snapshot of the run's provider-reported token usage.
func (m *RunMetrics) Usage() runtime.UsageMetrics {
	if m == nil {
		return runtime.UsageMetrics{}
	}

	return m.usage
}

func (m *RunMetrics) reset() {
	m.usage = runtime.UsageMetrics{}
}

func (m *RunMetrics) add(usage runtime.UsageMetrics) {
	m.usage.Add(usage)
}

type roleMetrics struct {
	usage       runtime.UsageMetrics
	duration    time.Duration
	wait        time.Duration
	turnStarted bool
	provider    string
	model       string
	reasoning   string
}

func newRoleMetrics(provider *agent.Provider) roleMetrics {
	if provider == nil {
		return roleMetrics{}
	}

	return roleMetrics{
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
