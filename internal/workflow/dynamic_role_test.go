package workflow

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
)

func TestRunnerMaterializesDynamicRoleProviderFromVisitData(t *testing.T) {
	t.Parallel()

	role := dynamicRoleResource(t, "roles/dynamic")
	root := resolvedRoot(t, role)
	process := &scriptedProcess{visits: map[string][][]string{role.ID: {{"done"}}}}
	factory := &scriptedFactory{process: process}

	got, err := (Runner{
		Root:    root,
		Factory: factory,
		Params:  map[string]string{"roles/dynamic.model": "model-v2"},
	}).Run(context.Background(), "review")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "done" {
		t.Errorf("Runner.Run() = %q, want done", got)
	}

	if factory.starts != 1 || process.sessions != 1 || len(process.roles) != 1 {
		t.Fatalf("starts/sessions/roles = %d/%d/%d, want 1/1/1", factory.starts, process.sessions, len(process.roles))
	}

	want := &agent.Provider{
		Type:      "generic_acp",
		Cmd:       "custom-agent",
		Model:     "model-v2",
		Reasoning: "high",
		Mode:      "review",
		ExtraArgs: []string{"--root=review"},
		Timeout:   "250ms",
	}
	if !reflect.DeepEqual(process.roles[0].Spec.Provider, want) {
		t.Errorf("session provider = %#v, want %#v", process.roles[0].Spec.Provider, want)
	}

	if role.Spec.Provider.Type != `{{ .State.provider }}` {
		t.Errorf("authored provider mutated: %#v", role.Spec.Provider)
	}
}

func TestRunnerDynamicRoleFailsBeforeProviderStart(t *testing.T) {
	t.Parallel()

	role := dynamicRoleResource(t, "roles/dynamic")
	role.Spec.State["provider"] = "unknown"
	root := resolvedRoot(t, role)
	factory := &scriptedFactory{process: &scriptedProcess{}}

	var output bytes.Buffer

	ctx := zerolog.New(&output).WithContext(context.Background())

	_, err := (Runner{
		Root:    root,
		Factory: factory,
		Params:  map[string]string{"roles/dynamic.model": "model-v2"},
	}).Run(ctx, "review")
	if err == nil || !strings.Contains(err.Error(), "unsupported spec.provider.type") {
		t.Fatalf("Runner.Run() error = %v, want unsupported provider", err)
	}

	if factory.starts != 0 {
		t.Errorf("provider starts = %d, want 0", factory.starts)
	}

	events := decodeLifecycleEvents(t, &output)

	finished := events[len(events)-1]
	if got := finished["role_token_usage"]; got != "unavailable" {
		t.Errorf("role_token_usage = %#v, want unavailable", got)
	}

	for _, field := range []string{"role_provider", "role_model", "role_reasoning"} {
		if _, ok := finished[field]; ok {
			t.Errorf("unresolved DynamicRole event contains %s: %#v", field, finished)
		}
	}
}

func TestRunnerRerendersDynamicRoleAndReusesOnlyMatchingProcesses(t *testing.T) {
	t.Parallel()

	role := dynamicRoleResource(t, "roles/dynamic")
	role.Spec.Params = nil
	role.Spec.Provider.Model = ""
	role.Spec.State["cmd"] = `{{ if index .State.outputs "worker" }}second-agent{{ else }}first-agent{{ end }}`
	loop := compositeResource(t, "workflows/loop", agent.LoopKind, []agent.Child{
		{Ref: role.ID, Alias: "worker", CanEscalate: true},
	}, 3, "{{ .Input }}", "")
	root := resolvedRoot(t, role, loop)
	process := &scriptedProcess{visits: map[string][][]string{
		role.ID: {{"first"}, {"second\n\n" + controlEscalate}},
	}}
	factory := &scriptedFactory{process: process}

	got, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "review")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "second" {
		t.Errorf("Runner.Run() = %q, want second", got)
	}

	if factory.starts != 2 || process.sessions != 2 {
		t.Errorf("starts/sessions = %d/%d, want 2/2", factory.starts, process.sessions)
	}

	gotCommands := []string{process.roles[0].Spec.Provider.Cmd, process.roles[1].Spec.Provider.Cmd}
	if want := []string{"first-agent", "second-agent"}; !reflect.DeepEqual(gotCommands, want) {
		t.Errorf("visit commands = %v, want %v", gotCommands, want)
	}
}

func TestRunnerDynamicRolesReuseEffectiveProviderWithFreshSessions(t *testing.T) {
	t.Parallel()

	first := dynamicRoleResource(t, "roles/first")
	first.Spec.Params = nil
	first.Spec.Provider.Model = "first-model"
	second := dynamicRoleResource(t, "roles/second")
	second.Spec.Params = nil
	second.Spec.Provider.Model = "second-model"
	rootResource := compositeResource(t, "workflows/pipeline", agent.SequentialKind, []agent.Child{
		{Ref: first.ID},
		{Ref: second.ID},
	}, 0, "{{ .Input }}", "")
	root := resolvedRoot(t, first, second, rootResource)
	process := &scriptedProcess{visits: map[string][][]string{
		first.ID:  {{"first"}},
		second.ID: {{"second"}},
	}}
	factory := &scriptedFactory{process: process}

	got, err := (Runner{Root: root, Factory: factory}).Run(context.Background(), "review")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "second" || factory.starts != 1 || process.sessions != 2 {
		t.Errorf("result/starts/sessions = %q/%d/%d, want second/1/2", got, factory.starts, process.sessions)
	}
}

func TestRunnerDynamicRoleMetricsUseEffectiveProvider(t *testing.T) {
	t.Parallel()

	role := dynamicRoleResource(t, "roles/dynamic")
	root := resolvedRoot(t, role)
	process := &scriptedProcess{visits: map[string][][]string{role.ID: {{"done"}}}}

	var output bytes.Buffer

	ctx := zerolog.New(&output).WithContext(context.Background())

	_, err := (Runner{
		Root:    root,
		Factory: &scriptedFactory{process: process},
		Params:  map[string]string{"roles/dynamic.model": "private-model-selection"},
	}).Run(ctx, "review")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	var finished map[string]any

	for _, event := range decodeLifecycleEvents(t, &output) {
		if event["message"] == "agent finished" && event["kind"] == "DynamicRole" {
			finished = event
		}
	}

	if finished == nil {
		t.Fatal("missing DynamicRole finish event")
	}

	for field, want := range map[string]any{
		"role_provider":    "generic_acp",
		"role_model":       "private-model-selection",
		"role_reasoning":   "high",
		"role_token_usage": "unavailable",
	} {
		if got := finished[field]; got != want {
			t.Errorf("%s = %#v, want %#v", field, got, want)
		}
	}

	if got := process.roles[0].Spec.Provider; got.Model != "private-model-selection" || got.Reasoning != "high" {
		t.Errorf("session received provider selections %#v", got)
	}
}

func TestRunnerDynamicRoleMetricsUseEffectiveProviderAfterStartFailure(t *testing.T) {
	t.Parallel()

	role := dynamicRoleResource(t, "roles/dynamic")
	root := resolvedRoot(t, role)

	var output bytes.Buffer

	ctx := zerolog.New(&output).WithContext(context.Background())

	_, err := (Runner{
		Root:    root,
		Factory: errorFactory{err: context.DeadlineExceeded},
		Params:  map[string]string{"roles/dynamic.model": "private-model-selection"},
	}).Run(ctx, "review")
	if err == nil {
		t.Fatal("Runner.Run() succeeded with a failing provider factory")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Runner.Run() error = %v, want deadline classification", err)
	}

	for _, event := range decodeLifecycleEvents(t, &output) {
		if event["message"] != "agent finished" || event["kind"] != "DynamicRole" {
			continue
		}

		if event["role_provider"] != "generic_acp" || event["role_model"] != "private-model-selection" || event["role_reasoning"] != "high" {
			t.Errorf("failed DynamicRole selections = %#v", event)
		}

		return
	}

	t.Fatal("missing DynamicRole finish event")
}

func TestRunnerDynamicRoleErrorsDoNotDiscloseRuntimeValues(t *testing.T) {
	t.Parallel()

	const privateValue = "private-provider-value"

	tests := []struct {
		name    string
		prepare func(*agent.Resource)
		factory func() runtime.ProcessFactory
		field   string
	}{
		{
			name: "normalization",
			prepare: func(role *agent.Resource) {
				role.Spec.Provider.Reasoning = "{{ .State.private }}"
				role.Spec.State["private"] = privateValue
			},
			field: "spec.provider",
		},
		{
			name: "startup",
			prepare: func(role *agent.Resource) {
				role.Spec.Provider.Model = "{{ .State.private }}"
				role.Spec.State["private"] = privateValue
			},
			factory: func() runtime.ProcessFactory {
				return errorFactory{err: errors.New("failed to start " + privateValue)}
			},
			field: "spec.provider",
		},
		{
			name: "session",
			prepare: func(role *agent.Resource) {
				role.Spec.Provider.Model = "{{ .State.private }}"
				role.Spec.State["private"] = privateValue
			},
			factory: func() runtime.ProcessFactory {
				return &scriptedFactory{process: &scriptedProcess{prepareErr: errors.New("failed to prepare " + privateValue)}}
			},
			field: "spec.provider",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			role := dynamicRoleResource(t, "roles/dynamic")
			test.prepare(&role)
			root := resolvedRoot(t, role)
			defaultFactory := &scriptedFactory{process: &scriptedProcess{}}

			var factory runtime.ProcessFactory = defaultFactory
			if test.factory != nil {
				factory = test.factory()
			}

			_, err := (Runner{
				Root: root, Factory: factory,
				Params: map[string]string{"roles/dynamic.model": "model-v2"},
			}).Run(context.Background(), "review")
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Runner.Run() error = %v, want %s", err, test.field)
			}

			if strings.Contains(err.Error(), privateValue) {
				t.Errorf("Runner.Run() disclosed runtime provider value: %v", err)
			}

			if test.name == "normalization" && defaultFactory.starts != 0 {
				t.Errorf("provider starts = %d, want 0 for invalid configuration", defaultFactory.starts)
			}
		})
	}
}

func dynamicRoleResource(t *testing.T, id string) agent.Resource {
	t.Helper()

	resource := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.DynamicRoleKind,
		ID:         id,
		Source:     id + ".md",
		Spec: agent.Spec{
			Description: id,
			Provider: &agent.Provider{
				Type:      `{{ .State.provider }}`,
				Cmd:       `{{ .State.cmd }}`,
				Model:     `{{ .Params.model }}`,
				Reasoning: `{{ .State.reasoning }}`,
				Mode:      `{{ .Input }}`,
				ExtraArgs: []string{`--root={{ .Prompt }}`},
				Timeout:   `{{ .State.timeout }}`,
			},
			Params: map[string]string{"model": "model selection"},
			State: map[string]any{
				"provider":  "generic_acp",
				"cmd":       "custom-agent",
				"reasoning": "high",
				"timeout":   "250ms",
			},
			Body: "Work on:\n{{ .Input }}",
		},
	}
	if err := resource.Validate(); err != nil {
		t.Fatalf("dynamic role resource %q validation error: %v", id, err)
	}

	return resource
}
