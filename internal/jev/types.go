// Package jev implements typed Jev evaluations independently of workflow and ACP.
package jev

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/baldaworks/callee/internal/agent"
)

// Request is one rendered Jev evaluation.
type Request struct {
	Model     string
	State     any
	Questions map[string]agent.JevQuestion
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

// Usage is provider-reported usage for the successful response.
type Usage struct {
	InputTokens  int64    `json:"inputTokens"`
	OutputTokens int64    `json:"outputTokens"`
	Cost         *float64 `json:"cost,omitempty"`
}

// Result is the provider-neutral value published to workflow state.
type Result struct {
	API            string            `json:"api"`
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
	API            string
	Provider       string
	RequestID      string
	RequestedModel string
	Model          string
	Attempts       int
	Usage          *Usage
	ErrorClass     ErrorClass
}

// Evaluator executes one rendered request through the selected API.
type Evaluator interface {
	Evaluate(ctx context.Context, api agent.JevAPI, request Request) (Result, Trace, error)
}

// Error is a classified failure that never contains response bodies or secrets.
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

	message := fmt.Sprintf("jev %s failed (%s)", e.Op, e.Class)
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

// MarshalResult returns the deterministic compact JSON artifact for result.
func MarshalResult(result Result) ([]byte, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal Jev result: %w", err)
	}

	return data, nil
}
