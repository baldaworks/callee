package runtime

// TokenUsage is the provider-reported token usage for one completed turn.
type TokenUsage struct {
	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
	CachedReadTokens int64
}

// TurnResult is the visible output and optional token usage for one provider turn.
type TurnResult struct {
	Content string
	Usage   *TokenUsage
}

// TokenUsageStatus describes how completely a provider reported usage.
type TokenUsageStatus string

const (
	// TokenUsageComplete means every attempted turn reported usage.
	TokenUsageComplete TokenUsageStatus = "complete"
	// TokenUsagePartial means only some attempted turns reported usage.
	TokenUsagePartial TokenUsageStatus = "partial"
	// TokenUsageUnavailable means no attempted turn reported usage.
	TokenUsageUnavailable TokenUsageStatus = "unavailable"
)

// UsageMetrics aggregates provider-reported usage across attempted turns.
type UsageMetrics struct {
	TokenUsage

	TurnsAttempted int
	TurnsReported  int
}

// AddTurn records one attempted turn and its optional provider usage.
func (m *UsageMetrics) AddTurn(usage *TokenUsage) {
	m.TurnsAttempted++

	if usage == nil {
		return
	}

	m.TurnsReported++
	m.InputTokens += usage.InputTokens
	m.OutputTokens += usage.OutputTokens
	m.TotalTokens += usage.TotalTokens
	m.CachedReadTokens += usage.CachedReadTokens
}

// Add merges another usage aggregate into this one.
func (m *UsageMetrics) Add(other UsageMetrics) {
	m.InputTokens += other.InputTokens
	m.OutputTokens += other.OutputTokens
	m.TotalTokens += other.TotalTokens
	m.CachedReadTokens += other.CachedReadTokens
	m.TurnsAttempted += other.TurnsAttempted
	m.TurnsReported += other.TurnsReported
}

// Status reports whether usage is complete, partial, or unavailable.
func (m UsageMetrics) Status() TokenUsageStatus {
	switch {
	case m.TurnsAttempted > 0 && m.TurnsReported == m.TurnsAttempted:
		return TokenUsageComplete
	case m.TurnsReported > 0:
		return TokenUsagePartial
	default:
		return TokenUsageUnavailable
	}
}
