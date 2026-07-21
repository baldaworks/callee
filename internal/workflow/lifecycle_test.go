package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
)

func TestRunnerLogsAgentLifecycleAcrossLoopVisits(t *testing.T) {
	t.Parallel()

	loop := compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
		{Ref: "roles/worker", Alias: "worker", CanEscalate: true},
	}, 5, "{{ .Input }}", "")
	loop.Spec.OnExhausted = "complete"

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
		loop,
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker": {{"first"}, {"second\n\n" + controlEscalate}},
	}}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "task")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "second" {
		t.Errorf("Runner.Run() = %q, want second", got)
	}

	events := decodeLifecycleEvents(t, &output)
	want := []struct {
		message string
		id      string
		kind    string
		ref     string
		status  string
		outcome string
		visit   float64
	}{
		{message: "running agent", id: "workflows/loop", kind: "Loop", visit: 1},
		{message: "running agent", id: "worker", kind: "Role", ref: "roles/worker", visit: 1},
		{message: "agent finished", id: "worker", kind: "Role", ref: "roles/worker", status: "completed", outcome: "return", visit: 1},
		{message: "running agent", id: "worker", kind: "Role", ref: "roles/worker", visit: 2},
		{message: "agent finished", id: "worker", kind: "Role", ref: "roles/worker", status: "completed", outcome: "escalate", visit: 2},
		{message: "agent finished", id: "workflows/loop", kind: "Loop", status: "completed", outcome: "return", visit: 1},
	}

	if len(events) != len(want) {
		t.Fatalf("lifecycle events = %#v, want %d events", events, len(want))
	}

	for index, expected := range want {
		event := events[index]

		for field, value := range map[string]any{
			"level":   "info",
			"message": expected.message,
			"id":      expected.id,
			"kind":    expected.kind,
			"visit":   expected.visit,
		} {
			if event[field] != value {
				t.Errorf("event %d field %s = %#v, want %#v; event=%#v", index, field, event[field], value, event)
			}
		}

		checkOptionalLifecycleField(t, index, event, "ref", expected.ref)
		checkOptionalLifecycleField(t, index, event, "status", expected.status)
		checkOptionalLifecycleField(t, index, event, "outcome", expected.outcome)

		_, hasDuration := event["duration"]
		if hasDuration != (expected.message == "agent finished") {
			t.Errorf("event %d duration presence = %t, want %t; event=%#v", index, hasDuration, expected.message == "agent finished", event)
		}

		for _, forbidden := range []string{"prompt", "input", "output", "params", "state", "artifact"} {
			if _, ok := event[forbidden]; ok {
				t.Errorf("event %d contains forbidden field %q: %#v", index, forbidden, event)
			}
		}
	}
}

func TestRunnerUsesIdenticalRoleMetricsForDirectAndNestedRoles(t *testing.T) {
	t.Parallel()

	usage := &runtime.TokenUsage{InputTokens: 11, OutputTokens: 3, TotalTokens: 17, CachedReadTokens: 4}
	tests := []struct {
		name      string
		resources func(*testing.T) []agent.Resource
	}{
		{
			name: "direct",
			resources: func(t *testing.T) []agent.Resource {
				return []agent.Resource{metricsRoleResource(t)}
			},
		},
		{
			name: "nested",
			resources: func(t *testing.T) []agent.Resource {
				return []agent.Resource{
					metricsRoleResource(t),
					compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{{Ref: "roles/worker", Alias: "worker"}}, 0, "{{ .Input }}", ""),
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			assertRoleMetrics(t, test.resources(t), usage)
		})
	}
}

func metricsRoleResource(t *testing.T) agent.Resource {
	t.Helper()

	role := roleResource(t, "roles/worker", false, nil, "{{ .Input }}")
	role.Spec.Provider.Model = "gpt-5.6-sol"
	role.Spec.Provider.Reasoning = "high"

	return role
}

func assertRoleMetrics(t *testing.T, resources []agent.Resource, usage *runtime.TokenUsage) {
	t.Helper()

	root := resolvedRoot(t, resources...)
	process := &scriptedProcess{
		visits:         map[string][][]string{"roles/worker": {{"done"}}},
		usages:         map[string][][]*runtime.TokenUsage{"roles/worker": {{usage}}},
		configurations: map[string]runtime.SessionConfiguration{"roles/worker": {Model: "provider-model", Reasoning: "medium"}},
	}
	metrics := &RunMetrics{}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}, Metrics: metrics}).Run(ctx, "task"); err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	var finished map[string]any

	for _, event := range decodeLifecycleEvents(t, &output) {
		if event["message"] == "agent finished" && event["kind"] == "Role" {
			finished = event
		}

		if event["kind"] != "Role" {
			assertNoRoleMetrics(t, event)
		}
	}

	if finished == nil {
		t.Fatal("missing Role finish event")
	}

	for field, want := range map[string]any{
		"role_provider":           "codex",
		"role_model":              "provider-model",
		"role_reasoning":          "medium",
		"role_token_usage":        "complete",
		"role_input_tokens":       float64(11),
		"role_output_tokens":      float64(3),
		"role_total_tokens":       float64(17),
		"role_cached_read_tokens": float64(4),
	} {
		if finished[field] != want {
			t.Errorf("%s = %#v, want %#v; event=%#v", field, finished[field], want, finished)
		}
	}

	for _, field := range []string{"role_duration", "role_wait_duration"} {
		if _, ok := finished[field]; !ok {
			t.Errorf("Role finish event has no %s: %#v", field, finished)
		}
	}

	runUsage := metrics.Usage()
	if runUsage.Status() != runtime.TokenUsageComplete || runUsage.TokenUsage != *usage {
		t.Errorf("run usage = %+v, want complete %+v", runUsage, *usage)
	}
}

func assertNoRoleMetrics(t *testing.T, event map[string]any) {
	t.Helper()

	for field := range event {
		if strings.HasPrefix(field, "role_") {
			t.Errorf("composite event contains %q: %#v", field, event)
		}
	}
}

func TestRunnerAggregatesPartialUsageAcrossRoles(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
		roleResource(t, "roles/reviewer", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
			{Ref: "roles/worker", Alias: "worker"},
			{Ref: "roles/reviewer", Alias: "reviewer"},
		}, 0, "{{ .Input }}", ""),
	)
	usage := &runtime.TokenUsage{InputTokens: 5, OutputTokens: 2, TotalTokens: 7}
	process := &scriptedProcess{
		visits: map[string][][]string{
			"roles/worker":   {{"draft"}},
			"roles/reviewer": {{"approved"}},
		},
		usages: map[string][][]*runtime.TokenUsage{
			"roles/worker": {{usage}},
		},
	}
	metrics := &RunMetrics{}

	if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}, Metrics: metrics}).Run(context.Background(), "task"); err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	got := metrics.Usage()
	if got.Status() != runtime.TokenUsagePartial || got.TokenUsage != *usage || got.TurnsAttempted != 2 || got.TurnsReported != 1 {
		t.Errorf("run usage = %+v, want one of two turns reported", got)
	}
}

func TestRunnerLogsRoleProviderFieldsBeforeExecutionFailure(t *testing.T) {
	t.Parallel()

	worker := metricsRoleResource(t)
	worker.Spec.State = map[string]any{"failure": `{{ fail "state failed" }}`}
	root := resolvedRoot(t, worker)

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: &scriptedProcess{}}}).Run(ctx, "task"); err == nil {
		t.Fatal("Runner.Run() error = nil, want state failure")
	}

	events := decodeLifecycleEvents(t, &output)
	finished := events[len(events)-1]

	for field, want := range map[string]any{
		"role_provider":    "codex",
		"role_model":       "gpt-5.6-sol",
		"role_reasoning":   "high",
		"role_token_usage": "unavailable",
	} {
		if finished[field] != want {
			t.Errorf("%s = %#v, want %#v; event=%#v", field, finished[field], want, finished)
		}
	}

	for _, field := range []string{"role_duration", "role_wait_duration"} {
		if _, ok := finished[field]; ok {
			t.Errorf("pre-execution failure contains %s: %#v", field, finished)
		}
	}
}

func TestRunnerLogsObservedRoleConfigurationAfterPrepareFailure(t *testing.T) {
	t.Parallel()

	worker := metricsRoleResource(t)
	root := resolvedRoot(t, worker)
	process := &scriptedProcess{
		visits:                map[string][][]string{"roles/worker": {{"unused"}}},
		prepareErr:            context.Canceled,
		prepareConfigurations: map[string]runtime.SessionConfiguration{"roles/worker": {Model: "observed-model", Reasoning: "medium"}},
	}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "task"); err == nil {
		t.Fatal("Runner.Run() error = nil, want prepare failure")
	}

	events := decodeLifecycleEvents(t, &output)
	finished := events[len(events)-1]

	for field, want := range map[string]any{
		"role_provider":    "codex",
		"role_model":       "observed-model",
		"role_reasoning":   "medium",
		"role_token_usage": "unavailable",
	} {
		if finished[field] != want {
			t.Errorf("%s = %#v, want %#v; event=%#v", field, finished[field], want, finished)
		}
	}

	if _, ok := finished["role_duration"]; ok {
		t.Errorf("prepare failure contains role_duration: %#v", finished)
	}
}

type replLifecycleTest struct {
	name        string
	repl        bool
	responses   []string
	answers     []string
	prepareErr  error
	wantErr     bool
	wantREPL    bool
	wantStatus  string
	wantOutcome string
}

func TestRunnerLogsREPLLifecycle(t *testing.T) {
	t.Parallel()

	tests := []replLifecycleTest{
		{
			name:        "immediate return",
			repl:        true,
			responses:   []string{"done\n\n" + controlReturn},
			wantREPL:    true,
			wantStatus:  "completed",
			wantOutcome: "return",
		},
		{
			name: "multiple awaits",
			repl: true,
			responses: []string{
				"First question?\n\n" + controlAwait,
				"Second question?\n\n" + controlAwait,
				"done\n\n" + controlReturn,
			},
			answers:     []string{"first answer", "second answer"},
			wantREPL:    true,
			wantStatus:  "completed",
			wantOutcome: "return",
		},
		{
			name:        "declared failure",
			repl:        true,
			responses:   []string{"validation failed\n\n" + controlFail},
			wantErr:     true,
			wantREPL:    true,
			wantStatus:  "failed",
			wantOutcome: "fail",
		},
		{
			name:       "turn error",
			repl:       true,
			responses:  []string{},
			wantErr:    true,
			wantREPL:   true,
			wantStatus: "error",
		},
		{
			name:       "prepare error",
			repl:       true,
			responses:  []string{"unused"},
			prepareErr: context.Canceled,
			wantErr:    true,
		},
		{
			name:        "non-REPL",
			responses:   []string{"done"},
			wantStatus:  "completed",
			wantOutcome: "return",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			runREPLLifecycleTest(t, test)
		})
	}
}

func runREPLLifecycleTest(t *testing.T, test replLifecycleTest) {
	t.Helper()

	root := resolvedRoot(t, roleResource(t, "roles/planner", test.repl, nil, "{{ .Input }}"))
	process := &scriptedProcess{
		visits: map[string][][]string{
			"roles/planner": {test.responses},
		},
		prepareErr: test.prepareErr,
	}
	interactor := &scriptedInteractor{
		answers:       append([]string(nil), test.answers...),
		waitPerPrompt: time.Second,
	}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}, Interactor: interactor}).Run(ctx, "task")
	if (err != nil) != test.wantErr {
		t.Fatalf("Runner.Run() error = %v, want error %t", err, test.wantErr)
	}

	events := decodeLifecycleEvents(t, &output)
	wantMessages := []string{"running agent", "agent finished"}

	if test.wantREPL {
		wantMessages = []string{"running agent", "entering repl", "exiting repl", "agent finished"}
	}

	if len(events) != len(wantMessages) {
		t.Fatalf("lifecycle events = %#v, want messages %v", events, wantMessages)
	}

	for index, message := range wantMessages {
		event := events[index]

		if event["message"] != message || event["id"] != "roles/planner" || event["kind"] != "Role" || event["visit"] != float64(1) {
			t.Errorf("event %d = %#v, want message=%q id=roles/planner kind=Role visit=1", index, event, message)
		}
	}

	finished := events[len(events)-1]
	assertUnavailableRoleMetrics(t, finished, test)

	if !test.wantREPL {
		return
	}

	entering := events[1]

	for _, field := range []string{"duration", "status", "outcome"} {
		if _, ok := entering[field]; ok {
			t.Errorf("entering repl event contains %q: %#v", field, entering)
		}
	}

	exiting := events[2]

	if exiting["status"] != test.wantStatus {
		t.Errorf("exiting repl status = %#v, want %q; event=%#v", exiting["status"], test.wantStatus, exiting)
	}

	checkOptionalLifecycleField(t, 2, exiting, "outcome", test.wantOutcome)

	if _, ok := exiting["duration"]; !ok {
		t.Errorf("exiting repl event has no duration: %#v", exiting)
	}

	for field := range exiting {
		if strings.HasPrefix(field, "role_") {
			t.Errorf("exiting repl event contains %q: %#v", field, exiting)
		}
	}
}

func assertUnavailableRoleMetrics(t *testing.T, finished map[string]any, test replLifecycleTest) {
	t.Helper()

	for field, want := range map[string]any{
		"role_provider":  "codex",
		"role_model":     "backend-default",
		"role_reasoning": "backend-default",
	} {
		if finished[field] != want {
			t.Errorf("%s = %#v, want %#v; event=%#v", field, finished[field], want, finished)
		}
	}

	if finished["role_token_usage"] != "unavailable" {
		t.Errorf("Role token usage = %#v, want unavailable; event=%#v", finished["role_token_usage"], finished)
	}

	_, hasRoleDuration := finished["role_duration"]
	_, hasRoleWait := finished["role_wait_duration"]
	wantRoleTiming := test.prepareErr == nil

	if hasRoleDuration != wantRoleTiming || hasRoleWait != wantRoleTiming {
		t.Errorf("Role timing presence = duration:%t wait:%t, want %t; event=%#v", hasRoleDuration, hasRoleWait, wantRoleTiming, finished)
	}

	if wantRoleTiming {
		wantWait := len(test.answers) > 0
		if gotWait := !durationValueIsZero(finished["role_wait_duration"]); gotWait != wantWait {
			t.Errorf("Role wait nonzero = %t, want %t; event=%#v", gotWait, wantWait, finished)
		}
	}
}

func durationValueIsZero(value any) bool {
	switch typed := value.(type) {
	case float64:
		return typed == 0
	case string:
		return typed == "0s"
	default:
		return false
	}
}

func TestRunnerLogsAgentDeclaredFailure(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker": {{"validation failed\n\n" + controlFail}},
	}}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "task")
	if err == nil {
		t.Fatal("Runner.Run() error = nil, want agent failure")
	}

	events := decodeLifecycleEvents(t, &output)
	if len(events) != 2 {
		t.Fatalf("lifecycle events = %#v, want 2 events", events)
	}

	finished := events[1]
	if finished["status"] != "failed" || finished["outcome"] != "fail" {
		t.Errorf("finish event = %#v, want failed/fail", finished)
	}
}

func TestRunnerLogsRuntimeErrorWithoutDuplicatingDiagnostic(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "{{ .Input }}"))
	process := &scriptedProcess{visits: map[string][][]string{}}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "task")
	if err == nil {
		t.Fatal("Runner.Run() error = nil, want runtime error")
	}

	events := decodeLifecycleEvents(t, &output)
	if len(events) != 2 {
		t.Fatalf("lifecycle events = %#v, want 2 events", events)
	}

	finished := events[1]
	if finished["status"] != "error" {
		t.Errorf("finish event status = %#v, want error; event=%#v", finished["status"], finished)
	}

	for _, field := range []string{"outcome", "error"} {
		if _, ok := finished[field]; ok {
			t.Errorf("finish event contains %q, want final CLI diagnostic to own error detail: %#v", field, finished)
		}
	}
}

func decodeLifecycleEvents(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()

	var events []map[string]any

	decoder := json.NewDecoder(output)

	for decoder.More() {
		var event map[string]any
		if err := decoder.Decode(&event); err != nil {
			t.Fatalf("decode lifecycle event: %v", err)
		}

		events = append(events, event)
	}

	return events
}

func checkOptionalLifecycleField(t *testing.T, index int, event map[string]any, field, want string) {
	t.Helper()

	got, ok := event[field]

	if want == "" {
		if ok {
			t.Errorf("event %d field %s = %#v, want absent; event=%#v", index, field, got, event)
		}

		return
	}

	if !ok || got != want {
		t.Errorf("event %d field %s = %#v, want %q; event=%#v", index, field, got, want, event)
	}
}
