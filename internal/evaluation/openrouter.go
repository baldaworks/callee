package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const openRouterEndpoint = "https://openrouter.ai/api/alpha/decisions"

type openRouterAdapter struct{ core httpCore }

type openRouterResponse struct {
	ID       string
	Model    string
	Provider string
	Answers  map[string]Answer
	Usage    *Usage
}

func (a openRouterAdapter) getenv(name string) string { return a.core.config.getenv(name) }

func (a openRouterAdapter) evaluate(ctx context.Context, config Config, request Request) (Result, Trace, error) {
	trace := Trace{Service: "openrouter", RequestedModel: request.Model}

	ctx, cancel := context.WithTimeout(ctx, apiTimeout(config.Timeout))
	defer cancel()

	var response openRouterResponse

	attempts, err := a.core.postJSON(ctx, openRouterEndpoint, "OPENROUTER_API_KEY", wireRequest{
		Model: request.Model, State: request.State, Questions: request.Questions,
	}, func(data []byte) error {
		decoded, decodeErr := decodeOpenRouterResponse(data)
		if decodeErr == nil {
			response = decoded
		}

		return decodeErr
	})
	trace.Attempts = attempts

	if err != nil {
		return Result{}, trace, err
	}

	if strings.TrimSpace(response.Model) == "" {
		return Result{}, trace, responseError("decode response", "actual model must not be blank")
	}

	return Result{
		Service: "openrouter", Provider: response.Provider, RequestID: response.ID,
		RequestedModel: request.Model, Model: response.Model, Answers: response.Answers, Usage: response.Usage,
	}, trace, nil
}

func decodeOpenRouterResponse(data []byte) (openRouterResponse, error) {
	var envelope struct {
		Answers  map[string]json.RawMessage `json:"answers"`
		ID       string                     `json:"id"`
		Model    string                     `json:"model"`
		Provider string                     `json:"provider"`
		Usage    struct {
			Cost         *float64 `json:"cost"`
			InputTokens  *int64   `json:"input_tokens"`
			OutputTokens *int64   `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := strictJSON(data, &envelope); err != nil {
		return openRouterResponse{}, err
	}

	if envelope.Usage.InputTokens == nil || envelope.Usage.OutputTokens == nil {
		return openRouterResponse{}, fmt.Errorf("usage input_tokens and output_tokens are required")
	}

	answers := make(map[string]Answer, len(envelope.Answers))
	for id, raw := range envelope.Answers {
		answer, err := decodeAnswer(raw)
		if err != nil {
			return openRouterResponse{}, fmt.Errorf("answer %q is invalid: %w", id, err)
		}

		answers[id] = answer
	}

	usage := &Usage{InputTokens: *envelope.Usage.InputTokens, OutputTokens: *envelope.Usage.OutputTokens, Cost: envelope.Usage.Cost}

	return openRouterResponse{ID: envelope.ID, Model: envelope.Model, Provider: envelope.Provider, Answers: answers, Usage: usage}, nil
}
