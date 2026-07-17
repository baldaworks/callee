package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
)

func TestRunnerSequentialPromotesOutputs(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "Worker: {{ .Input }}"),
		roleResource(t, "roles/validator", false, nil, "Validate: {{ .Input }}"),
		compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
			{Ref: "roles/worker", Alias: "worker"},
			{Ref: "roles/validator", Alias: "validator"},
		}, 0, "{{ .Input }}", "worker={{ .State.outputs.worker }} validator={{ .State.outputs.validator }}"),
	)

	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker":    {{"draft"}},
		"roles/validator": {{"approved"}},
	}}
	factory := &scriptedFactory{process: process}

	got, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "build it")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if want := "worker=draft validator=approved"; got != want {
		t.Errorf("Runner.Run() = %q, want %q", got, want)
	}

	if factory.starts != 1 {
		t.Errorf("provider starts = %d, want 1", factory.starts)
	}

	if gotPrompt := process.prompts["roles/validator"][0]; !strings.Contains(gotPrompt, "Validate: draft") {
		t.Errorf("validator prompt = %q, want predecessor output", gotPrompt)
	}
}

func TestRunnerLoopConsumesEscalation(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "Worker goal: {{ .Input }}"),
		roleResource(t, "roles/validator", false, nil, "Validate worker: {{ .Input }}"),
		compositeResource(t, "workflows/goalkeeper", agent.LoopKind, []agent.Child{
			{Ref: "roles/worker", Alias: "worker"},
			{Ref: "roles/validator", Alias: "validator", Input: "{{ .State.outputs.worker }}"},
		}, 5, "{{ .Input }}", "GoalKeeper finished with result: {{ .State.outputs.validator }}"),
	)

	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker": {
			{"draft one"},
			{"draft two"},
		},
		"roles/validator": {
			{"needs work"},
			{"approved\n\n" + controlEscalate},
		},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "ship feature")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if want := "GoalKeeper finished with result: approved"; got != want {
		t.Errorf("Runner.Run() = %q, want %q", got, want)
	}

	if gotVisits := process.sessions; gotVisits != 4 {
		t.Errorf("sessions = %d, want 4 fresh visits", gotVisits)
	}

	validatorPrompts := process.prompts["roles/validator"]
	if len(validatorPrompts) != 2 || !strings.Contains(validatorPrompts[1], "draft two") {
		t.Errorf("validator prompts = %v, want second worker output", validatorPrompts)
	}
}

func TestRunnerSequentialStickyEscalationRunsRemainingChildren(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/planner", false, nil, "{{ .Input }}"),
		roleResource(t, "roles/implementer", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
			{Ref: "roles/planner", Alias: "planner"},
			{Ref: "roles/implementer", Alias: "implementer"},
		}, 0, "{{ .Input }}", ""),
	)

	process := &scriptedProcess{visits: map[string][][]string{
		"roles/planner":     {{"plan\n\n" + controlEscalate}},
		"roles/implementer": {{"implementation"}},
	}}

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "task")
	if err == nil || !strings.Contains(err.Error(), "unconsumed escalation") {
		t.Fatalf("Runner.Run() error = %v, want unconsumed escalation", err)
	}

	for _, want := range []string{`resource "roles/planner"`, `path "workflows/pipeline -> planner"`, `root "workflows/pipeline"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Runner.Run() error = %v, want containing %q", err, want)
		}
	}

	if len(process.prompts["roles/implementer"]) != 1 {
		t.Errorf("implementer turns = %d, want 1", len(process.prompts["roles/implementer"]))
	}
}

func TestRunnerREPLRetainsVisitSession(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/planner", true, nil, "Plan: {{ .Input }}"))
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/planner": {{"Which target?\n\n" + controlAwait, "Final plan\n\n" + controlReturn}},
	}}
	interactor := &scriptedInteractor{answers: []string{"linux"}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}, Interactor: interactor}).Run(context.Background(), "make plan")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "Final plan" {
		t.Errorf("Runner.Run() = %q, want Final plan", got)
	}

	if process.sessions != 1 {
		t.Errorf("sessions = %d, want 1", process.sessions)
	}

	if len(interactor.displayed) != 1 || interactor.displayed[0] != "Which target?" {
		t.Errorf("displayed = %v, want question", interactor.displayed)
	}
}

func TestRunnerCollectsQualifiedParameterOnceAcrossLoopVisits(t *testing.T) {
	t.Parallel()

	params := map[string]string{"language": "Implementation language"}
	loop := compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{{Ref: "roles/worker", Alias: "worker"}}, 2, "{{ .Input }}", "")
	loop.Spec.OnExhausted = "complete"
	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, params, `{{ .Input }} in {{ .Params.language }}`),
		loop,
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker": {{"first"}, {"second"}},
	}}
	interactor := &scriptedInteractor{answers: []string{"Go"}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}, Interactor: interactor}).Run(context.Background(), "build")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "second" {
		t.Errorf("Runner.Run() = %q, want second", got)
	}

	if len(interactor.labels) != 1 || !strings.HasPrefix(interactor.labels[0], "worker.language") {
		t.Errorf("parameter prompts = %v, want worker.language once", interactor.labels)
	}

	for _, prompt := range process.prompts["roles/worker"] {
		if !strings.Contains(prompt, "in Go") {
			t.Errorf("worker prompt = %q, want cached parameter", prompt)
		}
	}
}

func TestRunnerCleanupFailureSuppressesArtifact(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))
	process := &scriptedProcess{
		visits:   map[string][][]string{"roles/worker": {{"result"}}},
		closeErr: errors.New("close failed"),
	}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "task")
	if err == nil || !strings.Contains(err.Error(), "cleanup workflow providers") {
		t.Fatalf("Runner.Run() error = %v, want cleanup error", err)
	}

	if got != "" {
		t.Errorf("Runner.Run() = %q, want empty artifact", got)
	}
}

func TestRunnerKeepsProviderContextLiveUntilClose(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))
	process := &scriptedProcess{
		visits:                    map[string][][]string{"roles/worker": {{"result"}}},
		requireLiveProcessContext: true,
	}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "result" {
		t.Errorf("Runner.Run() = %q, want result", got)
	}

	if process.processContextErrAtClose != nil {
		t.Errorf("provider process context at Close() = %v, want live context", process.processContextErrAtClose)
	}

	if !errors.Is(process.processContext.Err(), context.Canceled) {
		t.Errorf("provider process context after cleanup = %v, want context canceled", process.processContext.Err())
	}
}

func TestRunnerStateModifierFailureIsAtomicAndPrecedesProviderStart(t *testing.T) {
	t.Parallel()

	worker := roleResource(t, "roles/worker", false, nil, "{{ .Input }}")
	worker.Spec.State = map[string]any{
		"first":  "written",
		"second": `{{ fail "state failed" }}`,
	}
	root := resolvedRoot(t, worker)
	factory := &scriptedFactory{process: &scriptedProcess{}}

	_, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "task")
	if err == nil || !strings.Contains(err.Error(), "state failed") {
		t.Fatalf("Runner.Run() error = %v, want state failure", err)
	}

	if factory.starts != 0 {
		t.Errorf("provider starts = %d, want 0", factory.starts)
	}
}

func TestRunnerStateModifierReportsFirstLexicographicFailure(t *testing.T) {
	t.Parallel()

	worker := roleResource(t, "roles/worker", false, nil, "{{ .Input }}")
	worker.Spec.State = map[string]any{
		"zeta": map[string]any{"failure": `{{ fail "zeta failed" }}`},
		"alpha": map[string]any{
			"zeta": `{{ fail "nested zeta failed" }}`,
			"beta": `{{ fail "nested beta failed" }}`,
		},
	}
	root := resolvedRoot(t, worker)

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: &scriptedProcess{}}}).Run(context.Background(), "task")
	if err == nil || !strings.Contains(err.Error(), "nested beta failed") {
		t.Fatalf("Runner.Run() error = %v, want first lexicographic state failure", err)
	}
}

type scriptedFactory struct {
	process *scriptedProcess
	starts  int
}

func (f *scriptedFactory) Start(ctx context.Context, _ runtime.Provider) (runtime.ProviderProcess, error) {
	f.starts++
	f.process.processContext = ctx

	return f.process, nil
}

type scriptedProcess struct {
	visits                    map[string][][]string
	prompts                   map[string][]string
	processContext            context.Context
	processContextErrAtClose  error
	requireLiveProcessContext bool
	sessions                  int
	prepareErr                error
	closeErr                  error
}

func (p *scriptedProcess) NewSession(_ context.Context, role agent.Resource) (runtime.AgentSession, error) {
	if p.requireLiveProcessContext && p.processContext.Err() != nil {
		return nil, fmt.Errorf("provider process context canceled before session creation: %w", p.processContext.Err())
	}

	visits := p.visits[role.ID]
	if len(visits) == 0 {
		return nil, fmt.Errorf("no scripted visit for %s", role.ID)
	}

	p.visits[role.ID] = visits[1:]
	p.sessions++

	return &scriptedSession{roleID: role.ID, responses: append([]string(nil), visits[0]...), process: p}, nil
}

func (p *scriptedProcess) Close() error {
	if p.processContext != nil {
		p.processContextErrAtClose = p.processContext.Err()
	}

	return p.closeErr
}

type scriptedSession struct {
	roleID     string
	responses  []string
	process    *scriptedProcess
	prepared   bool
	prepareCtx context.Context
}

func (s *scriptedSession) Prepare(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("prepare context for %s has no deadline", s.roleID)
	}

	if s.process.prepareErr != nil {
		return s.process.prepareErr
	}

	s.prepared = true
	s.prepareCtx = ctx

	return nil
}

func (s *scriptedSession) Turn(_ context.Context, prompt string) (string, error) {
	if !s.prepared {
		return "", fmt.Errorf("session for %s was not prepared", s.roleID)
	}

	if !errors.Is(s.prepareCtx.Err(), context.Canceled) {
		return "", fmt.Errorf("prepare budget for %s remained active during turn", s.roleID)
	}

	if len(s.responses) == 0 {
		return "", fmt.Errorf("no scripted response for %s", s.roleID)
	}

	if s.process.prompts == nil {
		s.process.prompts = make(map[string][]string)
	}

	s.process.prompts[s.roleID] = append(s.process.prompts[s.roleID], prompt)

	response := s.responses[0]
	s.responses = s.responses[1:]

	return response, nil
}

type scriptedInteractor struct {
	answers   []string
	labels    []string
	displayed []string
}

func (i *scriptedInteractor) Prompt(_ context.Context, label string) (string, error) {
	i.labels = append(i.labels, label)
	if len(i.answers) == 0 {
		return "", fmt.Errorf("no scripted answer for %s", label)
	}

	answer := i.answers[0]
	i.answers = i.answers[1:]

	return answer, nil
}

func (i *scriptedInteractor) Display(text string) error {
	i.displayed = append(i.displayed, text)

	return nil
}

func resolvedRoot(t *testing.T, resources ...agent.Resource) *registry.ResolvedNode {
	t.Helper()

	configured, err := registry.NewAgentRegistry(resources)
	if err != nil {
		t.Fatalf("registry.NewAgentRegistry() error: %v", err)
	}

	root, err := configured.Resolve(resources[len(resources)-1].ID)
	if err != nil {
		t.Fatalf("registry.Resolve() error: %v", err)
	}

	return root
}

func roleResource(t *testing.T, id string, repl bool, params map[string]string, body string) agent.Resource {
	t.Helper()

	replValue := repl

	resource := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.RoleKind,
		ID:         id,
		Source:     id + ".md",
		Spec: agent.Spec{
			Description: id,
			Provider:    &agent.Provider{Type: "codex"},
			REPL:        &replValue,
			Params:      params,
			Body:        body,
		},
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("role resource %q validation error: %v", id, err)
	}

	return resource
}

func compositeResource(t *testing.T, id string, kind agent.Kind, children []agent.Child, iterations int, body, output string) agent.Resource {
	t.Helper()

	resource := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       kind,
		ID:         id,
		Source:     id + ".md",
		Spec: agent.Spec{
			Description: id,
			Children:    children,
			Body:        body,
			Output:      output,
		},
	}
	if kind == agent.LoopKind {
		resource.Spec.MaxIterations = &iterations
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("composite resource %q validation error: %v", id, err)
	}

	return resource
}
