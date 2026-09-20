package jev

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
)

func TestTypeSafeAdapterUsesFixedSystemOneContract(t *testing.T) {
	t.Parallel()

	request := validRequest()
	request.Model = "jev-latest"
	calls := 0
	config := httpConfig{
		client: &http.Client{Transport: roundTripFunc(func(httpRequest *http.Request) (*http.Response, error) {
			calls++

			if httpRequest.Method != http.MethodPost || httpRequest.URL.String() != typesafeEndpoint {
				t.Fatalf("request = %s %s", httpRequest.Method, httpRequest.URL)
			}

			if got := httpRequest.Header.Get("Authorization"); got != "Bearer typesafe-secret" {
				t.Fatalf("Authorization = %q", got)
			}

			data, err := io.ReadAll(httpRequest.Body)
			if err != nil {
				t.Fatalf("ReadAll(request) error = %v", err)
			}

			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("request JSON error = %v", err)
			}

			if len(payload) != 3 || payload["model"] != "jev-latest" || payload["state"] == nil || payload["questions"] == nil {
				t.Fatalf("request payload = %#v", payload)
			}

			return response(http.StatusOK, validTypesafeResponse()), nil
		})},
		getenv: func(name string) string {
			if name != "TYPESAFE_API_KEY" {
				t.Fatalf("credential env = %q", name)
			}

			return "typesafe-secret"
		},
		wait:   func(context.Context, time.Duration) error { return nil },
		jitter: func() float64 { return 0 },
	}
	evaluator := dispatcher{adapters: map[string]adapter{"typesafe": typesafeAdapter{core: httpCore{config: config}}}}

	result, trace, err := evaluator.Evaluate(context.Background(), agent.JevAPI{Type: "typesafe", Timeout: "2s"}, request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if calls != 1 || result.RequestedModel != "jev-latest" || result.Model != "jev-1.13.0" || result.Usage == nil || result.Usage.InputTokens != 42 || trace.Attempts != 1 {
		t.Fatalf("Evaluate() = (result %#v, trace %#v, calls %d)", result, trace, calls)
	}
}

func TestTypeSafeAdapterRejectsUnknownResponseFieldWithoutRetry(t *testing.T) {
	t.Parallel()

	calls := 0
	config := httpConfig{
		client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++

			return response(http.StatusOK, `{"model":"jev-1.13.0","answers":{},"usage":{},"secret_body":"must-not-appear"}`), nil
		})},
		getenv: func(string) string { return "typesafe-secret" },
		wait: func(context.Context, time.Duration) error {
			t.Fatal("decode errors must not retry")

			return nil
		},
		jitter: func() float64 { return 0 },
	}
	_, _, err := (typesafeAdapter{core: httpCore{config: config}}).evaluate(context.Background(), agent.JevAPI{Type: "typesafe"}, validRequest())

	var classified *Error
	if !errors.As(err, &classified) || classified.Class != ErrorResponse || calls != 1 {
		t.Fatalf("evaluate() = (error %v, calls %d)", err, calls)
	}

	if strings.Contains(err.Error(), "must-not-appear") {
		t.Fatalf("error leaks response value: %v", err)
	}
}

func validTypesafeResponse() string {
	return `{
  "model": "jev-1.13.0",
  "answers": {
    "urgent": {"type":"noul","noul":0.82},
    "team": {"type":"choice","choice":"billing","confidence":0.91,"probabilities":{"billing":0.8,"technical":0.2}},
    "risk": {"type":"score","score":1.01,"confidence":0.75,"legend":{"0":"Low","1":"Medium","2":"High"},"probabilities":{"0":0.33,"1":0.33,"2":0.34}}
  },
  "usage": {"input_tokens":42,"output_tokens":0}
}`
}
