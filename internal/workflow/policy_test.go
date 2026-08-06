package workflow

import (
	"context"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

func TestResolveRolePolicyKeepsPermissionAndProtocolIndependent(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name            string
		authored        bool
		interactive     *bool
		permissions     *agent.PermissionMode
		wantInteractive bool
		wantPermissions agent.PermissionMode
	}{
		{name: "spec defaults", wantPermissions: agent.PermissionModeAsk},
		{name: "permission does not enable repl", permissions: permissionModePointer(agent.PermissionModeAsk), wantPermissions: agent.PermissionModeAsk},
		{name: "permission does not disable repl", authored: true, permissions: permissionModePointer(agent.PermissionModeDeny), wantInteractive: true, wantPermissions: agent.PermissionModeDeny},
		{name: "interactive override does not change permission", interactive: boolPointer(true), permissions: permissionModePointer(agent.PermissionModeAllow), wantInteractive: true, wantPermissions: agent.PermissionModeAllow},
		{name: "one-shot override preserves ask", authored: true, interactive: boolPointer(false), wantPermissions: agent.PermissionModeAsk},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			role := roleResource(t, "roles/worker", test.authored, nil, "Work: {{ .Input }}")

			policy, err := ResolveRolePolicy(role, PolicyOverrides{Interactive: test.interactive, Permissions: test.permissions})
			if err != nil {
				t.Fatalf("ResolveRolePolicy() error: %v", err)
			}

			if policy.Interactive != test.wantInteractive || policy.Permissions != test.wantPermissions {
				t.Errorf("ResolveRolePolicy() = %+v, want interactive=%t permissions=%s", policy, test.wantInteractive, test.wantPermissions)
			}
		})
	}
}

func TestValidatePolicyOverridesRejectsUnsupportedPermission(t *testing.T) {
	t.Parallel()

	mode := agent.PermissionMode("sometimes")

	err := ValidatePolicyOverrides(PolicyOverrides{Permissions: &mode})
	if err == nil || !strings.Contains(err.Error(), "must be ask, allow, or deny") {
		t.Fatalf("ValidatePolicyOverrides() error = %v, want supported-values diagnostic", err)
	}
}

func TestTreeRequiresInteractionUsesEffectiveTreePolicy(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		resources []agent.Resource
		override  *agent.PermissionMode
		want      bool
	}{
		{name: "default ask", resources: []agent.Resource{roleResource(t, "roles/ask", false, nil, "Work: {{ .Input }}")}, want: true},
		{name: "automatic one-shot", resources: []agent.Resource{roleWithPermissions(t, "roles/auto", false, agent.PermissionModeAllow)}, want: false},
		{name: "authored repl", resources: []agent.Resource{roleWithPermissions(t, "roles/repl", true, agent.PermissionModeDeny)}, want: true},
		{name: "permission override removes ask without changing protocol", resources: []agent.Resource{roleResource(t, "roles/ask", false, nil, "Work: {{ .Input }}")}, override: permissionModePointer(agent.PermissionModeAllow), want: false},
		{name: "human", resources: []agent.Resource{humanResource(t, "humans/approval", "approval", "Approve")}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			root := resolvedRoot(t, test.resources...)

			got, err := TreeRequiresInteraction(root, PolicyOverrides{Permissions: test.override})
			if err != nil {
				t.Fatalf("TreeRequiresInteraction() error: %v", err)
			}

			if got != test.want {
				t.Errorf("TreeRequiresInteraction() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestRunnerPermissionOverrideUsesSessionCopyAndKeepsProtocolIndependent(t *testing.T) {
	t.Parallel()

	for _, mode := range []agent.PermissionMode{
		agent.PermissionModeAsk,
		agent.PermissionModeAllow,
		agent.PermissionModeDeny,
	} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()

			role := roleWithPermissions(t, "roles/worker", false, agent.PermissionModeAllow)
			root := resolvedRoot(t, role)
			process := &scriptedProcess{visits: map[string][][]string{
				"roles/worker": {{"done"}},
			}}

			got, err := (Runner{
				Root:               root,
				Factory:            &scriptedFactory{process: process},
				PermissionOverride: permissionModePointer(mode),
			}).Run(context.Background(), "work")
			if err != nil {
				t.Fatalf("Runner.Run() error: %v", err)
			}

			if got != "done" {
				t.Errorf("Runner.Run() = %q, want done", got)
			}

			if len(process.roles) != 1 || process.roles[0].EffectivePermissionMode() != mode {
				t.Fatalf("session Roles = %+v, want one Role with permissions=%s", process.roles, mode)
			}

			if process.roles[0].Interactive() {
				t.Error("permission override unexpectedly enabled REPL protocol")
			}

			if root.Resource.EffectivePermissionMode() != agent.PermissionModeAllow {
				t.Errorf("canonical permissions = %s, want authored allow", root.Resource.EffectivePermissionMode())
			}
		})
	}
}

func TestRunnerPermissionOverrideReachesNestedAliasesAndLoopVisits(t *testing.T) {
	t.Parallel()

	nested := compositeResource(t, "workflows/nested", agent.SequentialKind, []agent.Child{
		{Ref: "roles/nested", Alias: "nested_role"},
	}, 0, "{{ .Input }}", "{{ .State.outputs.nested_role }}")
	loop := compositeResource(t, "workflows/repeat", agent.LoopKind, []agent.Child{
		{Ref: "roles/repeated", Alias: "repeated"},
	}, 2, "{{ .Input }}", "{{ .State.outputs.repeated }}")
	loop.Spec.OnExhausted = "complete"
	root := resolvedRoot(t,
		roleResource(t, "roles/direct", false, nil, "Direct: {{ .Input }}"),
		roleResource(t, "roles/nested", false, nil, "Nested: {{ .Input }}"),
		roleResource(t, "roles/repeated", false, nil, "Repeated: {{ .Input }}"),
		nested,
		loop,
		compositeResource(t, "workflows/root", agent.SequentialKind, []agent.Child{
			{Ref: "roles/direct", Alias: "direct"},
			{Ref: "workflows/nested", Alias: "nested"},
			{Ref: "workflows/repeat", Alias: "repeat"},
		}, 0, "{{ .Input }}", "{{ .State.outputs.repeat }}"),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/direct":   {{"direct"}},
		"roles/nested":   {{"nested"}},
		"roles/repeated": {{"repeat-one"}, {"repeat-two"}},
	}}

	got, err := (Runner{
		Root:               root,
		Factory:            &scriptedFactory{process: process},
		PermissionOverride: permissionModePointer(agent.PermissionModeDeny),
	}).Run(context.Background(), "work")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "repeat-two" {
		t.Errorf("Runner.Run() = %q, want repeat-two", got)
	}

	if len(process.roles) != 4 {
		t.Fatalf("session Roles = %d, want four visits", len(process.roles))
	}

	for _, role := range process.roles {
		if role.EffectivePermissionMode() != agent.PermissionModeDeny {
			t.Errorf("session Role %q permissions = %s, want deny", role.ID, role.EffectivePermissionMode())
		}
	}

	for _, node := range []*registry.ResolvedNode{root.Children[0], root.Children[1].Children[0], root.Children[2].Children[0]} {
		if node.Resource.Spec.Permissions != nil {
			t.Errorf("canonical Role %q permissions mutated to %+v", node.ResourceID, node.Resource.Spec.Permissions)
		}
	}
}

func TestRunnerRejectsUnsupportedPermissionOverrideBeforeFactoryStart(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, roleResource(t, "roles/worker", false, nil, "Work: {{ .Input }}"))
	process := &scriptedProcess{visits: map[string][][]string{"roles/worker": {{"done"}}}}
	factory := &scriptedFactory{process: process}
	mode := agent.PermissionMode("sometimes")

	_, err := (Runner{Root: root, Factory: factory, PermissionOverride: &mode}).Run(context.Background(), "work")
	if err == nil || !strings.Contains(err.Error(), "must be ask, allow, or deny") {
		t.Fatalf("Runner.Run() error = %v, want supported-values diagnostic", err)
	}

	if factory.starts != 0 {
		t.Errorf("factory starts = %d, want zero", factory.starts)
	}
}

func roleWithPermissions(t *testing.T, id string, interactive bool, mode agent.PermissionMode) agent.Resource {
	t.Helper()

	role := roleResource(t, id, interactive, nil, "Work: {{ .Input }}")
	role.Spec.Permissions = &agent.Permissions{Mode: mode}

	return role
}

func permissionModePointer(mode agent.PermissionMode) *agent.PermissionMode {
	return &mode
}

func boolPointer(value bool) *bool {
	return &value
}
