package evaluation

import (
	"context"
	"errors"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
)

type adapterFunc struct {
	call func(context.Context, Config, Request) (Result, Trace, error)
	env  map[string]string
}

func (f adapterFunc) evaluate(ctx context.Context, config Config, request Request) (Result, Trace, error) {
	return f.call(ctx, config, request)
}
func (f adapterFunc) getenv(name string) string { return f.env[name] }

func TestDispatcherResolvesTypeSafeModelAndValidates(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name, authored, environment, want string
	}{
		{name: "authored", authored: "jev-pinned", environment: "jev-env", want: "jev-pinned"},
		{name: "environment", environment: "jev-env", want: "jev-env"},
		{name: "default", want: "jev-latest"},
	} {
		t.Run(test.name, func(t *testing.T) {
			calls := 0
			evaluator := dispatcher{adapters: map[agent.Kind]adapter{
				agent.TypeSafeJevKind: adapterFunc{
					env: map[string]string{"TYPESAFE_DEFAULT_MODEL": test.environment},
					call: func(_ context.Context, _ Config, request Request) (Result, Trace, error) {
						calls++
						result := validResult()
						result.RequestedModel = request.Model

						return result, Trace{Attempts: 1}, nil
					},
				},
			}}

			result, trace, err := evaluator.Evaluate(context.Background(), Config{Kind: agent.TypeSafeJevKind, Model: test.authored}, validRequest())
			if err != nil || result.RequestedModel != test.want || trace.RequestedModel != test.want || calls != 1 {
				t.Fatalf("Evaluate() = (result %#v, trace %#v, error %v, calls %d)", result, trace, err, calls)
			}
		})
	}
}

func TestDispatcherClassifiesAdapterFailure(t *testing.T) {
	t.Parallel()

	evaluator := dispatcher{adapters: map[agent.Kind]adapter{
		agent.OpenRouterDecisionKind: adapterFunc{call: func(context.Context, Config, Request) (Result, Trace, error) {
			return Result{}, Trace{Attempts: 2}, &Error{Class: ErrorRateLimit, Op: "request", Code: 429}
		}},
	}}
	_, trace, err := evaluator.Evaluate(context.Background(), Config{Kind: agent.OpenRouterDecisionKind, Model: "vendor/model"}, validRequest())

	var classified *Error
	if !errors.As(err, &classified) || trace.ErrorClass != ErrorRateLimit || trace.Attempts != 2 {
		t.Fatalf("Evaluate() = (trace %#v, error %v)", trace, err)
	}
}
