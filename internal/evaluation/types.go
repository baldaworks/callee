// Package evaluation implements typed remote evaluations independently of workflow and ACP.
package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/baldaworks/callee/internal/agent"
)

// Config selects a service adapter and its resource-level request settings.
type Config struct {
	Kind    agent.Kind
	Model   string
	Timeout time.Duration
}

// Request is one fully rendered typed evaluation request.
type Request struct {
	Model     string
	State     any
	Questions map[string]agent.EvaluationQuestion
}

// Answer is the canonical union returned for one question.
type Answer struct {
	Type          string             `json:"type"`
	Noul          *float64           `json:"noul,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Score         *float64           `json:"score,omitempty"`
	Legend        map[string]any     `json:"legend,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Usage is service-reported usage for a successful response.
type Usage struct {
	InputTokens  int64    `json:"inputTokens"`
	OutputTokens int64    `json:"outputTokens"`
	Cost         *float64 `json:"cost,omitempty"`
}

// Result is the service-neutral value published to workflow state.
type Result struct {
	Service        string            `json:"service"`
	Provider       string            `json:"provider,omitempty"`
	RequestID      string            `json:"requestId,omitempty"`
	RequestedModel string            `json:"requestedModel"`
	Model          string            `json:"model"`
	Answers        map[string]Answer `json:"answers"`
	Usage          *Usage            `json:"usage,omitempty"`
}

// ErrorClass is a bounded operational failure category.
type ErrorClass string

const (
	ErrorConfiguration  ErrorClass = "configuration"
	ErrorAuthentication ErrorClass = "authentication"
	ErrorRateLimit      ErrorClass = "rate_limit"
	ErrorTransport      ErrorClass = "transport"
	ErrorTimeout        ErrorClass = "timeout"
	ErrorProvider       ErrorClass = "provider"
	ErrorResponse       ErrorClass = "response"
	ErrorCanceled       ErrorClass = "canceled"
)

// Trace contains safe fields suitable for lifecycle logging.
type Trace struct {
	Service        string
	Provider       string
	RequestID      string
	RequestedModel string
	Model          string
	Attempts       int
	Usage          *Usage
	ErrorClass     ErrorClass
}

// Evaluator executes one rendered request through the service selected by kind.
type Evaluator interface {
	Evaluate(ctx context.Context, config Config, request Request) (Result, Trace, error)
}

// Error is a classified failure that excludes response bodies and secrets.
type Error struct {
	Class ErrorClass
	Op    string
	Code  int
	Err   error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	message := fmt.Sprintf("evaluation %s failed (%s)", e.Op, e.Class)
	if e.Code != 0 {
		message += fmt.Sprintf(" with HTTP status %d", e.Code)
	}

	if e.Err != nil {
		message += ": " + e.Err.Error()
	}

	return message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

// MarshalResult returns deterministic compact JSON for result.
func MarshalResult(result Result) ([]byte, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal evaluation result: %w", err)
	}

	return data, nil
}
