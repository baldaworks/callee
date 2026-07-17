package runtime

import (
	"reflect"
	"testing"

	resource "github.com/baldaworks/callee/internal/agent"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
)

func TestNormalizeAgent(t *testing.T) {
	t.Parallel()

	for _, providerType := range resource.SupportedProviderTypes() {
		t.Run(providerType, func(t *testing.T) {
			t.Parallel()

			role := testAgentRole(providerType)
			role.Spec.Provider.Model = "model"
			role.Spec.Provider.Reasoning = "high"
			role.Spec.Provider.Mode = "review"
			role.Spec.Provider.ExtraArgs = []string{"--stdio"}
			role.Spec.Provider.Cmd = "/bin/agent"

			cfg, err := NormalizeAgent(role)
			if err != nil {
				t.Fatalf("NormalizeAgent() error: %v", err)
			}

			want, _ := resource.RuntimeType(providerType)
			if cfg.Type != want {
				t.Errorf("NormalizeAgent().Type = %q, want %q", cfg.Type, want)
			}
		})
	}
}

func TestAgentSessionState(t *testing.T) {
	t.Parallel()

	role := testAgentRole("codex")
	role.Spec.Provider.Model = "gpt-5.6-sol"
	role.Spec.Provider.Mode = "review"
	role.Spec.Provider.Reasoning = "high"

	state := agentSessionState(role)

	acpState, ok := state[acpagent.SessionStateKey].(map[string]any)
	if !ok {
		t.Fatalf("ACP state = %#v", state)
	}

	values, ok := acpState["config_values"].([]acpagent.SessionConfigValue)
	if !ok {
		t.Fatalf("session config values = %#v", acpState["config_values"])
	}

	want := []acpagent.SessionConfigValue{
		acpagent.SelectSessionConfigValue("model", "gpt-5.6-sol"),
		acpagent.SelectSessionConfigValue("mode", "review"),
		acpagent.SelectSessionConfigValue("reasoning_effort", "high"),
	}
	if !reflect.DeepEqual(values, want) {
		t.Errorf("session config values = %#v, want %#v", values, want)
	}
}

func TestProviderForAgentUsesRuntimeCommands(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want []string
	}{
		{name: "codex", want: []string{"npx", "-y", "@normahq/codex-acp-bridge@1.7.4"}},
		{name: "grok", want: []string{"grok", "agent", "stdio"}},
		{name: "cursor", want: []string{"agent", "acp"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			provider, err := ProviderForAgent(testAgentRole(test.name))
			if err != nil {
				t.Fatalf("ProviderForAgent() error: %v", err)
			}

			if !reflect.DeepEqual(provider.command, test.want) {
				t.Errorf("provider command = %#v, want %#v", provider.command, test.want)
			}
		})
	}
}

func testAgentRole(providerType string) resource.Resource {
	return resource.Resource{
		APIVersion: resource.APIVersion,
		Kind:       resource.RoleKind,
		ID:         "roles/test",
		Spec: resource.Spec{
			Description: "test role",
			Provider:    &resource.Provider{Type: providerType},
			Body:        "{{ .Input }}",
		},
	}
}
