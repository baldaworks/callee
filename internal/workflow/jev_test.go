package workflow

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	jevapi "github.com/baldaworks/callee/internal/jev"
	"github.com/rs/zerolog"
)

type fakeJevCall struct {
	api     agent.JevAPI
	request jevapi.Request
}

type fakeJevResponse struct {
	result jevapi.Result
	trace  jevapi.Trace
	err    error
}

type fakeJevEvaluator struct {
	calls     []fakeJevCall
	responses []fakeJevResponse
}

func (f *fakeJevEvaluator) Evaluate(_ context.Context, api agent.JevAPI, request jevapi.Request) (jevapi.Result, jevapi.Trace, error) {
	f.calls = append(f.calls, fakeJevCall{api: api, request: request})
	if len(f.responses) == 0 {
		return jevapi.Result{}, jevapi.Trace{}, errors.New("no fake Jev response")
	}

	response := f.responses[0]
	f.responses = f.responses[1:]

	return response.result, response.trace, response.err
}

func TestRunnerExecutesRootJevWithoutProviderProcessAndLogsSafeTrace(t *testing.T) {
	t.Parallel()

	cost := 0.0042
	answer := 0.87
	evaluator := &fakeJevEvaluator{responses: []fakeJevResponse{{
		result: jevapi.Result{
			API:            "openrouter",
			Provider:       "TypeSafe",
			RequestID:      "request-42",
			RequestedModel: "~typesafe/jev-latest",
			Model:          "typesafe/jev-1.13",
			Answers:        map[string]jevapi.Answer{"urgent": {Type: "noul", Noul: &answer}},
			Usage:          &jevapi.Usage{InputTokens: 21, OutputTokens: 5, Cost: &cost},
		},
		trace: jevapi.Trace{
			API:            "openrouter",
			Provider:       "TypeSafe",
			RequestID:      "request-42",
			RequestedModel: "~typesafe/jev-latest",
			Model:          "typesafe/jev-1.13",
			Attempts:       2,
			Usage:          &jevapi.Usage{InputTokens: 21, OutputTokens: 5, Cost: &cost},
		},
	}}}
	resource := jevResource(t, "judges/urgent", "openrouter", map[string]any{
		"prompt":  "{{ .Prompt }}",
		"input":   "{{ .Input }}",
		"tenant":  "{{ .State.tenant }}",
		"enabled": true,
		"count":   3,
		"missing": nil,
	})
	resource.Spec.State = map[string]any{"tenant": "tenant-for-{{ .Input }}"}
	resource.Spec.Questions["urgent"] = agent.JevQuestion{
		Type: "noul",
		Instructions: map[string]any{
			"rule":    "Judge {{ .State.tenant }}",
			"enabled": true,
		},
	}
	root := resolvedRoot(t, resource)
	factory := &scriptedFactory{process: &scriptedProcess{}}
	metrics := &RunMetrics{}

	var logs bytes.Buffer

	ctx := zerolog.New(&logs).Level(zerolog.InfoLevel).WithContext(context.Background())

	artifact, err := (Runner{
		Root:         root,
		Factory:      factory,
		JevEvaluator: evaluator,
		Metrics:      metrics,
	}).Run(ctx, "triage this")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if factory.starts != 0 {
		t.Fatalf("provider process starts = %d, want 0", factory.starts)
	}

	if len(evaluator.calls) != 1 {
		t.Fatalf("Jev calls = %d, want 1", len(evaluator.calls))
	}

	wantState := map[string]any{
		"prompt":  "triage this",
		"input":   "triage this",
		"tenant":  "tenant-for-triage this",
		"enabled": true,
		"count":   3,
		"missing": nil,
	}
	if !reflect.DeepEqual(evaluator.calls[0].request.State, wantState) {
		t.Errorf("request state = %#v, want %#v", evaluator.calls[0].request.State, wantState)
	}

	wantInstructions := map[string]any{"rule": "Judge tenant-for-triage this", "enabled": true}
	if got := evaluator.calls[0].request.Questions["urgent"].Instructions; !reflect.DeepEqual(got, wantInstructions) {
		t.Errorf("instructions = %#v, want %#v", got, wantInstructions)
	}

	if !strings.Contains(artifact, `"noul":0.87`) || !strings.Contains(artifact, `"model":"typesafe/jev-1.13"`) {
		t.Errorf("artifact = %s, want canonical result", artifact)
	}

	if got := metrics.Usage(); got.InputTokens != 0 || got.OutputTokens != 0 || got.TotalTokens != 0 {
		t.Errorf("run metrics = %#v, want unchanged zero role metrics", got)
	}

	events := decodeLifecycleEvents(t, &logs)
	if len(events) != 2 {
		t.Fatalf("lifecycle events = %#v, want start and finish", events)
	}

	finished := events[1]
	for field, want := range map[string]any{
		"status":              "completed",
		"outcome":             "return",
		"jev_api":             "openrouter",
		"jev_requested_model": "~typesafe/jev-latest",
		"jev_attempts":        float64(2),
		"jev_model":           "typesafe/jev-1.13",
		"jev_provider":        "TypeSafe",
		"jev_request_id":      "request-42",
		"jev_input_tokens":    float64(21),
		"jev_output_tokens":   float64(5),
		"jev_cost":            cost,
	} {
		if finished[field] != want {
			t.Errorf("finish field %s = %#v, want %#v", field, finished[field], want)
		}
	}

	for _, forbidden := range []string{"evidence", "questions", "answers", "api_key", "authorization"} {
		if _, ok := finished[forbidden]; ok {
			t.Errorf("finish event exposes %q: %#v", forbidden, finished)
		}
	}
}

func TestRunnerSequentialMakesJevEvaluationAvailableToFollowingRole(t *testing.T) {
	t.Parallel()

	answer := 0.8
	evaluator := &fakeJevEvaluator{responses: []fakeJevResponse{{result: jevapi.Result{
		API:            "typesafe",
		RequestedModel: "jev-latest",
		Model:          "jev-1.13",
		Answers:        map[string]jevapi.Answer{"urgent": {Type: "noul", Noul: &answer}},
	}}}}
	judge := jevResource(t, "judges/urgent", "typesafe", "{{ .Input }}")
	reader := roleResource(t, "roles/reader", false, nil, `{{ .Input }} score={{ .State.evaluations.judge.answers.urgent.noul }} artifact={{ .State.outputs.judge }}`)
	pipeline := compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
		{Ref: judge.ID, Alias: "judge"},
		{Ref: reader.ID, Alias: "reader"},
	}, 0, "{{ .Input }}", "{{ .State.outputs.reader }}")
	root := resolvedRoot(t, judge, reader, pipeline)
	process := &scriptedProcess{visits: map[string][][]string{reader.ID: {{"consumed"}}}}
	factory := &scriptedFactory{process: process}

	artifact, err := (Runner{Root: root, Factory: factory, JevEvaluator: evaluator}).Run(context.Background(), "task")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if artifact != "consumed" {
		t.Errorf("artifact = %q, want consumed", artifact)
	}

	if len(evaluator.calls) != 1 || factory.starts != 1 {
		t.Fatalf("calls = %d, provider starts = %d; want 1 each", len(evaluator.calls), factory.starts)
	}

	prompt := process.prompts[reader.ID][0]
	if !strings.Contains(prompt, "score=0.8") || !strings.Contains(prompt, `"answers":{"urgent"`) {
		t.Errorf("reader prompt did not consume structured evaluation and artifact:\n%s", prompt)
	}
}

func TestRunnerLogsClassifiedJevFailureWithoutErrorPayload(t *testing.T) {
	t.Parallel()

	evaluator := &fakeJevEvaluator{responses: []fakeJevResponse{{
		trace: jevapi.Trace{Attempts: 3, ErrorClass: jevapi.ErrorResponse},
		err:   errors.New("super-secret evidence and answer"),
	}}}
	root := resolvedRoot(t, jevResource(t, "judges/urgent", "typesafe", "{{ .Input }}"))

	var logs bytes.Buffer

	ctx := zerolog.New(&logs).Level(zerolog.InfoLevel).WithContext(context.Background())

	_, err := (Runner{
		Root:         root,
		Factory:      &scriptedFactory{process: &scriptedProcess{}},
		JevEvaluator: evaluator,
	}).Run(ctx, "private input")
	if err == nil {
		t.Fatal("Runner.Run() error = nil, want evaluator failure")
	}

	if strings.Contains(logs.String(), "super-secret") || strings.Contains(logs.String(), "private input") {
		t.Fatalf("lifecycle logs expose request or error payload: %s", logs.String())
	}

	events := decodeLifecycleEvents(t, &logs)
	if len(events) != 2 {
		t.Fatalf("lifecycle events = %#v, want start and finish", events)
	}

	finished := events[1]
	for field, want := range map[string]any{
		"status":              "error",
		"jev_api":             "typesafe",
		"jev_requested_model": "jev-latest",
		"jev_attempts":        float64(3),
		"jev_error_class":     "response",
	} {
		if finished[field] != want {
			t.Errorf("finish field %s = %#v, want %#v", field, finished[field], want)
		}
	}
}

func TestJevPublishesOnlyCompleteSuccessfulResultsAndLastSuccessWins(t *testing.T) {
	t.Parallel()

	first := 0.2
	second := 0.9
	evaluator := &fakeJevEvaluator{responses: []fakeJevResponse{
		{result: jevResult(first)},
		{result: jevResult(second)},
		{trace: jevapi.Trace{Attempts: 3, ErrorClass: jevapi.ErrorResponse}, err: errors.New("secret evidence and answer")},
	}}
	node := resolvedRoot(t, jevResource(t, "judges/urgent", "typesafe", "{{ .Input }}"))
	run := &runState{
		prompt:       "task",
		state:        map[string]any{"outputs": map[string]string{}, "scripts": map[string]any{}, "evaluations": map[string]any{}},
		jevEvaluator: evaluator,
	}

	for _, input := range []string{"first", "second"} {
		if _, err := run.jev(context.Background(), node, input); err != nil {
			t.Fatalf("jev(%q) error: %v", input, err)
		}
	}

	outputs := run.state["outputs"].(map[string]string)
	beforeArtifact := outputs[node.EffectiveID]
	beforeEvaluation := run.state["evaluations"].(map[string]any)[node.EffectiveID]

	if !strings.Contains(beforeArtifact, `"noul":0.9`) {
		t.Fatalf("last successful artifact = %s, want second result", beforeArtifact)
	}

	result, err := run.jev(context.Background(), node, "third")
	if err == nil {
		t.Fatal("jev(third) error = nil, want evaluator failure")
	}

	if result.jevTrace.API != "typesafe" || result.jevTrace.RequestedModel != "jev-latest" || result.jevTrace.ErrorClass != jevapi.ErrorResponse {
		t.Errorf("failure trace = %#v, want completed safe trace", result.jevTrace)
	}

	if outputs[node.EffectiveID] != beforeArtifact || !reflect.DeepEqual(run.state["evaluations"].(map[string]any)[node.EffectiveID], beforeEvaluation) {
		t.Fatal("failed Jev visit changed the last successful publication")
	}
}

func jevResource(t *testing.T, id, apiType string, evidence any) agent.Resource {
	t.Helper()

	resource := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.JevKind,
		ID:         id,
		Source:     id + ".md",
		Spec: agent.Spec{
			Description: id,
			API:         &agent.JevAPI{Type: apiType},
			Questions: map[string]agent.JevQuestion{
				"urgent": {Type: "noul", Instructions: "Is this urgent?"},
			},
		},
	}
	if body, ok := evidence.(string); ok {
		resource.Spec.Body = body
	} else {
		resource.Spec.Evidence = evidence
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("Jev resource %q validation error: %v", id, err)
	}

	return resource
}

func jevResult(value float64) jevapi.Result {
	return jevapi.Result{
		API:            "typesafe",
		RequestedModel: "jev-latest",
		Model:          "jev-1.13",
		Answers:        map[string]jevapi.Answer{"urgent": {Type: "noul", Noul: &value}},
	}
}
