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

func TestOpenRouterAdapterUsesFixedDecisionsContract(t *testing.T) {
	t.Parallel()

	request := validRequest()
	request.Model = "~typesafe/jev-latest"
	calls := 0
	config := httpConfig{
		client: &http.Client{Transport: roundTripFunc(func(httpRequest *http.Request) (*http.Response, error) {
			calls++

			assertOpenRouterRequest(t, httpRequest)

			return response(http.StatusOK, validOpenRouterResponse()), nil
		})},
		getenv: func(name string) string {
			if name != "OPENROUTER_API_KEY" {
				t.Fatalf("credential env = %q", name)
			}

			return "openrouter-secret"
		},
		wait:   func(context.Context, time.Duration) error { return nil },
		jitter: func() float64 { return 0 },
	}
	evaluator := dispatcher{adapters: map[string]adapter{"openrouter": openRouterAdapter{core: httpCore{config: config}}}}

	result, trace, err := evaluator.Evaluate(context.Background(), agent.JevAPI{Type: "openrouter"}, request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if calls != 1 || result.Model != "typesafe/jev-1.13" || result.Provider != "TypeSafe" || result.RequestID != "gen-123" || result.Usage == nil || result.Usage.Cost == nil || *result.Usage.Cost != 0.000018 || trace.RequestID != "gen-123" {
		t.Fatalf("Evaluate() = (result %#v, trace %#v, calls %d)", result, trace, calls)
	}
}

func assertOpenRouterRequest(t *testing.T, request *http.Request) {
	t.Helper()

	if request.Method != http.MethodPost || request.URL.String() != openRouterEndpoint {
		t.Fatalf("request = %s %s", request.Method, request.URL)
	}

	if got := request.Header.Get("Authorization"); got != "Bearer openrouter-secret" {
		t.Fatalf("Authorization = %q", got)
	}

	data, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatalf("ReadAll(request) error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("request JSON error = %v", err)
	}

	if len(payload) != 3 || payload["model"] != "~typesafe/jev-latest" || payload["state"] == nil || payload["questions"] == nil {
		t.Fatalf("request payload = %#v", payload)
	}

	for _, forbidden := range []string{"messages", "stream", "response_format", "provider", "trace", "user", "session_id"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("request unexpectedly contains %q", forbidden)
		}
	}
}

func TestOpenRouterAdapterRejectsNonJevModelBeforeIO(t *testing.T) {
	t.Parallel()

	calls := 0
	config := defaultHTTPConfig()
	config.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++

		return response(http.StatusOK, validOpenRouterResponse()), nil
	})
	config.getenv = func(string) string { return "secret" }
	request := validRequest()
	request.Model = "openai/gpt-5"

	_, _, err := (openRouterAdapter{core: httpCore{config: config}}).evaluate(context.Background(), agent.JevAPI{Type: "openrouter"}, request)

	var classified *Error
	if !errors.As(err, &classified) || classified.Class != ErrorConfiguration || calls != 0 {
		t.Fatalf("evaluate() = (error %v, calls %d)", err, calls)
	}
}

func TestOpenRouterAdapterRejectsUnexpectedProvider(t *testing.T) {
	t.Parallel()

	config := defaultHTTPConfig()
	config.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return response(http.StatusOK, strings.Replace(validOpenRouterResponse(), `"provider":"TypeSafe"`, `"provider":"Other"`, 1)), nil
	})
	config.getenv = func(string) string { return "secret" }
	request := validRequest()
	request.Model = "typesafe/jev-1.13"

	_, _, err := (openRouterAdapter{core: httpCore{config: config}}).evaluate(context.Background(), agent.JevAPI{Type: "openrouter"}, request)
	if err == nil || !strings.Contains(err.Error(), "provider is not TypeSafe") {
		t.Fatalf("evaluate() error = %v", err)
	}
}

func validOpenRouterResponse() string {
	base := strings.Replace(validTypesafeResponse(), `"model": "jev-1.13.0"`, `"model": "typesafe/jev-1.13"`, 1)

	return strings.Replace(
		base,
		`"usage": {"input_tokens":42,"output_tokens":0}`,
		`"id":"gen-123","provider":"TypeSafe","usage":{"input_tokens":42,"output_tokens":0,"cost":0.000018}`,
		1,
	)
}
