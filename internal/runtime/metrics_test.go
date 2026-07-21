package runtime

import "testing"

func TestUsageMetricsStatusAndAggregation(t *testing.T) {
	t.Parallel()

	metrics := UsageMetrics{}
	if got := metrics.Status(); got != TokenUsageUnavailable {
		t.Fatalf("empty status = %q, want %q", got, TokenUsageUnavailable)
	}

	metrics.AddTurn(&TokenUsage{InputTokens: 10, OutputTokens: 2, TotalTokens: 12, CachedReadTokens: 4})

	if got := metrics.Status(); got != TokenUsageComplete {
		t.Fatalf("reported status = %q, want %q", got, TokenUsageComplete)
	}

	metrics.AddTurn(nil)

	if got := metrics.Status(); got != TokenUsagePartial {
		t.Fatalf("partial status = %q, want %q", got, TokenUsagePartial)
	}

	other := UsageMetrics{}
	other.AddTurn(&TokenUsage{InputTokens: 7, OutputTokens: 3, TotalTokens: 10, CachedReadTokens: 1})
	metrics.Add(other)

	if metrics.InputTokens != 17 || metrics.OutputTokens != 5 || metrics.TotalTokens != 22 || metrics.CachedReadTokens != 5 {
		t.Errorf("aggregated tokens = %+v", metrics.TokenUsage)
	}

	if metrics.TurnsAttempted != 3 || metrics.TurnsReported != 2 {
		t.Errorf("aggregated turns = %d/%d, want 2/3 reported", metrics.TurnsReported, metrics.TurnsAttempted)
	}
}
