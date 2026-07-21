package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
)

func TestRunAgentsGroupsProvidersAndSessionConfigurations(t *testing.T) {
	t.Parallel()

	roles := []agent.Resource{
		doctorRole("roles/a", "model-a"),
		doctorRole("roles/b", "model-a"),
		doctorRole("roles/c", "model-c"),
	}
	roles[0].Spec.Permissions = &agent.Permissions{Mode: agent.PermissionModeAllow}
	roles[1].Spec.Permissions = &agent.Permissions{Mode: agent.PermissionModeDeny}
	process := &doctorProcess{}
	factory := &doctorFactory{process: process}

	var stdout bytes.Buffer
	if err := RunAgents(context.Background(), roles, factory, time.Second, &stdout); err != nil {
		t.Fatalf("RunAgents() error: %v", err)
	}

	if factory.starts != 1 {
		t.Errorf("provider starts = %d, want 1", factory.starts)
	}

	if process.sessions != 2 {
		t.Errorf("disposable sessions = %d, want 2 unique configurations", process.sessions)
	}

	if process.checks != 2 {
		t.Errorf("remote session checks = %d, want 2", process.checks)
	}

	if !process.closed {
		t.Errorf("provider process was not closed")
	}

	for _, want := range []string{`agent "roles/a": ok`, `agent "roles/b": ok`, `agent "roles/c": ok`, "callee doctor: ok"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout %q does not contain %q", stdout.String(), want)
		}
	}
}

func TestRunAgentsAttributesGroupFailure(t *testing.T) {
	t.Parallel()

	roles := []agent.Resource{doctorRole("roles/a", ""), doctorRole("roles/b", "")}
	process := &doctorProcess{sessionErr: errors.New("session failed")}

	var stdout bytes.Buffer

	err := RunAgents(context.Background(), roles, &doctorFactory{process: process}, time.Second, &stdout)
	if err == nil || !strings.Contains(err.Error(), `agent "roles/a"`) || !strings.Contains(err.Error(), `agent "roles/b"`) {
		t.Fatalf("RunAgents() error = %v, want both roles attributed", err)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
}

func TestRunAgentsAttributesSessionFailureOnlyToMatchingTuple(t *testing.T) {
	t.Parallel()

	roles := []agent.Resource{
		doctorRole("roles/a", "model-a"),
		doctorRole("roles/b", "model-a"),
		doctorRole("roles/c", "model-c"),
	}
	process := &doctorProcess{prepareErrors: map[string]error{"model-a": errors.New("model-a rejected")}}

	var stdout bytes.Buffer

	err := RunAgents(context.Background(), roles, &doctorFactory{process: process}, time.Second, &stdout)
	if err == nil {
		t.Fatal("RunAgents() error = nil, want tuple failure")
	}

	for _, roleID := range []string{"roles/a", "roles/b"} {
		if !strings.Contains(err.Error(), `agent "`+roleID+`"`) {
			t.Errorf("RunAgents() error = %v, want attribution to %s", err, roleID)
		}
	}

	if strings.Contains(err.Error(), `agent "roles/c"`) {
		t.Errorf("RunAgents() error = %v, must not attribute model-a failure to roles/c", err)
	}

	if process.checks != 2 {
		t.Errorf("session checks = %d, want both independent tuples checked", process.checks)
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty on doctor failure", stdout.String())
	}
}

func TestRunAgentsClosesProviderAfterGroupTimeout(t *testing.T) {
	t.Parallel()

	process := &doctorProcess{waitForContext: true}

	err := RunAgents(context.Background(), []agent.Resource{doctorRole("roles/a", "model-a")}, &doctorFactory{process: process}, time.Millisecond, &bytes.Buffer{})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("RunAgents() error = %v, want deadline exceeded", err)
	}

	if !process.closed {
		t.Fatal("provider process was not closed after group timeout")
	}
}

func TestWriteGraphFormats(t *testing.T) {
	t.Parallel()

	worker := doctorRole("roles/worker", "")
	maxIterations := 2
	pipeline := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.LoopKind,
		ID:         "workflows/pipeline",
		Spec: agent.Spec{
			Description: "pipeline",
			Children: []agent.Child{{
				Ref:         "roles/worker",
				Alias:       "worker",
				CanEscalate: true,
				Session:     agent.SessionModeStateful,
			}},
			Body:          "{{ .Input }}",
			MaxIterations: &maxIterations,
		},
	}

	configured, err := registry.NewAgentRegistry([]agent.Resource{worker, pipeline})
	if err != nil {
		t.Fatalf("registry.NewAgentRegistry() error: %v", err)
	}

	for _, test := range []struct {
		format string
		want   string
	}{
		{format: "text", want: "-> roles/worker alias=worker canEscalate=true session=stateful"},
		{format: "mermaid", want: "worker, canEscalate=true, session=stateful"},
		{format: "dot", want: "worker, canEscalate=true, session=stateful"},
	} {
		t.Run(test.format, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer
			if err := WriteGraph(&output, configured, test.format); err != nil {
				t.Fatalf("WriteGraph() error: %v", err)
			}

			if !strings.Contains(output.String(), test.want) {
				t.Errorf("WriteGraph() = %q, want containing %q", output.String(), test.want)
			}
		})
	}
}

type doctorFactory struct {
	process *doctorProcess
	starts  int
}

func (f *doctorFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	f.starts++

	return f.process, nil
}

type doctorProcess struct {
	sessions       int
	checks         int
	sessionErr     error
	prepareErrors  map[string]error
	waitForContext bool
	closed         bool
}

func (p *doctorProcess) NewSession(_ context.Context, role agent.Resource, _ string) (runtime.AgentSession, error) {
	p.sessions++
	if p.sessionErr != nil {
		return nil, p.sessionErr
	}

	return doctorSession{process: p, prepareErr: p.prepareErrors[role.Spec.Provider.Model]}, nil
}

func (p *doctorProcess) Close() error {
	p.closed = true

	return nil
}

type doctorSession struct {
	process    *doctorProcess
	prepareErr error
}

func (doctorSession) Turn(context.Context, string) (runtime.TurnResult, error) {
	return runtime.TurnResult{}, errors.New("doctor must not send a model prompt")
}

func (s doctorSession) Prepare(ctx context.Context) error {
	s.process.checks++
	if s.process.waitForContext {
		<-ctx.Done()

		return ctx.Err()
	}

	return s.prepareErr
}

func doctorRole(id, model string) agent.Resource {
	repl := false

	return agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.RoleKind,
		ID:         id,
		Spec: agent.Spec{
			Description: id,
			Provider:    &agent.Provider{Type: "codex", Model: model},
			REPL:        &repl,
			Body:        "{{ .Input }}",
		},
	}
}
