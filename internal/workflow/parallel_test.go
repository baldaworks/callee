package workflow

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
)

func TestRunnerParallelFansOutAndAggregatesInAuthoredOrder(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/left", true, nil, "Left: {{ .Input }}"),
		roleResource(t, "roles/right", true, nil, "Right: {{ .Input }}"),
		compositeResource(t, "workflows/parallel", agent.ParallelKind, []agent.Child{
			{Ref: "roles/left", Alias: "left"},
			{Ref: "roles/right", Alias: "right"},
		}, 0, "{{ .Input }}", ""),
	)
	process := &parallelTestProcess{
		started:   make(chan string, 2),
		release:   make(chan struct{}),
		responses: map[string]string{"left": "L", "right": "R"},
	}
	factory := &parallelTestFactory{process: process}

	type runResult struct {
		artifact string
		err      error
	}

	done := make(chan runResult, 1)

	var logs bytes.Buffer

	runCtx := zerolog.New(zerolog.SyncWriter(&logs)).WithContext(context.Background())
	go func() {
		artifact, err := (Runner{Root: root, Factory: factory}).Run(runCtx, "task")
		done <- runResult{artifact: artifact, err: err}
	}()

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case id := <-process.started:
			seen[id] = true
		case result := <-done:
			t.Fatalf("Runner.Run() completed before both branches started: artifact=%q err=%v", result.artifact, result.err)
		}
	}

	close(process.release)

	result := <-done
	if result.err != nil {
		t.Fatalf("Runner.Run() error: %v", result.err)
	}

	if result.artifact != `{"left":"L","right":"R"}` {
		t.Fatalf("Runner.Run() = %q, want authored-order aggregate", result.artifact)
	}

	if factory.startCount() != 1 {
		t.Errorf("provider starts = %d, want one reused process", factory.startCount())
	}

	if process.sessionCount() != 2 {
		t.Errorf("provider sessions = %d, want two fresh sessions", process.sessionCount())
	}

	for _, role := range process.sessionRoles() {
		if role.Interactive() {
			t.Errorf("Parallel Role %q remained interactive", role.ID)
		}

		if role.EffectivePermissionMode() != agent.PermissionModeAllow {
			t.Errorf("Parallel Role %q permissions = %s, want allow", role.ID, role.EffectivePermissionMode())
		}
	}

	events := decodeLifecycleEvents(t, &logs)

	var finish map[string]any

	for _, event := range events {
		if event["id"] == "workflows/parallel" && event["message"] == "agent finished" {
			finish = event
		}
	}

	if finish == nil {
		t.Fatal("Parallel finish event is missing")
	}

	for key, want := range map[string]any{
		"parallel_branches":  float64(2),
		"parallel_started":   float64(2),
		"parallel_completed": float64(2),
		"parallel_failed":    float64(0),
		"parallel_joined":    true,
	} {
		if got := finish[key]; got != want {
			t.Errorf("finish[%s] = %#v, want %#v", key, got, want)
		}
	}
}

type parallelTestFactory struct {
	mu      sync.Mutex
	starts  int
	process *parallelTestProcess
}

func (f *parallelTestFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	f.mu.Lock()
	f.starts++
	f.mu.Unlock()

	return f.process, nil
}

func (f *parallelTestFactory) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.starts
}

type parallelTestProcess struct {
	mu        sync.Mutex
	started   chan string
	release   chan struct{}
	responses map[string]string
	roles     []agent.Resource
	closes    int
}

func (p *parallelTestProcess) NewSession(_ context.Context, role agent.Resource, effectiveID string) (runtime.AgentSession, error) {
	p.mu.Lock()
	p.roles = append(p.roles, role)
	p.mu.Unlock()

	response, ok := p.responses[effectiveID]
	if !ok {
		return nil, fmt.Errorf("missing response for %s", effectiveID)
	}

	return &parallelTestSession{id: effectiveID, response: response, process: p}, nil
}

func (p *parallelTestProcess) Close() error {
	p.mu.Lock()
	p.closes++
	p.mu.Unlock()

	return nil
}

func (p *parallelTestProcess) sessionCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	return len(p.roles)
}

func (p *parallelTestProcess) sessionRoles() []agent.Resource {
	p.mu.Lock()
	defer p.mu.Unlock()

	return append([]agent.Resource(nil), p.roles...)
}

type parallelTestSession struct {
	id       string
	response string
	process  *parallelTestProcess
	prepared bool
}

func (s *parallelTestSession) Prepare(context.Context) error {
	s.prepared = true

	return nil
}

func (s *parallelTestSession) Turn(ctx context.Context, _ string) (runtime.TurnResult, error) {
	if !s.prepared {
		return runtime.TurnResult{}, fmt.Errorf("session %s was not prepared", s.id)
	}

	s.process.started <- s.id

	select {
	case <-s.process.release:
		return runtime.TurnResult{Content: s.response}, nil
	case <-ctx.Done():
		return runtime.TurnResult{}, ctx.Err()
	}
}

func TestRunnerParallelSingleChildJoin(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/only", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/parallel", agent.ParallelKind, []agent.Child{{Ref: "roles/only", Alias: "only"}}, 0, "{{ .Input }}", ""),
	)
	release := make(chan struct{})
	close(release)
	process := &parallelTestProcess{
		started:   make(chan string, 1),
		release:   release,
		responses: map[string]string{"only": "done"},
	}

	artifact, err := (Runner{Root: root, Factory: &parallelTestFactory{process: process}}).Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if artifact != `{"only":"done"}` {
		t.Fatalf("Runner.Run() = %q, want one-child aggregate", artifact)
	}
}

func TestRunnerParallelReportsFailuresInAuthoredOrder(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/first", false, nil, "{{ .Input }}"),
		roleResource(t, "roles/second", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/parallel", agent.ParallelKind, []agent.Child{
			{Ref: "roles/first", Alias: "first"},
			{Ref: "roles/second", Alias: "second"},
		}, 0, "{{ .Input }}", ""),
	)
	process := &parallelTestProcess{
		started: make(chan string, 2),
		release: make(chan struct{}),
		responses: map[string]string{
			"first":  "first failure\n\n" + controlFail,
			"second": "second failure\n\n" + controlFail,
		},
	}

	done := make(chan error, 1)

	go func() {
		_, err := (Runner{Root: root, Factory: &parallelTestFactory{process: process}}).Run(context.Background(), "task")
		done <- err
	}()

	<-process.started
	<-process.started
	close(process.release)

	err := <-done
	if err == nil {
		t.Fatal("Runner.Run() error = nil, want Parallel failure")
	}

	message := err.Error()
	first := strings.Index(message, `parallel child "first" failed`)

	second := strings.Index(message, `parallel child "second" failed`)
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("Runner.Run() error = %q, want authored failure order", message)
	}
}

func TestRunStateSnapshotPreservesConcreteStateTypes(t *testing.T) {
	t.Parallel()

	run := &runState{state: map[string]any{
		"outputs": map[string]string{"worker": "done"},
		"scripts": map[string]any{"check": map[string]any{"exitCode": int64(7)}},
		"nested":  map[string]any{"items": []any{int32(3), true}},
	}}

	snapshot := run.snapshotState()
	if _, ok := snapshot["outputs"].(map[string]string); !ok {
		t.Fatalf("snapshot outputs type = %T, want map[string]string", snapshot["outputs"])
	}

	exitCode := snapshot["scripts"].(map[string]any)["check"].(map[string]any)["exitCode"]
	if _, ok := exitCode.(int64); !ok {
		t.Fatalf("snapshot exitCode type = %T, want int64", exitCode)
	}

	snapshot["nested"].(map[string]any)["items"].([]any)[0] = int32(9)

	original := run.state["nested"].(map[string]any)["items"].([]any)[0]
	if original != int32(3) {
		t.Fatalf("snapshot mutation changed original state to %#v", original)
	}
}
