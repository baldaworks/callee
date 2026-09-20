package jev

import (
	"context"
	"errors"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
)

type adapterFunc func(context.Context, agent.JevAPI, Request) (Result, Trace, error)

func (f adapterFunc) evaluate(ctx context.Context, api agent.JevAPI, request Request) (Result, Trace, error) {
	return f(ctx, api, request)
}

func TestDispatcherValidatesBeforeAndAfterAdapter(t *testing.T) {
	t.Parallel()

	calls := 0
	evaluator := dispatcher{adapters: map[string]adapter{
		"typesafe": adapterFunc(func(context.Context, agent.JevAPI, Request) (Result, Trace, error) {
			calls++

			return validResult(), Trace{Attempts: 1}, nil
		}),
	}}

	request := validRequest()

	result, trace, err := evaluator.Evaluate(context.Background(), agent.JevAPI{Type: "typesafe"}, request)
	if err != nil {
		t.Fatalf("Evaluate() error = %v", err)
	}

	if result.Model != "jev-1.13.0" || trace.Attempts != 1 || trace.Model != result.Model || calls != 1 {
		t.Fatalf("Evaluate() result/trace/calls = (%#v, %#v, %d)", result, trace, calls)
	}

	request.State = nil
	if _, _, err := evaluator.Evaluate(context.Background(), agent.JevAPI{Type: "typesafe"}, request); err == nil || calls != 1 {
		t.Fatalf("invalid request Evaluate() error = %v, calls = %d", err, calls)
	}
}

func TestDispatcherClassifiesAdapterFailure(t *testing.T) {
	t.Parallel()

	evaluator := dispatcher{adapters: map[string]adapter{
		"typesafe": adapterFunc(func(context.Context, agent.JevAPI, Request) (Result, Trace, error) {
			return Result{}, Trace{Attempts: 2}, &Error{Class: ErrorRateLimit, Op: "request", Code: 429}
		}),
	}}
	_, trace, err := evaluator.Evaluate(context.Background(), agent.JevAPI{Type: "typesafe"}, validRequest())

	var classified *Error
	if !errors.As(err, &classified) || trace.ErrorClass != ErrorRateLimit || trace.Attempts != 2 {
		t.Fatalf("Evaluate() = (trace %#v, error %v)", trace, err)
	}
}
