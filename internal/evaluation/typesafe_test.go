package evaluation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
)

func TestTypeSafeAdapterUsesConfiguredSystemOneContract(t *testing.T) {
	t.Parallel()

	calls := 0
	config := httpConfig{
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++

			if request.Method != http.MethodPost || request.URL.String() != "https://proxy.example/typesafe/v1/systemone" {
				t.Fatalf("request = %s %s", request.Method, request.URL)
			}

			if request.Header.Get("Authorization") != "Bearer typesafe-secret" {
				t.Fatal("missing authorization")
			}

			data, _ := io.ReadAll(request.Body)

			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil || len(payload) != 3 || payload["model"] != "jev-latest" {
				t.Fatalf("request payload = %#v, error %v", payload, err)
			}

			return response(http.StatusOK, validTypesafeResponse()), nil
		})},
		getenv: func(name string) string {
			switch name {
			case "TYPESAFE_API_KEY":
				return "typesafe-secret"
			case "TYPESAFE_BASE_URL":
				return "https://proxy.example/typesafe"
			default:
				return ""
			}
		},
		wait: func(context.Context, time.Duration) error { return nil }, jitter: func() float64 { return 0 },
	}
	evaluator := dispatcher{adapters: map[agent.Kind]adapter{agent.TypeSafeJevKind: typesafeAdapter{core: httpCore{config: config}}}}

	result, trace, err := evaluator.Evaluate(context.Background(), Config{Kind: agent.TypeSafeJevKind, Timeout: 2 * time.Second}, validRequest())
	if err != nil || calls != 1 || result.Service != "typesafe" || result.RequestedModel != "jev-latest" || trace.Attempts != 1 {
		t.Fatalf("Evaluate() = (result %#v, trace %#v, error %v, calls %d)", result, trace, err, calls)
	}
}

func TestResolveTypeSafeEndpointRejectsUnsafeRoots(t *testing.T) {
	t.Parallel()

	if endpoint, err := resolveTypeSafeEndpoint(""); err != nil || endpoint != "https://api.typesafe.ai/v1/systemone" {
		t.Fatalf("resolveTypeSafeEndpoint(default) = (%q, %v)", endpoint, err)
	}

	for _, value := range []string{"http://example.test", "https://user@example.test", "https://example.test?q=x", "https://example.test/#fragment", "relative"} {
		if endpoint, err := resolveTypeSafeEndpoint(value); err == nil || endpoint != "" {
			t.Fatalf("resolveTypeSafeEndpoint(%q) = (%q, %v)", value, endpoint, err)
		}
	}
}

func validTypesafeResponse() string {
	return `{
  "model": "jev-1.13.0",
  "answers": {
    "urgent": {"type": "noul", "noul": 0.82},
    "team": {
      "type": "choice",
      "choice": "billing",
      "confidence": 0.91,
      "probabilities": {"billing": 0.8, "technical": 0.2}
    },
    "risk": {
      "type": "score",
      "score": 1.01,
      "confidence": 0.75,
      "legend": {"0": "Low", "1": "Medium", "2": "High"},
      "probabilities": {"0": 0.33, "1": 0.33, "2": 0.34}
    }
  },
  "usage": {"input_tokens": 42, "output_tokens": 0}
}`
}
