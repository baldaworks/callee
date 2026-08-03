package workflow

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
	"google.golang.org/adk/v2/session"
)

func TestCollectRootRunOutputSelectsTypedTerminalEnvelope(t *testing.T) {
	t.Parallel()

	want := rootRunOutput{execution: nodeExecution{result: nodeResult{artifact: "done"}}}
	events := func(yield func(*session.Event, error) bool) {
		if !yield(&session.Event{
			Output:   nodeResult{artifact: "not terminal"},
			NodeInfo: &session.NodeInfo{Path: "callee_node_999999"},
		}, nil) {
			return
		}

		yield(&session.Event{
			Output:   want,
			NodeInfo: &session.NodeInfo{Path: "an/arbitrary/adk/path"},
		}, nil)
	}

	got, err := collectRootRunOutput(context.Background(), events)
	if err != nil {
		t.Fatalf("collectRootRunOutput() error: %v", err)
	}

	if got.execution.result.artifact != want.execution.result.artifact {
		t.Fatalf("collectRootRunOutput() artifact = %q, want %q", got.execution.result.artifact, want.execution.result.artifact)
	}
}

func TestCollectRootRunOutputRequiresExactlyOneTerminalEnvelope(t *testing.T) {
	t.Parallel()

	for _, count := range []int{0, 2} {
		t.Run(strings.Repeat("terminal_", count), func(t *testing.T) {
			t.Parallel()

			events := func(yield func(*session.Event, error) bool) {
				for range count {
					if !yield(&session.Event{Output: rootRunOutput{}}, nil) {
						return
					}
				}
			}

			_, err := collectRootRunOutput(context.Background(), events)
			if err == nil || !strings.Contains(err.Error(), "want exactly one") {
				t.Fatalf("collectRootRunOutput() error = %v, want terminal count error", err)
			}
		})
	}
}

func TestADKCompilerNamesAreDeterministicAndPathSafe(t *testing.T) {
	t.Parallel()

	compiler := &adkCompiler{}
	for index, want := range []string{
		"callee_node_000001",
		"callee_node_000002",
		"callee_node_000003",
	} {
		if got := compiler.nextName(); got != want {
			t.Errorf("nextName() call %d = %q, want %q", index+1, got, want)
		}
	}
}

func TestRunnerPreservesCalleeErrorChainAcrossADK(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("provider start sentinel")
	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))

	_, err := (Runner{
		Root:    root,
		Factory: errorFactory{err: wantErr},
	}).Run(context.Background(), "task")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Runner.Run() error = %v, want errors.Is(_, sentinel)", err)
	}
}

func TestRunnerPreservesCancellationAcrossADK(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))
	process := &scriptedProcess{
		visits:       map[string][][]string{"roles/worker": {{"done"}}},
		turnStarted:  make(chan struct{}, 1),
		releaseTurns: make(chan struct{}),
	}

	ctx, cancel := context.WithCancel(context.Background())

	var logs bytes.Buffer

	ctx = zerolog.New(&logs).WithContext(ctx)

	type runResult struct {
		artifact string
		err      error
	}

	done := make(chan runResult, 1)

	go func() {
		artifact, err := (Runner{
			Root:    root,
			Factory: &scriptedFactory{process: process},
		}).Run(ctx, "task")
		done <- runResult{artifact: artifact, err: err}
	}()

	<-process.turnStarted
	cancel()

	got := <-done
	if got.artifact != "" {
		t.Errorf("Runner.Run() artifact = %q, want empty", got.artifact)
	}

	if !errors.Is(got.err, context.Canceled) {
		t.Fatalf("Runner.Run() error = %v, want errors.Is(_, context.Canceled)", got.err)
	}

	if process.closes != 1 {
		t.Errorf("provider closes = %d, want 1", process.closes)
	}

	events := decodeLifecycleEvents(t, &logs)
	if len(events) == 0 {
		t.Fatal("lifecycle events are empty")
	}

	if gotMessage := events[len(events)-1]["message"]; gotMessage != "agent finished" {
		t.Errorf("last lifecycle message = %#v, want agent finished", gotMessage)
	}
}

type errorFactory struct {
	err error
}

func (f errorFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	return nil, f.err
}
