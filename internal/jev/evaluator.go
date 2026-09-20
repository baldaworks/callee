package jev

import (
	"context"
	"errors"
	"fmt"

	"github.com/baldaworks/callee/internal/agent"
)

type adapter interface {
	evaluate(ctx context.Context, api agent.JevAPI, request Request) (Result, Trace, error)
}

type dispatcher struct{ adapters map[string]adapter }

// NewEvaluator constructs the production evaluator with both fixed Jev APIs.
func NewEvaluator() Evaluator {
	core := httpCore{config: defaultHTTPConfig()}

	return dispatcher{adapters: map[string]adapter{
		"typesafe":   typesafeAdapter{core: core},
		"openrouter": openRouterAdapter{core: core},
	}}
}

func (d dispatcher) Evaluate(ctx context.Context, api agent.JevAPI, request Request) (Result, Trace, error) {
	trace := Trace{API: api.Type, RequestedModel: request.Model}
	if err := ValidateRequest(request); err != nil {
		trace.ErrorClass = ErrorResponse

		return Result{}, trace, err
	}

	selected, ok := d.adapters[api.Type]
	if !ok {
		err := &Error{Class: ErrorConfiguration, Op: "select API", Err: fmt.Errorf("unsupported API type %q", api.Type)}
		trace.ErrorClass = err.Class

		return Result{}, trace, err
	}

	result, adapterTrace, err := selected.evaluate(ctx, api, request)
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

	trace.API = result.API
	trace.Provider = result.Provider
	trace.RequestID = result.RequestID
	trace.Model = result.Model
	trace.Usage = result.Usage
	trace.ErrorClass = ""

	return result, trace, nil
}

func mergeTrace(base, update Trace) Trace {
	if update.API != "" {
		base.API = update.API
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
