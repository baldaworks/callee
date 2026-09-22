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

func TestRunAgentsDefersDynamicRolesWithoutStartingProviders(t *testing.T) {
	t.Parallel()

	dynamic := doctorDynamicRole("roles/dynamic")

	var stdout bytes.Buffer

	if err := RunAgents(context.Background(), []agent.Resource{dynamic}, nil, time.Second, &stdout); err != nil {
		t.Fatalf("RunAgents() error: %v", err)
	}

	for _, want := range []string{`agent "roles/dynamic": deferred (provider resolves at runtime)`, "callee doctor: ok"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout %q does not contain %q", stdout.String(), want)
		}
	}
}

func TestRunAgentsChecksStaticRolesAndDefersDynamicRoles(t *testing.T) {
	t.Parallel()

	process := &doctorProcess{}
	factory := &doctorFactory{process: process}

	var stdout bytes.Buffer

	err := RunAgents(context.Background(), []agent.Resource{
		doctorDynamicRole("roles/dynamic"),
		doctorRole("roles/static", ""),
	}, factory, time.Second, &stdout)
	if err != nil {
		t.Fatalf("RunAgents() error: %v", err)
	}

	if factory.starts != 1 || process.sessions != 1 {
		t.Fatalf("starts/sessions = %d/%d, want 1/1", factory.starts, process.sessions)
	}

	for _, want := range []string{`agent "roles/dynamic": deferred (provider resolves at runtime)`, `agent "roles/static": ok`} {
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

func TestRunAgentsChecksEvaluationCredentialsWithoutInference(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "typesafe-key")
	t.Setenv("TYPESAFE_BASE_URL", "")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "")
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")

	jevs := []agent.Resource{
		doctorEvaluation("judges/native", "typesafe"),
		doctorEvaluation("judges/router", "openrouter"),
	}

	var stdout bytes.Buffer
	if err := RunAgents(context.Background(), jevs, nil, time.Second, &stdout); err != nil {
		t.Fatalf("RunAgents() error: %v", err)
	}

	for _, want := range []string{`agent "judges/native": ok`, `agent "judges/router": ok`, "callee doctor: ok"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout %q does not contain %q", stdout.String(), want)
		}
	}
}

func TestRunAgentsRequiresOnlyMatchingEvaluationCredential(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "")
	t.Setenv("TYPESAFE_BASE_URL", "")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "")
	t.Setenv("OPENROUTER_API_KEY", "openrouter-key")

	factory := &doctorFactory{process: &doctorProcess{}}

	err := RunAgents(context.Background(), []agent.Resource{
		doctorEvaluation("judges/native", "typesafe"),
		doctorEvaluation("judges/router", "openrouter"),
	}, factory, time.Second, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "TYPESAFE_API_KEY") || strings.Contains(err.Error(), "OPENROUTER_API_KEY is required") {
		t.Fatalf("RunAgents() error = %v, want only missing TypeSafe credential", err)
	}

	if factory.starts != 0 {
		t.Fatalf("provider starts = %d, want 0 for evaluation-only doctor", factory.starts)
	}
}

func TestRunAgentsChecksMixedRoleAndEvaluationResources(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "typesafe-key")
	t.Setenv("TYPESAFE_BASE_URL", "")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "")
	t.Setenv("OPENROUTER_API_KEY", "")

	factory := &doctorFactory{process: &doctorProcess{}}

	err := RunAgents(context.Background(), []agent.Resource{
		doctorRole("roles/worker", ""),
		doctorEvaluation("judges/router", "openrouter"),
	}, factory, time.Second, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), `agent "judges/router"`) || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Fatalf("RunAgents() error = %v, want missing OpenRouter credential", err)
	}

	if factory.starts != 1 {
		t.Fatalf("provider starts = %d, want Role runtime check despite evaluation credential failure", factory.starts)
	}
}

func TestRunAgentsValidatesTypeSafeEnvironmentWithoutInference(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "typesafe-key")
	t.Setenv("TYPESAFE_BASE_URL", "http://unsafe.example")
	t.Setenv("TYPESAFE_DEFAULT_MODEL", "jev-latest")

	err := RunAgents(context.Background(), []agent.Resource{
		doctorEvaluation("judges/native", "typesafe"),
	}, nil, time.Second, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "TYPESAFE_BASE_URL") || strings.Contains(err.Error(), "unsafe.example") {
		t.Fatalf("RunAgents() error = %v, want safe TypeSafe base URL failure", err)
	}
}

func TestWriteGraphFormats(t *testing.T) {
	t.Parallel()

	worker := doctorDynamicRole("roles/worker")
	maxIterations := 2
	pipeline := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.LoopKind,
		ID:         "workflows/pipeline",
		Spec: agent.Spec{
			Description:   "pipeline",
			Children:      []agent.Child{{Ref: "roles/worker", Alias: "worker", CanEscalate: true}},
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
		{format: "text", want: "-> roles/worker alias=worker canEscalate=true"},
		{format: "mermaid", want: "worker, canEscalate=true"},
		{format: "dot", want: "worker, canEscalate=true"},
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

			if !strings.Contains(output.String(), "roles/worker [DynamicRole]") {
				t.Errorf("WriteGraph() = %q, want DynamicRole node", output.String())
			}
		})
	}
}

func TestWriteGraphFormatsLabelRouterEdges(t *testing.T) {
	t.Parallel()

	named := doctorRole("roles/named", "")
	fallback := doctorRole("roles/fallback", "")
	router := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.RouterKind,
		ID:         "workflows/router",
		Spec: agent.Spec{
			Description: "router",
			Route:       "{{ .Input }}",
			Children: []agent.Child{
				{Ref: "roles/named", Alias: "named", Route: `bug|"urgent"`},
				{Ref: "roles/fallback", Alias: "fallback", Default: true},
			},
			Body: "{{ .Input }}",
		},
	}

	configured, err := registry.NewAgentRegistry([]agent.Resource{named, fallback, router})
	if err != nil {
		t.Fatalf("registry.NewAgentRegistry() error: %v", err)
	}

	for _, test := range []struct {
		format string
		wants  []string
	}{
		{format: "text", wants: []string{`alias=named route="bug|\"urgent\"" canEscalate=false`, "alias=fallback default=true canEscalate=false"}},
		{format: "mermaid", wants: []string{`named, route=&quot;bug&#124;&#34;urgent&#34;&quot;, canEscalate=false`, "fallback, default=true, canEscalate=false"}},
		{format: "dot", wants: []string{`named, route=\"bug|\\\"urgent\\\"\", canEscalate=false`, "fallback, default=true, canEscalate=false"}},
	} {
		t.Run(test.format, func(t *testing.T) {
			t.Parallel()

			var output bytes.Buffer

			if err := WriteGraph(&output, configured, test.format); err != nil {
				t.Fatalf("WriteGraph() error: %v", err)
			}

			for _, want := range test.wants {
				if !strings.Contains(output.String(), want) {
					t.Errorf("WriteGraph() = %q, want containing %q", output.String(), want)
				}
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
			Interactive: &repl,
			Body:        "{{ .Input }}",
		},
	}
}

func doctorDynamicRole(id string) agent.Resource {
	role := doctorRole(id, "")
	role.Kind = agent.DynamicRoleKind
	role.Spec.Provider.Type = `{{ .State.provider }}`
	role.Spec.State = map[string]any{"provider": "codex"}

	return role
}

func doctorEvaluation(id, apiType string) agent.Resource {
	kind := agent.TypeSafeJevKind
	if apiType == "openrouter" {
		kind = agent.OpenRouterDecisionKind
	}

	result := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       kind,
		ID:         id,
		Spec: agent.Spec{
			Description: id,
			Body:        "{{ .Input }}",
			Questions: map[string]agent.EvaluationQuestion{
				"ready": {Type: "noul", Instructions: "Ready?", Criteria: map[string]any{"true": "yes", "false": "no"}},
			},
		},
	}
	if kind == agent.OpenRouterDecisionKind {
		result.Spec.Model = "typesafe/jev-1.13"
	}

	return result
}
