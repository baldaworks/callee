package evaluation

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
)

func TestOpenRouterAdapterAcceptsGenericDecisionModelAndProvider(t *testing.T) {
	t.Parallel()

	calls := 0
	config := httpConfig{
		client: &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++

			if request.URL.String() != openRouterEndpoint {
				t.Fatalf("URL = %s", request.URL)
			}

			return response(http.StatusOK, validOpenRouterResponse()), nil
		})},
		getenv: func(name string) string {
			if name == "OPENROUTER_API_KEY" {
				return "secret"
			}

			return ""
		},
		wait: func(context.Context, time.Duration) error { return nil }, jitter: func() float64 { return 0 },
	}
	evaluator := dispatcher{adapters: map[agent.Kind]adapter{agent.OpenRouterDecisionKind: openRouterAdapter{core: httpCore{config: config}}}}

	result, trace, err := evaluator.Evaluate(context.Background(), Config{Kind: agent.OpenRouterDecisionKind, Model: "other/decision-model"}, validRequest())
	if err != nil || calls != 1 || result.Model != "other/decision-model-v2" || result.Provider != "OtherAI" || result.RequestID != "gen-123" || trace.RequestID != "gen-123" {
		t.Fatalf("Evaluate() = (result %#v, trace %#v, error %v, calls %d)", result, trace, err, calls)
	}
}

func validOpenRouterResponse() string {
	return strings.NewReplacer(
		`"model": "jev-1.13.0"`, `"id": "gen-123", "model": "other/decision-model-v2", "provider": "OtherAI"`,
		`"usage": {"input_tokens": 42, "output_tokens": 0}`,
		`"usage": {"input_tokens": 42, "output_tokens": 0, "cost": 0.000018}`,
	).Replace(validTypesafeResponse())
}
