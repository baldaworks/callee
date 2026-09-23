package agent

import (
	"reflect"
	"strings"
	"testing"
)

func TestDynamicRoleRendersEveryProviderField(t *testing.T) {
	t.Parallel()

	resource := Resource{
		ID:         "roles/dynamic",
		APIVersion: APIVersion,
		Kind:       DynamicRoleKind,
		Spec: Spec{
			Description: "dynamic worker",
			Provider: &Provider{
				Type:      `{{ .State.provider }}`,
				Cmd:       `{{ .State.cmd }}`,
				Model:     `{{ .Params.model }}`,
				Reasoning: `{{ .State.reasoning }}`,
				Mode:      `{{ .Input }}`,
				ExtraArgs: []string{`--profile={{ .State.profile }}`, `{{ .Prompt }}`},
				Timeout:   `{{ .State.timeout }}`,
			},
			Params: map[string]string{"model": "model selection"},
			Body:   "Work on:\n{{ .Input }}",
		},
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("Validate() error: %v", err)
	}

	effective, err := resource.RenderDynamicProvider(TemplateData{
		Prompt: "root request",
		Input:  "review",
		State: map[string]any{
			"provider":  "generic_acp",
			"cmd":       "custom-agent",
			"reasoning": "high",
			"profile":   "safe",
			"timeout":   "45s",
		},
		Params: map[string]string{"model": "model-v2"},
	})
	if err != nil {
		t.Fatalf("RenderDynamicProvider() error: %v", err)
	}

	want := &Provider{
		Type:      "generic_acp",
		Cmd:       "custom-agent",
		Model:     "model-v2",
		Reasoning: "high",
		Mode:      "review",
		ExtraArgs: []string{"--profile=safe", "root request"},
		Timeout:   "45s",
	}
	if !reflect.DeepEqual(effective.Spec.Provider, want) {
		t.Errorf("effective provider = %#v, want %#v", effective.Spec.Provider, want)
	}

	if resource.Spec.Provider.Type != `{{ .State.provider }}` {
		t.Errorf("authored provider was mutated: %#v", resource.Spec.Provider)
	}
}

func TestDynamicRoleMarkdownRoundTripPreservesProviderTemplates(t *testing.T) {
	t.Parallel()

	resource := validDynamicRole()

	encoded, err := EncodeMarkdown(resource)
	if err != nil {
		t.Fatalf("EncodeMarkdown() error: %v", err)
	}

	decoded, err := DecodeMarkdown(resource.ID, "dynamic.md", encoded)
	if err != nil {
		t.Fatalf("DecodeMarkdown() error: %v", err)
	}

	if decoded.Kind != DynamicRoleKind {
		t.Errorf("decoded kind = %q, want %q", decoded.Kind, DynamicRoleKind)
	}

	if !reflect.DeepEqual(decoded.Spec.Provider, resource.Spec.Provider) {
		t.Errorf("decoded provider = %#v, want %#v", decoded.Spec.Provider, resource.Spec.Provider)
	}
}

func TestDynamicRoleRejectsInvalidProviderTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field func(*Provider)
		want  string
	}{
		{name: "type", field: func(provider *Provider) { provider.Type = "{{ env \"HOME\" }}" }, want: "spec.provider.type"},
		{name: "cmd", field: func(provider *Provider) { provider.Cmd = "{{ .Output }}" }, want: "spec.provider.cmd"},
		{name: "extra argument", field: func(provider *Provider) { provider.ExtraArgs = []string{"{{"} }, want: "spec.provider.extraArgs[0]"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := validDynamicRole()
			test.field(resource.Spec.Provider)

			err := resource.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDynamicRoleRejectsInvalidRenderedProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state map[string]any
		want  string
	}{
		{name: "unsupported type", state: map[string]any{"provider": "unknown", "cmd": "agent", "arg": "ok", "timeout": "1s"}, want: "unsupported spec.provider.type"},
		{name: "blank generic command", state: map[string]any{"provider": "generic_acp", "cmd": "", "arg": "ok", "timeout": "1s"}, want: "requires nonblank spec.provider.cmd"},
		{name: "blank argument", state: map[string]any{"provider": "generic_acp", "cmd": "agent", "arg": "", "timeout": "1s"}, want: "spec.provider.extraArgs[0]"},
		{name: "invalid timeout", state: map[string]any{"provider": "generic_acp", "cmd": "agent", "arg": "ok", "timeout": "later"}, want: "spec.provider.timeout"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := validDynamicRole()

			_, err := resource.RenderDynamicProvider(TemplateData{State: test.state})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("RenderDynamicProvider() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestDynamicRoleErrorsDoNotDiscloseRenderedProviderValues(t *testing.T) {
	t.Parallel()

	const privateValue = "private-provider-value"

	tests := []struct {
		name  string
		field string
		set   func(*Provider)
	}{
		{name: "type", field: "spec.provider.type", set: func(provider *Provider) { provider.Type = "{{ .State.private }}" }},
		{name: "timeout", field: "spec.provider.timeout", set: func(provider *Provider) { provider.Timeout = "{{ .State.private }}" }},
		{name: "render", field: "spec.provider.model", set: func(provider *Provider) { provider.Model = "{{ fail .State.private }}" }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			resource := validDynamicRole()
			test.set(resource.Spec.Provider)

			_, err := resource.RenderDynamicProvider(TemplateData{State: map[string]any{
				"provider": "generic_acp", "cmd": "agent", "arg": "ok", "timeout": "1s", "private": privateValue,
			}})
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("RenderDynamicProvider() error = %v, want %s", err, test.field)
			}

			if strings.Contains(err.Error(), privateValue) {
				t.Errorf("RenderDynamicProvider() disclosed rendered value: %v", err)
			}
		})
	}
}

func TestRoleProviderRemainsStatic(t *testing.T) {
	t.Parallel()

	resource := validDynamicRole()
	resource.Kind = RoleKind

	err := resource.Validate()
	if err == nil || !strings.Contains(err.Error(), "provider/type") {
		t.Fatalf("Validate() error = %v, want static Role provider rejection", err)
	}
}

func validDynamicRole() Resource {
	return Resource{
		ID:         "roles/dynamic",
		APIVersion: APIVersion,
		Kind:       DynamicRoleKind,
		Spec: Spec{
			Description: "dynamic worker",
			Provider: &Provider{
				Type:      "{{ .State.provider }}",
				Cmd:       "{{ .State.cmd }}",
				ExtraArgs: []string{"{{ .State.arg }}"},
				Timeout:   "{{ .State.timeout }}",
			},
			Body: "Work on:\n{{ .Input }}",
		},
	}
}
