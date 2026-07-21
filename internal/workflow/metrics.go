package workflow

import (
	"time"

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
}
