package jev

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/baldaworks/callee/internal/agent"
)

const typesafeEndpoint = "https://api.typesafe.ai/v1/systemone"

const defaultAPITimeout = 30 * time.Second

type typesafeAdapter struct{ core httpCore }

type wireRequest struct {
	Model     string                       `json:"model"`
	State     any                          `json:"state"`
	Questions map[string]agent.JevQuestion `json:"questions"`
}

type typesafeResponse struct {
	Model   string
	Answers map[string]Answer
	Usage   *Usage
}

func (a typesafeAdapter) evaluate(ctx context.Context, api agent.JevAPI, request Request) (Result, Trace, error) {
	trace := Trace{API: "typesafe", RequestedModel: request.Model}

	ctx, cancel := context.WithTimeout(ctx, apiTimeout(api))
	defer cancel()

	var response typesafeResponse

	attempts, err := a.core.postJSON(ctx, typesafeEndpoint, "TYPESAFE_API_KEY", wireRequest{
		Model: request.Model, State: request.State, Questions: request.Questions,
	}, func(data []byte) error {
		decoded, decodeErr := decodeTypesafeResponse(data)
		if decodeErr == nil {
			response = decoded
		}

		return decodeErr
	})

	trace.Attempts = attempts
	if err != nil {
		return Result{}, trace, err
	}

	if !strings.HasPrefix(response.Model, "jev-") {
		return Result{}, trace, responseError("decode response", "actual model is not a TypeSafe Jev model")
	}

	result := Result{
		API:            "typesafe",
		RequestedModel: request.Model,
		Model:          response.Model,
		Answers:        response.Answers,
		Usage:          response.Usage,
	}

	return result, trace, nil
}

func decodeTypesafeResponse(data []byte) (typesafeResponse, error) {
	var envelope struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
		Usage   struct {
			InputTokens  *int64 `json:"input_tokens"`
			OutputTokens *int64 `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := strictJSON(data, &envelope); err != nil {
		return typesafeResponse{}, err
	}

	answers := make(map[string]Answer, len(envelope.Answers))
	for id, raw := range envelope.Answers {
		answer, err := decodeAnswer(raw)
		if err != nil {
			return typesafeResponse{}, fmt.Errorf("answer %q is invalid: %w", id, err)
		}

		answers[id] = answer
	}

	var usage *Usage
	if envelope.Usage.InputTokens != nil || envelope.Usage.OutputTokens != nil {
		usage = &Usage{}
		if envelope.Usage.InputTokens != nil {
			usage.InputTokens = *envelope.Usage.InputTokens
		}

		if envelope.Usage.OutputTokens != nil {
			usage.OutputTokens = *envelope.Usage.OutputTokens
		}
	}

	return typesafeResponse{Model: envelope.Model, Answers: answers, Usage: usage}, nil
}

func decodeAnswer(data []byte) (Answer, error) {
	var discriminator struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &discriminator); err != nil {
		return Answer{}, fmt.Errorf("decode discriminator: %w", err)
	}

	switch discriminator.Type {
	case "noul":
		var value struct {
			Type string   `json:"type"`
			Noul *float64 `json:"noul"`
		}
		if err := strictJSON(data, &value); err != nil {
			return Answer{}, err
		}

		return Answer{Type: value.Type, Noul: value.Noul}, nil
	case "choice":
		var value struct {
			Type          string             `json:"type"`
			Choice        string             `json:"choice"`
			Confidence    *float64           `json:"confidence"`
			Probabilities map[string]float64 `json:"probabilities"`
		}
		if err := strictJSON(data, &value); err != nil {
			return Answer{}, err
		}

		return Answer{Type: value.Type, Choice: value.Choice, Confidence: value.Confidence, Probabilities: value.Probabilities}, nil
	case "score":
		var value struct {
			Type          string             `json:"type"`
			Score         *float64           `json:"score"`
			Confidence    *float64           `json:"confidence"`
			Legend        map[string]any     `json:"legend"`
			Probabilities map[string]float64 `json:"probabilities"`
		}
		if err := strictJSON(data, &value); err != nil {
			return Answer{}, err
		}

		return Answer{Type: value.Type, Score: value.Score, Confidence: value.Confidence, Legend: value.Legend, Probabilities: value.Probabilities}, nil
	default:
		return Answer{}, fmt.Errorf("unsupported type %q", discriminator.Type)
	}
}

func strictJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()

	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("response contains trailing JSON values")
		}

		return fmt.Errorf("decode trailing response data: %w", err)
	}

	return nil
}

func apiTimeout(api agent.JevAPI) time.Duration {
	if api.Timeout != "" {
		if timeout, err := time.ParseDuration(api.Timeout); err == nil && timeout > 0 {
			return timeout
		}
	}

	return defaultAPITimeout
}
