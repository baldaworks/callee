package evaluation

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
)

type adapter interface {
	evaluate(ctx context.Context, config Config, request Request) (Result, Trace, error)
	getenv(name string) string
}

type dispatcher struct{ adapters map[agent.Kind]adapter }

func NewEvaluator() Evaluator {
	core := httpCore{config: defaultHTTPConfig()}

	return dispatcher{adapters: map[agent.Kind]adapter{
		agent.TypeSafeJevKind:        typesafeAdapter{core: core},
		agent.OpenRouterDecisionKind: openRouterAdapter{core: core},
	}}
}

func (d dispatcher) Evaluate(ctx context.Context, config Config, request Request) (Result, Trace, error) {
	selected, ok := d.adapters[config.Kind]
	if !ok {
		err := &Error{Class: ErrorConfiguration, Op: "select service", Err: fmt.Errorf("unsupported evaluation kind %q", config.Kind)}

		return Result{}, Trace{ErrorClass: err.Class}, err
	}

	request.Model = strings.TrimSpace(config.Model)
	if config.Kind == agent.TypeSafeJevKind && request.Model == "" {
		request.Model = strings.TrimSpace(selected.getenv("TYPESAFE_DEFAULT_MODEL"))
		if request.Model == "" {
			request.Model = "jev-latest"
		}
	}

	trace := Trace{Service: serviceForKind(config.Kind), RequestedModel: request.Model}
	if err := ValidateRequest(request); err != nil {
		trace.ErrorClass = ErrorResponse

		return Result{}, trace, err
	}

	result, adapterTrace, err := selected.evaluate(ctx, config, request)
	trace = mergeTrace(trace, adapterTrace)

	if err != nil {
		var classified *Error
		if errors.As(err, &classified) {
			trace.ErrorClass = classified.Class
		} else {
			trace.ErrorClass = contextClass(ctx, err)
		}

		return Result{}, trace, err
	}

	if err := ValidateResult(request, result); err != nil {
		trace.ErrorClass = ErrorResponse

		return Result{}, trace, err
	}

	trace.Service = result.Service
	trace.Provider = result.Provider
	trace.RequestID = result.RequestID
	trace.Model = result.Model
	trace.Usage = result.Usage
	trace.ErrorClass = ""

	return result, trace, nil
}

func serviceForKind(kind agent.Kind) string {
	switch kind {
	case agent.TypeSafeJevKind:
		return "typesafe"
	case agent.OpenRouterDecisionKind:
		return "openrouter"
	default:
		return ""
	}
}

func mergeTrace(base, update Trace) Trace {
	if update.Service != "" {
		base.Service = update.Service
	}

	if update.Provider != "" {
		base.Provider = update.Provider
	}

	if update.RequestID != "" {
		base.RequestID = update.RequestID
	}

	if update.RequestedModel != "" {
		base.RequestedModel = update.RequestedModel
	}

	if update.Model != "" {
		base.Model = update.Model
	}

	if update.Attempts != 0 {
		base.Attempts = update.Attempts
	}

	if update.Usage != nil {
		base.Usage = update.Usage
	}

	if update.ErrorClass != "" {
		base.ErrorClass = update.ErrorClass
	}

	return base
}
