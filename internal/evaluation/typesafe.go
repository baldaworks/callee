package evaluation

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/baldaworks/callee/internal/agent"
)

const defaultTypeSafeBaseURL = "https://api.typesafe.ai"

const defaultAPITimeout = 30 * time.Second

type typesafeAdapter struct{ core httpCore }

type wireRequest struct {
	Model     string                              `json:"model"`
	State     any                                 `json:"state"`
	Questions map[string]agent.EvaluationQuestion `json:"questions"`
}

type typesafeResponse struct {
	Model   string
	Answers map[string]Answer
	Usage   *Usage
}

func (a typesafeAdapter) getenv(name string) string { return a.core.config.getenv(name) }

func (a typesafeAdapter) evaluate(ctx context.Context, config Config, request Request) (Result, Trace, error) {
	trace := Trace{Service: "typesafe", RequestedModel: request.Model}
	if !strings.HasPrefix(request.Model, "jev-") {
		return Result{}, trace, &Error{Class: ErrorConfiguration, Op: "validate model", Err: fmt.Errorf("requested model is not a TypeSafe Jev model")}
	}

	endpoint, err := resolveTypeSafeEndpoint(a.getenv("TYPESAFE_BASE_URL"))
	if err != nil {
		return Result{}, trace, err
	}

	ctx, cancel := context.WithTimeout(ctx, apiTimeout(config.Timeout))
	defer cancel()

	var response typesafeResponse

	attempts, err := a.core.postJSON(ctx, endpoint, "TYPESAFE_API_KEY", wireRequest{
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

	return Result{
		Service: "typesafe", RequestedModel: request.Model, Model: response.Model,
		Answers: response.Answers, Usage: response.Usage,
	}, trace, nil
}

func resolveTypeSafeEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultTypeSafeBaseURL
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", &Error{Class: ErrorConfiguration, Op: "configure TypeSafe endpoint", Err: fmt.Errorf("TYPESAFE_BASE_URL must be an absolute HTTPS API root without userinfo, query, or fragment")}
	}

	parsed.Path = path.Join("/", parsed.Path, "v1/systemone")
	parsed.RawPath = ""

	return parsed.String(), nil
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

func apiTimeout(timeout time.Duration) time.Duration {
	if timeout > 0 {
		return timeout
	}

	return defaultAPITimeout
}
