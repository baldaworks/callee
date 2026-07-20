package workflow

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
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

func TestRunnerLogsStatefulSessionCreationAndReuseAtDebug(t *testing.T) {
	t.Parallel()

	loop := compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
		{
			Ref:         "roles/worker",
			Alias:       "worker",
			CanEscalate: true,
			Session:     agent.SessionModeStateful,
		},
	}, 2, "{{ .Input }}", "")
	root := resolvedRoot(t,
		roleResource(t, "roles/worker", false, nil, "{{ .Input }}"),
		loop,
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/worker": {{"first", "second\n\n" + controlEscalate}},
	}}

	var output bytes.Buffer

	logger := zerolog.New(&output)
	ctx := logger.WithContext(context.Background())

	if _, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(ctx, "task"); err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	var sessionEvents []map[string]any

	for _, event := range decodeLifecycleEvents(t, &output) {
		if event["message"] == "agent session created" || event["message"] == "agent session reused" {
			sessionEvents = append(sessionEvents, event)
		}
	}

	if len(sessionEvents) != 2 {
		t.Fatalf("session events = %#v, want created and reused", sessionEvents)
	}

	for index, message := range []string{"agent session created", "agent session reused"} {
		event := sessionEvents[index]
		for field, want := range map[string]any{
			"level":   "debug",
			"message": message,
			"id":      "worker",
			"session": "stateful",
			"loop":    "workflows/loop",
		} {
			if event[field] != want {
				t.Errorf("session event %d field %s = %#v, want %#v; event=%#v", index, field, event[field], want, event)
			}
		}
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
	interactor := &scriptedInteractor{answers: append([]string(nil), test.answers...)}

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
