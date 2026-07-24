package workflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

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

	if got, want := strings.Join(process.effectiveIDs, ","), "worker,validator"; got != want {
		t.Errorf("effective session IDs = %q, want %q", got, want)
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
			{Ref: "roles/validator", Alias: "validator", CanEscalate: true, Input: "{{ .State.outputs.worker }}"},
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

	if gotSessions := process.sessions; gotSessions != 4 {
		t.Errorf("sessions = %d, want one fresh session for each of 4 Role visits", gotSessions)
	}

	validatorPrompts := process.prompts["roles/validator"]
	if len(validatorPrompts) != 2 || !strings.Contains(validatorPrompts[1], "draft two") {
		t.Errorf("validator prompts = %v, want second worker output", validatorPrompts)
	}

	if !strings.Contains(validatorPrompts[1], controlEscalate) {
		t.Errorf("validator prompt does not allow escalation beneath Loop:\n%s", validatorPrompts[1])
	}
}

func TestRunnerLoopMakesEscalationAvailableThroughSequential(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/reviewer", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/review", agent.SequentialKind, []agent.Child{
			{Ref: "roles/reviewer", Alias: "reviewer", CanEscalate: true},
		}, 0, "{{ .Input }}", ""),
		compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
			{Ref: "workflows/review", Alias: "review", CanEscalate: true},
		}, 1, "{{ .Input }}", "{{ .State.outputs.reviewer }}"),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/reviewer": {{"approved\n\n" + controlEscalate}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "review")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "approved" {
		t.Errorf("Runner.Run() = %q, want approved", got)
	}

	if prompt := process.prompts["roles/reviewer"][0]; !strings.Contains(prompt, controlEscalate) {
		t.Errorf("reviewer prompt does not allow escalation through Sequential beneath Loop:\n%s", prompt)
	}
}

func TestRunnerSequentialContinuesAfterNestedLoopEscalation(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/refiner", false, nil, "Refine: {{ .Input }}"),
		roleResource(t, "roles/publisher", false, nil, "Publish: {{ .Input }}"),
		compositeResource(t, "workflows/refinement", agent.LoopKind, []agent.Child{
			{Ref: "roles/refiner", Alias: "refiner", CanEscalate: true},
		}, 3, "{{ .Input }}", "refined={{ .State.outputs.refiner }}"),
		compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
			{Ref: "workflows/refinement", Alias: "refinement"},
			{Ref: "roles/publisher", Alias: "publisher"},
		}, 0, "{{ .Input }}", ""),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/refiner":   {{"draft\n\n" + controlEscalate}},
		"roles/publisher": {{"published"}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "write")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "published" {
		t.Errorf("Runner.Run() = %q, want published", got)
	}

	if prompt := process.prompts["roles/refiner"][0]; !strings.Contains(prompt, controlEscalate) {
		t.Errorf("refiner prompt does not allow escalation inside nested Loop:\n%s", prompt)
	}

	publisherPrompt := process.prompts["roles/publisher"][0]
	if strings.Contains(publisherPrompt, controlEscalate) {
		t.Errorf("publisher prompt unexpectedly allows escalation outside Loop:\n%s", publisherPrompt)
	}

	if !strings.Contains(publisherPrompt, "Publish: refined=draft") {
		t.Errorf("publisher prompt = %q, want nested Loop output", publisherPrompt)
	}
}

func TestRunnerOuterLoopConsumesStickySequentialEscalation(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/evaluator", false, nil, "Evaluate: {{ .Input }}"),
		roleResource(t, "roles/recorder", false, nil, "Record: {{ .Input }}"),
		compositeResource(t, "workflows/phase", agent.SequentialKind, []agent.Child{
			{Ref: "roles/evaluator", Alias: "evaluator", CanEscalate: true},
			{Ref: "roles/recorder", Alias: "recorder", CanEscalate: true},
		}, 0, "{{ .Input }}", ""),
		compositeResource(t, "workflows/outer", agent.LoopKind, []agent.Child{
			{Ref: "workflows/phase", Alias: "phase", CanEscalate: true},
		}, 3, "{{ .Input }}", "result={{ .State.outputs.recorder }}"),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/evaluator": {{"ready\n\n" + controlEscalate}},
		"roles/recorder":  {{"recorded"}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "evaluate")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "result=recorded" {
		t.Errorf("Runner.Run() = %q, want result=recorded", got)
	}

	if prompt := process.prompts["roles/recorder"][0]; !strings.Contains(prompt, "Record: ready") {
		t.Errorf("recorder prompt = %q, want escalator artifact", prompt)
	}

	for _, roleID := range []string{"roles/evaluator", "roles/recorder"} {
		if prompt := process.prompts[roleID][0]; !strings.Contains(prompt, controlEscalate) {
			t.Errorf("%s prompt does not preserve Loop escalation capability:\n%s", roleID, prompt)
		}
	}
}

func TestRunnerNestedLoopEscalationStopsNearestLoopOnly(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/inner-worker", false, nil, "Inner: {{ .Input }}"),
		roleResource(t, "roles/gate", false, nil, "Gate: {{ .Input }}"),
		compositeResource(t, "workflows/inner", agent.LoopKind, []agent.Child{
			{Ref: "roles/inner-worker", Alias: "inner_worker", CanEscalate: true},
		}, 2, "{{ .Input }}", "inner={{ .State.outputs.inner_worker }}"),
		compositeResource(t, "workflows/outer", agent.LoopKind, []agent.Child{
			{Ref: "workflows/inner", Alias: "inner_loop"},
			{Ref: "roles/gate", Alias: "gate", CanEscalate: true},
		}, 3, "{{ .Input }}", "outer={{ .State.outputs.gate }}"),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/inner-worker": {
			{"draft-one\n\n" + controlEscalate},
			{"draft-two\n\n" + controlEscalate},
		},
		"roles/gate": {
			{"continue"},
			{"done\n\n" + controlEscalate},
		},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "iterate")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "outer=done" {
		t.Errorf("Runner.Run() = %q, want outer=done", got)
	}

	if process.sessions != 4 {
		t.Errorf("provider sessions = %d, want 4", process.sessions)
	}

	gatePrompts := process.prompts["roles/gate"]
	if len(gatePrompts) != 2 {
		t.Fatalf("gate prompts = %d, want 2", len(gatePrompts))
	}

	for index, want := range []string{"Gate: inner=draft-one", "Gate: inner=draft-two"} {
		if !strings.Contains(gatePrompts[index], want) {
			t.Errorf("gate prompt %d = %q, want containing %q", index, gatePrompts[index], want)
		}
	}
}

func TestRunnerFailureOverridesStickyEscalationInsideLoop(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/evaluator", false, nil, "{{ .Input }}"),
		roleResource(t, "roles/validator", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/phase", agent.SequentialKind, []agent.Child{
			{Ref: "roles/evaluator", Alias: "evaluator", CanEscalate: true},
			{Ref: "roles/validator", Alias: "validator", CanEscalate: true},
		}, 0, "{{ .Input }}", ""),
		compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
			{Ref: "workflows/phase", Alias: "phase", CanEscalate: true},
		}, 3, "{{ .Input }}", ""),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/evaluator": {{"candidate\n\n" + controlEscalate}},
		"roles/validator": {{"invalid\n\n" + controlFail}},
	}}

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "evaluate")
	if err == nil || !strings.Contains(err.Error(), `agent "validator" failed: invalid`) {
		t.Fatalf("Runner.Run() error = %v, want validator failure", err)
	}

	if len(process.prompts["roles/validator"]) != 1 {
		t.Errorf("validator turns = %d, want 1", len(process.prompts["roles/validator"]))
	}

	for _, roleID := range []string{"roles/evaluator", "roles/validator"} {
		if prompt := process.prompts[roleID][0]; !strings.Contains(prompt, controlEscalate) {
			t.Errorf("%s prompt does not preserve Loop escalation capability:\n%s", roleID, prompt)
		}
	}
}

func TestRunnerREPLRoleCanEscalateOnlyInsideLoop(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/reviewer", true, nil, "Review: {{ .Input }}"),
		compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
			{Ref: "roles/reviewer", Alias: "reviewer", CanEscalate: true},
		}, 2, "{{ .Input }}", "{{ .State.outputs.reviewer }}"),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/reviewer": {{"approved\n\n" + controlEscalate}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "review")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "approved" {
		t.Errorf("Runner.Run() = %q, want approved", got)
	}

	prompt := process.prompts["roles/reviewer"][0]
	for _, want := range []string{controlAwait, controlReturn, controlEscalate, controlFail} {
		if !strings.Contains(prompt, want) {
			t.Errorf("reviewer REPL prompt is missing %q:\n%s", want, prompt)
		}
	}
}

func TestRunnerRejectsUnauthorizedLoopEscalationForOneShotAndREPLRoles(t *testing.T) {
	t.Parallel()

	for _, repl := range []bool{false, true} {
		t.Run(fmt.Sprintf("repl=%t", repl), func(t *testing.T) {
			t.Parallel()

			root := resolvedRoot(t,
				roleResource(t, "roles/worker", repl, nil, "{{ .Input }}"),
				compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
					{Ref: "roles/worker", Alias: "worker"},
				}, 2, "{{ .Input }}", ""),
			)
			process := &scriptedProcess{visits: map[string][][]string{
				"roles/worker": {{"done\n\n" + controlEscalate}},
			}}

			_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "work")
			if err == nil || !strings.Contains(err.Error(), "attempted unauthorized escalation") {
				t.Fatalf("Runner.Run() error = %v, want unauthorized escalation", err)
			}

			prompt := process.prompts["roles/worker"][0]
			if strings.Contains(prompt, controlEscalate) {
				t.Errorf("worker prompt unexpectedly includes escalation control:\n%s", prompt)
			}
		})
	}
}

func TestRunnerRejectsUnauthorizedEscalationOutsideLoop(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "attempted unauthorized escalation") {
		t.Fatalf("Runner.Run() error = %v, want unauthorized escalation", err)
	}

	for _, want := range []string{`resource "roles/planner"`, `path "workflows/pipeline -> planner"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Runner.Run() error = %v, want containing %q", err, want)
		}
	}

	if len(process.prompts["roles/implementer"]) != 0 {
		t.Errorf("implementer turns = %d, want 0", len(process.prompts["roles/implementer"]))
	}

	if prompt := process.prompts["roles/planner"][0]; strings.Contains(prompt, controlEscalate) {
		t.Errorf("planner prompt unexpectedly allows escalation without a Loop ancestor:\n%s", prompt)
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

	if prompt := process.prompts["roles/planner"][0]; strings.Contains(prompt, controlEscalate) {
		t.Errorf("root REPL prompt unexpectedly allows escalation:\n%s", prompt)
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
	usages                    map[string][][]*runtime.TokenUsage
	configurations            map[string]runtime.SessionConfiguration
	prepareConfigurations     map[string]runtime.SessionConfiguration
	releaseTurns              chan struct{}
	turnStarted               chan struct{}
	prompts                   map[string][]string
	processContext            context.Context
	processContextErrAtClose  error
	requireLiveProcessContext bool
	sessions                  int
	effectiveIDs              []string
	prepareErr                error
	turnErr                   error
	closeErr                  error
}

func (p *scriptedProcess) NewSession(_ context.Context, role agent.Resource, effectiveID string) (runtime.AgentSession, error) {
	if p.requireLiveProcessContext && p.processContext.Err() != nil {
		return nil, fmt.Errorf("provider process context canceled before session creation: %w", p.processContext.Err())
	}

	visits := p.visits[role.ID]
	if len(visits) == 0 {
		return nil, fmt.Errorf("no scripted visit for %s", role.ID)
	}

	p.visits[role.ID] = visits[1:]
	p.sessions++
	p.effectiveIDs = append(p.effectiveIDs, effectiveID)

	var usages []*runtime.TokenUsage

	usageVisits := p.usages[role.ID]
	if len(usageVisits) > 0 {
		p.usages[role.ID] = usageVisits[1:]
		usages = append([]*runtime.TokenUsage(nil), usageVisits[0]...)
	}

	return &scriptedSession{
		roleID:               role.ID,
		responses:            append([]string(nil), visits[0]...),
		usages:               usages,
		configuration:        runtime.SessionConfiguration{Model: role.Spec.Provider.Model, Reasoning: role.Spec.Provider.Reasoning},
		prepareConfiguration: p.prepareConfigurations[role.ID],
		turnConfiguration:    p.configurations[role.ID],
		process:              p,
	}, nil
}

func (p *scriptedProcess) Close() error {
	if p.processContext != nil {
		p.processContextErrAtClose = p.processContext.Err()
	}

	return p.closeErr
}

type scriptedSession struct {
	roleID               string
	responses            []string
	usages               []*runtime.TokenUsage
	configuration        runtime.SessionConfiguration
	prepareConfiguration runtime.SessionConfiguration
	turnConfiguration    runtime.SessionConfiguration
	process              *scriptedProcess
	prepared             bool
	prepareCtx           context.Context
}

func (s *scriptedSession) Prepare(ctx context.Context) error {
	if _, ok := ctx.Deadline(); !ok {
		return fmt.Errorf("prepare context for %s has no deadline", s.roleID)
	}

	if s.prepareConfiguration != (runtime.SessionConfiguration{}) {
		s.configuration = s.prepareConfiguration
	}

	if s.process.prepareErr != nil {
		return s.process.prepareErr
	}

	s.prepared = true
	s.prepareCtx = ctx

	return nil
}

func (s *scriptedSession) Configuration() runtime.SessionConfiguration {
	return s.configuration
}

func (s *scriptedSession) Turn(ctx context.Context, prompt string) (runtime.TurnResult, error) {
	if !s.prepared {
		return runtime.TurnResult{}, fmt.Errorf("session for %s was not prepared", s.roleID)
	}

	if !errors.Is(s.prepareCtx.Err(), context.Canceled) {
		return runtime.TurnResult{}, fmt.Errorf("prepare budget for %s remained active during turn", s.roleID)
	}

	if len(s.responses) == 0 {
		return runtime.TurnResult{}, fmt.Errorf("no scripted response for %s", s.roleID)
	}

	if s.process.prompts == nil {
		s.process.prompts = make(map[string][]string)
	}

	s.process.prompts[s.roleID] = append(s.process.prompts[s.roleID], prompt)

	if s.process.turnStarted != nil {
		s.process.turnStarted <- struct{}{}
	}

	if s.process.releaseTurns != nil {
		select {
		case <-s.process.releaseTurns:
		case <-ctx.Done():
			return runtime.TurnResult{}, ctx.Err()
		}
	}

	if s.process.turnErr != nil {
		return runtime.TurnResult{}, s.process.turnErr
	}

	response := s.responses[0]
	s.responses = s.responses[1:]

	var usage *runtime.TokenUsage
	if len(s.usages) > 0 {
		usage = s.usages[0]
		s.usages = s.usages[1:]
	}

	if s.turnConfiguration != (runtime.SessionConfiguration{}) {
		s.configuration = s.turnConfiguration
	}

	return runtime.TurnResult{Content: response, Usage: usage}, nil
}

type scriptedInteractor struct {
	answers       []string
	labels        []string
	displayed     []string
	waitPerPrompt time.Duration
	waited        time.Duration
}

func (i *scriptedInteractor) Prompt(_ context.Context, label string) (string, error) {
	i.labels = append(i.labels, label)
	i.waited += i.waitPerPrompt

	if len(i.answers) == 0 {
		return "", fmt.Errorf("no scripted answer for %s", label)
	}

	answer := i.answers[0]
	i.answers = i.answers[1:]

	return answer, nil
}

func (i *scriptedInteractor) WaitDuration() time.Duration {
	return i.waited
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
			Interactive: &replValue,
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
