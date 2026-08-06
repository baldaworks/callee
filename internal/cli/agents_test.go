package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/workflow"
)

func TestAgentListAndView(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Implements a task.
  provider:
    type: codex
  permissions:
    mode: allow
  params:
    language: Implementation language
  body: |
    Implement in {{ .Params.language }}:
    {{ .Input }}
`)
	writeVersionedAgent(t, dir, "workflows/pipeline.yml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Runs a worker.
  children:
    - ref: roles/worker
      alias: worker
  body: |
    {{ .Input }}
`)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "list", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent list exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var catalog agentListOutput
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("decode agent list: %v", err)
	}

	if len(catalog.Agents) != 2 || catalog.Agents[0].ResourceID != "roles/worker" || catalog.Agents[1].Kind != agent.SequentialKind {
		t.Errorf("agent list = %+v", catalog.Agents)
	}

	stdout.Reset()
	stderr.Reset()

	exitCode = Run(context.Background(), []string{"agent", "view", "workflows/pipeline", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent view exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var view agentViewOutput
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("decode agent view: %v", err)
	}

	if view.ResourceID != "workflows/pipeline" || len(view.ResolvedTree.Children) != 1 {
		t.Errorf("agent view = %+v", view)
	}

	if len(view.RequiredParams) != 1 || view.RequiredParams[0].Key != "worker.language" {
		t.Errorf("required params = %+v, want worker.language", view.RequiredParams)
	}

	resolvedRole := view.ResolvedTree.Children[0]
	if resolvedRole.AuthoredPermissions == nil || resolvedRole.AuthoredPermissions.Mode != agent.PermissionModeAllow || resolvedRole.Permissions == nil || resolvedRole.Permissions.Mode != agent.PermissionModeAllow {
		t.Errorf("resolved permissions = authored %+v effective %+v, want allow/allow", resolvedRole.AuthoredPermissions, resolvedRole.Permissions)
	}

	assertAgentViewInteraction(t, view, resolvedRole)
}

func assertAgentViewInteraction(t *testing.T, view agentViewOutput, role *registry.ResolvedNode) {
	t.Helper()

	if role.Interactive == nil || role.REPL == nil || *role.Interactive != *role.REPL {
		t.Errorf("resolved interactive compat = interactive %+v repl %+v, want matching values", role.Interactive, role.REPL)
	}

	if role.AuthoredInteractive == nil || *role.AuthoredInteractive != *role.Interactive {
		t.Errorf("resolved interactive = authored %+v effective %+v, want matching values", role.AuthoredInteractive, role.Interactive)
	}

	if view.SpecDrivenInteractive || view.Interactive {
		t.Errorf("view interactive = spec-driven %t effective %t, want false/false", view.SpecDrivenInteractive, view.Interactive)
	}
}

func TestAgentViewProjectsGlobalPermissionOverrideWithoutMutation(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Implements a task.
  provider: {type: codex}
  permissions: {mode: ask}
  body: 'Work: {{ .Input }}'
`)

	for _, args := range [][]string{
		{"--permissions=allow", "agent", "view", "roles/worker", "--json"},
		{"agent", "view", "roles/worker", "--json", "--permissions=allow"},
	} {
		var stdout, stderr bytes.Buffer
		if exitCode := Run(context.Background(), args, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("agent view %q exit = %d, stderr = %q", args, exitCode, stderr.String())
		}

		var view agentViewOutput
		if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
			t.Fatalf("decode agent view: %v", err)
		}

		if !view.SpecDrivenInteractive || view.Interactive {
			t.Errorf("view interactive = spec-driven %t effective %t, want true/false", view.SpecDrivenInteractive, view.Interactive)
		}

		role := view.ResolvedTree
		if role.AuthoredPermissions == nil || role.AuthoredPermissions.Mode != agent.PermissionModeAsk || role.Permissions == nil || role.Permissions.Mode != agent.PermissionModeAllow {
			t.Errorf("view permissions = authored %+v effective %+v, want ask/allow", role.AuthoredPermissions, role.Permissions)
		}

		if role.AuthoredInteractive == nil || role.Interactive == nil || *role.AuthoredInteractive || *role.Interactive {
			t.Errorf("view Role interactive = authored %+v effective %+v, want false/false", role.AuthoredInteractive, role.Interactive)
		}
	}

	var stdout, stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"agent", "view", "roles/worker", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("unoverridden agent view exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var view agentViewOutput
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("decode unoverridden agent view: %v", err)
	}

	if !view.Interactive || view.ResolvedTree.Permissions == nil || view.ResolvedTree.Permissions.Mode != agent.PermissionModeAsk {
		t.Errorf("unoverridden view = %+v, want interactive ask policy", view)
	}
}

func TestGlobalPermissionsFlagValidation(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "blank", args: []string{"agent", "list", "--permissions="}, want: "--permissions must be ask, allow, or deny"},
		{name: "unknown", args: []string{"--permissions=sometimes", "agent", "list"}, want: "--permissions must be ask, allow, or deny"},
		{name: "bare", args: []string{"agent", "list", "--permissions"}, want: "flag needs an argument: --permissions"},
		{name: "incompatible values", args: []string{"agent", "run", "roles/missing", "--interactive=false", "--permissions=ask"}, want: "--interactive=false is incompatible with --permissions=ask"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if exitCode := Run(context.Background(), test.args, &stdout, &stderr); exitCode != exitError {
				t.Fatalf("Run(%q) exit = %d, want %d", test.args, exitCode, exitError)
			}

			if stdout.Len() != 0 || !strings.Contains(stderr.String(), test.want) {
				t.Errorf("Run(%q) stdout/stderr = %q/%q, want empty and %q", test.args, stdout.String(), stderr.String(), test.want)
			}
		})
	}
}

func TestGlobalPermissionsFlagAcceptsSupportedValues(t *testing.T) {
	isolateAgentRoots(t)

	for _, mode := range []string{"ask", "allow", "deny"} {
		var stdout, stderr bytes.Buffer
		if exitCode := Run(context.Background(), []string{"agent", "list", "--permissions=" + mode}, &stdout, &stderr); exitCode != 0 {
			t.Errorf("--permissions=%s exit = %d, stderr = %q", mode, exitCode, stderr.String())
		}
	}
}

func TestGlobalPermissionsFlagHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"agent", "view", "--help"}, {"agent", "run", "--help"}} {
		var stdout, stderr bytes.Buffer
		if exitCode := Run(context.Background(), args, &stdout, &stderr); exitCode != 0 {
			t.Fatalf("Run(%q) exit = %d, stderr = %q", args, exitCode, stderr.String())
		}

		for _, want := range []string{"--permissions", "ask", "allow", "deny", "every Role"} {
			if !strings.Contains(stdout.String(), want) {
				t.Errorf("Run(%q) help = %q, want containing %q", args, stdout.String(), want)
			}
		}
	}
}

func TestAgentRunPassesGlobalPermissionOverrideToSessionRole(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Implements a task.
  provider: {type: codex}
  permissions: {mode: ask}
---
Work: {{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	terminalCalls := 0
	openTerminal = func() (io.ReadWriteCloser, error) {
		terminalCalls++

		return nil, errors.New("non-interactive run must not open a terminal")
	}
	process := &cliTestProcess{response: "done"}
	newWorkflowFactory = func(_ io.Writer, interactor *terminalInteractor, pauses *workflow.PauseController) runtime.ProcessFactory {
		if interactor != nil || pauses != nil {
			t.Errorf("non-interactive factory interactor/pauses = %v/%v, want nil/nil", interactor, pauses)
		}

		return cliTestFactory{process: process}
	}

	var stdout, stderr bytes.Buffer

	args := []string{"agent", "run", "roles/worker", "--message", "work", "--permissions=deny"}
	if exitCode := Run(context.Background(), args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if len(process.roles) != 1 || process.roles[0].EffectivePermissionMode() != agent.PermissionModeDeny {
		t.Fatalf("session Roles = %+v, want one Role with permissions=deny", process.roles)
	}

	if process.roles[0].Interactive() {
		t.Error("permission override unexpectedly changed Role protocol")
	}

	if terminalCalls != 0 {
		t.Errorf("openTerminal calls = %d, want zero", terminalCalls)
	}
}

func TestAgentRunSpecDrivenAutomaticTreeDoesNotOpenTerminal(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Automatic worker.
  provider: {type: codex}
  permissions: {mode: deny}
---
Work: {{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	terminalCalls := 0
	openTerminal = func() (io.ReadWriteCloser, error) {
		terminalCalls++

		return nil, errors.New("unexpected terminal open")
	}
	process := &cliTestProcess{response: "done"}
	newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
		return cliTestFactory{process: process}
	}

	var stdout, stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"agent", "run", "roles/worker", "--message", "work"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if terminalCalls != 0 || stdout.String() != "done" {
		t.Errorf("terminal calls/stdout = %d/%q, want 0/done", terminalCalls, stdout.String())
	}
}

func TestAgentRunNonInteractivePreflightFailsBeforeTerminalOrFactory(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/auto.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Automatic worker.
  provider: {type: codex}
  permissions: {mode: allow}
  params: {language: Language}
---
Work in {{ .Params.language }}: {{ .Input }}
`)
	writeVersionedAgent(t, dir, "roles/ask.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Asking worker.
  provider: {type: codex}
---
Work: {{ .Input }}
`)
	writeVersionedAgent(t, dir, "humans/approval.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Human
spec:
  description: Approval.
  responseKey: approval
---
Approve: {{ .Input }}
`)
	writeVersionedAgent(t, dir, "workflows/router.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes work.
  route: '{{ .Input }}'
  children:
    - ref: roles/auto
      alias: auto
      route: auto
    - ref: humans/approval
      alias: approval
      default: true
---
{{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "missing message", args: []string{"agent", "run", "roles/auto", "--interactive=false", "--permissions=allow", "--param", "roles/auto.language=Go"}, want: "explicit nonblank --message"},
		{name: "missing parameter", args: []string{"agent", "run", "roles/auto", "--interactive=false", "--permissions=allow", "--message", "work"}, want: `required parameter "roles/auto.language" is missing`},
		{name: "spec ask", args: []string{"agent", "run", "roles/ask", "--interactive=false", "--message", "work"}, want: `Role "roles/ask" uses permissions=ask`},
		{name: "unselected Human branch", args: []string{"agent", "run", "workflows/router", "--interactive=false", "--permissions=allow", "--message", "auto", "--param", "auto.language=Go"}, want: `Human "approval" requires operator input`},
	} {
		t.Run(test.name, func(t *testing.T) {
			terminalCalls := 0
			factoryCalls := 0
			openTerminal = func() (io.ReadWriteCloser, error) {
				terminalCalls++

				return nil, errors.New("unexpected terminal open")
			}
			newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
				factoryCalls++

				return cliTestFactory{process: &cliTestProcess{response: "unexpected"}}
			}

			var stdout, stderr bytes.Buffer
			if exitCode := Run(context.Background(), test.args, &stdout, &stderr); exitCode != exitError {
				t.Fatalf("Run(%q) exit = %d, want %d", test.args, exitCode, exitError)
			}

			if terminalCalls != 0 || factoryCalls != 0 {
				t.Errorf("terminal/factory calls = %d/%d, want zero/zero", terminalCalls, factoryCalls)
			}

			if stdout.Len() != 0 || !strings.Contains(stderr.String(), test.want) {
				t.Errorf("stdout/stderr = %q/%q, want empty and %q", stdout.String(), stderr.String(), test.want)
			}
		})
	}
}

func TestAgentRunInteractiveAllowsAutomaticPermissions(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Worker.
  provider: {type: codex}
---
Work: {{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	terminalCalls := 0
	openTerminal = func() (io.ReadWriteCloser, error) {
		terminalCalls++

		return &splitTerminal{input: strings.NewReader("")}, nil
	}
	process := &cliTestProcess{response: "done\n\ncallee.control.v1.return"}
	newWorkflowFactory = func(_ io.Writer, interactor *terminalInteractor, pauses *workflow.PauseController) runtime.ProcessFactory {
		if interactor == nil || pauses == nil {
			t.Errorf("interactive factory interactor/pauses = %v/%v, want non-nil", interactor, pauses)
		}

		return cliTestFactory{process: process}
	}

	var stdout, stderr bytes.Buffer

	args := []string{"agent", "run", "roles/worker", "--message", "work", "--interactive=true", "--permissions=allow"}
	if exitCode := Run(context.Background(), args, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if terminalCalls != 1 || stdout.String() != "done" {
		t.Errorf("terminal calls/stdout = %d/%q, want 1/done", terminalCalls, stdout.String())
	}
}

func TestAgentSchemaCommand(t *testing.T) {
	tests := []struct {
		kind       string
		definition string
	}{
		{kind: "Role", definition: "role"},
		{kind: "Script", definition: "script"},
		{kind: "Human", definition: "human"},
		{kind: "Sequential", definition: "sequential"},
		{kind: "Loop", definition: "loop"},
		{kind: "Router", definition: "router"},
	}

	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := Run(context.Background(), []string{"agent", "schema", test.kind}, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("agent schema exit = %d, stderr = %q", exitCode, stderr.String())
			}

			var document map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
				t.Fatalf("json.Unmarshal(%q): %v", test.kind, err)
			}

			if got, want := document["$ref"], "#/$defs/"+test.definition; got != want {
				t.Fatalf("schema[%q] $ref = %#v, want %q", test.kind, got, want)
			}

			if _, exists := document["$id"]; exists {
				t.Fatalf("schema[%q] unexpectedly contains $id", test.kind)
			}

			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestAgentSchemaCommandReportsKindErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing kind",
			args: []string{"agent", "schema"},
			want: "accepts 1 arg(s), received 0",
		},
		{
			name: "unsupported kind",
			args: []string{"agent", "schema", "Parallel"},
			want: `unsupported kind "Parallel" (want Role, Script, Human, Sequential, Loop, or Router)`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := Run(context.Background(), test.args, &stdout, &stderr)
			if exitCode != exitError {
				t.Fatalf("agent schema exit = %d, want %d", exitCode, exitError)
			}

			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}

			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("stderr = %q, want containing %q", stderr.String(), test.want)
			}
		})
	}
}

func TestAgentListAndViewIncludeRouterEdges(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")

	for _, id := range []string{"named", "fallback"} {
		writeVersionedAgent(t, dir, "roles/"+id+".md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: `+id+`
  provider: {type: codex}
---
		{{ .Input }}
`)
	}

	writeVersionedAgent(t, dir, "workflows/router.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes work.
  route: '{{ .Input }}'
  children:
    - ref: roles/named
      alias: named
      route: 'bug|urgent'
    - ref: roles/fallback
      alias: fallback
      default: true
  body: |
    {{ .Input }}
`)

	writeVersionedAgent(t, dir, "workflows/outer.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Wraps the Router.
  children:
    - ref: workflows/router
      alias: router
  body: |
    {{ .Input }}
`)

	var stdout, stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"agent", "list", "--kind", "Router", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent list exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var catalog agentListOutput
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("decode agent list: %v", err)
	}

	if len(catalog.Agents) != 1 || catalog.Agents[0].ResourceID != "workflows/router" || catalog.Agents[0].Kind != agent.RouterKind {
		t.Fatalf("Router catalog = %+v", catalog.Agents)
	}

	stdout.Reset()
	stderr.Reset()

	if exitCode := Run(context.Background(), []string{"agent", "view", "workflows/outer", "--json"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent view exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var view agentViewOutput

	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("decode agent view: %v", err)
	}

	if len(view.ResolvedTree.Children) != 1 || len(view.ResolvedTree.Children[0].Children) != 2 {
		t.Fatalf("resolved Router tree = %+v", view.ResolvedTree)
	}

	routerChildren := view.ResolvedTree.Children[0].Children

	if routerChildren[0].Route != "bug|urgent" || routerChildren[0].Default {
		t.Errorf("named resolved edge = route %q default %t", routerChildren[0].Route, routerChildren[0].Default)
	}

	if routerChildren[1].Route != "" || !routerChildren[1].Default {
		t.Errorf("default resolved edge = route %q default %t", routerChildren[1].Route, routerChildren[1].Default)
	}

	stdout.Reset()
	stderr.Reset()

	if exitCode := Run(context.Background(), []string{"agent", "view", "workflows/outer"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent text view exit = %d, stderr = %q", exitCode, stderr.String())
	}

	for _, want := range []string{
		"router [Router] -> workflows/router",
		`named [Role] -> roles/named canEscalate=false route="bug|urgent"`,
		"fallback [Role] -> roles/fallback canEscalate=false default=true",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("agent view = %q, want containing %q", stdout.String(), want)
		}
	}
}

func TestAgentListAndViewIncludeHumanNodes(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")

	writeVersionedAgent(t, dir, "workflows/request-input.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Human
spec:
  description: Requests operator input.
  responseKey: approval
  body: |
    Question: {{ .Input }}
`)
	writeVersionedAgent(t, dir, "workflows/pipeline.yml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Runs a human step.
  children:
    - ref: workflows/request-input
      alias: approver
  body: |
    {{ .Input }}
`)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "list", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent list exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var catalog agentListOutput
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("decode agent list: %v", err)
	}

	if len(catalog.Agents) != 2 {
		t.Fatalf("agent list count = %d, want 2", len(catalog.Agents))
	}

	foundHuman := false

	for _, item := range catalog.Agents {
		if item.ResourceID == "workflows/request-input" && item.Kind == agent.HumanKind {
			foundHuman = true

			break
		}
	}

	if !foundHuman {
		t.Fatalf("agent list = %+v, want workflows/request-input with kind Human", catalog.Agents)
	}

	stdout.Reset()
	stderr.Reset()

	exitCode = Run(context.Background(), []string{"agent", "view", "workflows/pipeline", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent view exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var view agentViewOutput
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("decode agent view: %v", err)
	}

	if len(view.ResolvedTree.Children) != 1 {
		t.Fatalf("resolved children = %d, want 1", len(view.ResolvedTree.Children))
	}

	if got, want := view.ResolvedTree.Children[0].Kind, agent.HumanKind; got != want {
		t.Fatalf("resolved child kind = %q, want %q", got, want)
	}
}

func TestAgentListUsesExclusiveAgentRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	defaultDir := filepath.Join(project, ".callee")
	customDir := filepath.Join(project, "agents")

	writeVersionedAgent(t, defaultDir, "roles/default.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Default worker.
  provider:
    type: codex
---
{{ .Input }}
`)
	writeVersionedAgent(t, customDir, "roles/custom.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Custom worker.
  provider:
    type: codex
---
{{ .Input }}
`)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--agent-root", customDir, "agent", "list", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent list exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var catalog agentListOutput
	if err := json.Unmarshal(stdout.Bytes(), &catalog); err != nil {
		t.Fatalf("decode agent list: %v", err)
	}

	if len(catalog.Agents) != 1 || catalog.Agents[0].ResourceID != "roles/custom" {
		t.Fatalf("agent list = %+v, want only roles/custom", catalog.Agents)
	}
}

func TestAgentViewUsesExclusiveAgentRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	defaultDir := filepath.Join(project, ".callee")
	customDir := filepath.Join(project, "agents")

	writeVersionedAgent(t, defaultDir, "roles/default.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Default worker.
  provider:
    type: codex
---
{{ .Input }}
`)
	writeVersionedAgent(t, customDir, "roles/custom.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Custom worker.
  provider:
    type: codex
---
{{ .Input }}
`)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--agent-root", customDir, "agent", "view", "roles/custom", "--json"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent view exit = %d, stderr = %q", exitCode, stderr.String())
	}

	var view agentViewOutput
	if err := json.Unmarshal(stdout.Bytes(), &view); err != nil {
		t.Fatalf("decode agent view: %v", err)
	}

	if view.ResourceID != "roles/custom" {
		t.Fatalf("agent view = %+v, want roles/custom", view)
	}
}

func TestAgentValidateStandaloneFiles(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name    string
		file    string
		content string
	}{
		{
			name: "Markdown Role",
			file: "worker.md",
			content: `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: worker
  provider: {type: codex}
---
{{ .Input }}
`,
		},
		{
			name: "YAML Role",
			file: "worker.yaml",
			content: `apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: worker
  provider: {type: codex}
  body: |
    {{ .Input }}
`,
		},
		{
			name: "standalone YML workflow",
			file: "pipeline.yml",
			content: `apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: pipeline
  children: [roles/not-installed]
  body: |
    {{ .Input }}
`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, test.file)
			if err := os.WriteFile(path, []byte(test.content), 0o600); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer

			exitCode := Run(context.Background(), []string{"agent", "validate", path}, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("agent validate exit = %d, stderr = %q", exitCode, stderr.String())
			}

			if got, want := stdout.String(), path+": ok\n"; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}

			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}

func TestAgentValidateReportsFileErrors(t *testing.T) {
	root := t.TempDir()

	invalidYAML := filepath.Join(root, "invalid.yaml")
	if err := os.WriteFile(invalidYAML, []byte("apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: invalid\n  provider: {type: codex}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	unsupported := filepath.Join(root, "agent.json")
	if err := os.WriteFile(unsupported, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "invalid object", path: invalidYAML, want: "missing property 'body'"},
		{name: "unsupported extension", path: unsupported, want: "unsupported agent file extension"},
		{name: "missing file", path: filepath.Join(root, "missing.yml"), want: "read agent file"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			exitCode := Run(context.Background(), []string{"agent", "validate", test.path}, &stdout, &stderr)
			if exitCode != exitError {
				t.Errorf("agent validate exit = %d, want %d", exitCode, exitError)
			}

			if stdout.Len() != 0 {
				t.Errorf("stdout = %q, want empty", stdout.String())
			}

			if !strings.Contains(stderr.String(), test.want) {
				t.Errorf("stderr = %q, want containing %q", stderr.String(), test.want)
			}
		})
	}
}

func TestAgentRunWritesArtifactAfterCleanup(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Implements a task.
  provider:
    type: codex
---
Task: {{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	terminal := &splitTerminal{input: strings.NewReader("build\n")}
	openTerminal = func() (io.ReadWriteCloser, error) { return terminal, nil }
	process := &cliTestProcess{
		response: "implemented",
		usage: &runtime.TokenUsage{
			InputTokens:      11,
			OutputTokens:     3,
			TotalTokens:      17,
			CachedReadTokens: 4,
		},
	}
	newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
		return cliTestFactory{process: process}
	}

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "run", "roles/worker"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got, want := stdout.String(), "implemented"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	diagnostics := stripANSI(stderr.String())
	for _, want := range []string{"INF running agent", "id=roles/worker", "kind=Role", "visit=1", "INF agent finished", "status=completed", "outcome=return", "INF agent run finished"} {
		if !strings.Contains(diagnostics, want) {
			t.Errorf("stderr = %q, want containing %q", stderr.String(), want)
		}
	}

	requireDiagnosticLine(t, diagnostics,
		"INF agent finished",
		"role_provider=codex",
		"role_model=backend-default",
		"role_reasoning=backend-default",
		"role_duration=",
		"role_wait_duration=0s",
		"role_token_usage=complete",
		"role_input_tokens=11",
		"role_output_tokens=3",
		"role_total_tokens=17",
		"role_cached_read_tokens=4",
	)
	requireDiagnosticLine(t, diagnostics,
		"INF agent run finished",
		"agent_duration=",
		"agent_wait_duration=",
		"agent_token_usage=complete",
		"agent_input_tokens=11",
		"agent_output_tokens=3",
		"agent_total_tokens=17",
		"agent_cached_read_tokens=4",
	)

	if strings.Contains(stderr.String(), "implemented") {
		t.Errorf("stderr = %q, want artifact only on stdout", stderr.String())
	}

	if !process.closed {
		t.Errorf("provider process was not closed before command return")
	}
}

func TestAgentRunRoutesNamedDefaultAndFailsClosed(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")

	for _, id := range []string{"named", "fallback"} {
		writeVersionedAgent(t, dir, "roles/"+id+".md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: `+id+`
  provider: {type: codex}
---
		{{ .Input }}
`)
	}

	writeVersionedAgent(t, dir, "workflows/router.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes work.
  route: '{{ .Prompt }}'
  children:
    - ref: roles/named
      alias: named
      route: named
    - ref: roles/fallback
      alias: fallback
      default: true
  body: |
    payload={{ .Input }}
`)

	writeVersionedAgent(t, dir, "workflows/closed-router.yaml", `apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes known work only.
  route: '{{ .Prompt }}'
  children:
    - ref: roles/named
      alias: named
      route: named
  body: |
    payload={{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	openTerminal = func() (io.ReadWriteCloser, error) {
		return &splitTerminal{input: strings.NewReader("")}, nil
	}

	var (
		process *cliTestProcess
		starts  int
	)

	newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
		return cliTestFactory{process: process, starts: &starts}
	}

	for _, test := range []struct {
		name            string
		message         string
		wantArtifact    string
		wantEffectiveID string
		wantDiagnostic  string
	}{
		{name: "named", message: "named", wantArtifact: "named-result", wantEffectiveID: "named", wantDiagnostic: "route=named"},
		{name: "default", message: "unknown", wantArtifact: "default-result", wantEffectiveID: "fallback", wantDiagnostic: "default=true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			process = &cliTestProcess{responses: map[string]string{
				"named":    "named-result",
				"fallback": "default-result",
			}}
			starts = 0

			var stdout, stderr bytes.Buffer

			exitCode := Run(context.Background(), []string{"agent", "run", "workflows/router", "--message", test.message}, &stdout, &stderr)
			if exitCode != 0 {
				t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
			}

			if stdout.String() != test.wantArtifact {
				t.Errorf("stdout = %q, want %q", stdout.String(), test.wantArtifact)
			}

			if starts != 1 || len(process.effectiveIDs) != 1 || process.effectiveIDs[0] != test.wantEffectiveID {
				t.Errorf("provider starts/effective IDs = %d/%#v, want 1/%q", starts, process.effectiveIDs, test.wantEffectiveID)
			}

			if diagnostics := stripANSI(stderr.String()); !strings.Contains(diagnostics, "selected router child") || !strings.Contains(diagnostics, test.wantDiagnostic) {
				t.Errorf("stderr = %q, want Router selection and %q", stderr.String(), test.wantDiagnostic)
			}
		})
	}

	process = &cliTestProcess{response: "unexpected"}
	starts = 0

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "run", "workflows/closed-router", "--message", "unknown"}, &stdout, &stderr)
	if exitCode != exitError {
		t.Fatalf("closed Router exit = %d, want %d; stderr=%q", exitCode, exitError, stderr.String())
	}

	if stdout.Len() != 0 || starts != 0 {
		t.Errorf("closed Router stdout/provider starts = %q/%d, want empty/0", stdout.String(), starts)
	}

	if !strings.Contains(stderr.String(), `route "unknown" did not match any child`) {
		t.Errorf("stderr = %q, want fail-closed route diagnostic", stderr.String())
	}
}

func TestAgentRunInteractiveFlagHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if exitCode := Run(context.Background(), []string{"agent", "run", "--help"}, &stdout, &stderr); exitCode != 0 {
		t.Fatalf("agent run --help exit = %d, stderr = %q", exitCode, stderr.String())
	}

	for _, want := range []string{"--interactive", "--interactive=true", "--interactive=false", "every Role's interactive mode"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("agent run help = %q, want containing %q", stdout.String(), want)
		}
	}
}

func TestAgentRunInteractiveFlagRequiresExplicitValue(t *testing.T) {
	var stdout, stderr bytes.Buffer

	if exitCode := Run(context.Background(), []string{"agent", "run", "roles/worker", "--interactive"}, &stdout, &stderr); exitCode != exitError {
		t.Fatalf("agent run bare --interactive exit = %d, want %d; stderr=%q", exitCode, exitError, stderr.String())
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), "flag needs an argument: --interactive") {
		t.Errorf("stderr = %q, want explicit-value diagnostic", stderr.String())
	}
}

func TestAgentRunUsesExclusiveAgentRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	defaultDir := filepath.Join(project, ".callee")
	customDir := filepath.Join(project, "agents")

	writeVersionedAgent(t, defaultDir, "roles/default.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Default worker.
  provider:
    type: codex
---
default: {{ .Input }}
`)
	writeVersionedAgent(t, customDir, "roles/custom.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Custom worker.
  provider:
    type: codex
---
custom: {{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	terminal := &splitTerminal{input: strings.NewReader("build\n")}
	openTerminal = func() (io.ReadWriteCloser, error) { return terminal, nil }
	process := &cliTestProcess{response: "implemented"}
	newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
		return cliTestFactory{process: process}
	}

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--agent-root", customDir, "agent", "run", "roles/custom"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got, want := stdout.String(), "implemented"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	if diagnostics := stripANSI(stderr.String()); !strings.Contains(diagnostics, "id=roles/custom") {
		t.Fatalf("stderr = %q, want custom role diagnostics", stderr.String())
	}
}

var ansiCSIPattern = regexp.MustCompile(`\x1b\[[0-?]*[ -/]*[@-~]`)

func stripANSI(value string) string {
	return ansiCSIPattern.ReplaceAllString(value, "")
}

func TestAgentRunRequiresTTYBeforeProviderFactory(t *testing.T) {
	project := isolateAgentRoots(t)
	dir := filepath.Join(project, ".callee")
	writeVersionedAgent(t, dir, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Implements a task.
  provider:
    type: codex
---
{{ .Input }}
`)

	oldOpenTerminal := openTerminal
	oldFactory := newWorkflowFactory

	t.Cleanup(func() {
		openTerminal = oldOpenTerminal
		newWorkflowFactory = oldFactory
	})

	openTerminal = func() (io.ReadWriteCloser, error) { return nil, errors.New("no controlling TTY") }
	factoryCreated := false
	newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
		factoryCreated = true

		return cliTestFactory{process: &cliTestProcess{response: "unexpected"}}
	}

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "run", "roles/worker", "--message", "build"}, &stdout, &stderr)
	if exitCode != exitError {
		t.Fatalf("agent run exit = %d, want %d", exitCode, exitError)
	}

	if factoryCreated {
		t.Fatal("provider factory was created before controlling-TTY preflight")
	}

	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), "interactive terminal is required") {
		t.Errorf("stderr = %q, want controlling-TTY diagnostic", stderr.String())
	}
}

func TestWriteResolvedNodeIncludesEffectivePolicies(t *testing.T) {
	t.Parallel()

	repl := false
	maxIterations := 3
	root := &registry.ResolvedNode{
		EffectiveID:   "goalkeeper",
		ResourceID:    "workflows/goalkeeper",
		Kind:          agent.LoopKind,
		MaxIterations: &maxIterations,
		OnExhausted:   "fail",
		Children: []*registry.ResolvedNode{
			{
				EffectiveID: "worker",
				ResourceID:  "roles/worker",
				Kind:        agent.RoleKind,
				Interactive: &repl,
				REPL:        &repl,
				Children:    []*registry.ResolvedNode{},
			},
		},
	}

	var output bytes.Buffer
	if err := writeResolvedNode(&output, root, "  "); err != nil {
		t.Fatalf("writeResolvedNode() error: %v", err)
	}

	for _, want := range []string{"maxIterations=3 onExhausted=fail", "interactive=false", "canEscalate=false", "permissions=ask authoredPermissions=default"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("writeResolvedNode() = %q, want containing %q", output.String(), want)
		}
	}
}

type splitTerminal struct {
	input  *strings.Reader
	output bytes.Buffer
}

func (t *splitTerminal) Read(p []byte) (int, error)  { return t.input.Read(p) }
func (t *splitTerminal) Write(p []byte) (int, error) { return t.output.Write(p) }
func (t *splitTerminal) Close() error                { return nil }

type cliTestFactory struct {
	process *cliTestProcess
	starts  *int
}

func (f cliTestFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	if f.starts != nil {
		*f.starts++
	}

	return f.process, nil
}

type cliTestProcess struct {
	response     string
	responses    map[string]string
	usage        *runtime.TokenUsage
	closed       bool
	effectiveIDs []string
	roles        []agent.Resource
}

func (p *cliTestProcess) NewSession(_ context.Context, role agent.Resource, effectiveID string) (runtime.AgentSession, error) {
	p.effectiveIDs = append(p.effectiveIDs, effectiveID)
	p.roles = append(p.roles, role)

	response := p.response
	if selected, ok := p.responses[effectiveID]; ok {
		response = selected
	}

	return cliTestSession{response: response, usage: p.usage}, nil
}

func (p *cliTestProcess) Close() error {
	p.closed = true

	return nil
}

type cliTestSession struct {
	response string
	usage    *runtime.TokenUsage
}

func (s cliTestSession) Turn(context.Context, string) (runtime.TurnResult, error) {
	if s.response == "" {
		return runtime.TurnResult{}, fmt.Errorf("missing response")
	}

	return runtime.TurnResult{Content: s.response, Usage: s.usage}, nil
}

func (cliTestSession) Prepare(context.Context) error {
	return nil
}

func writeVersionedAgent(t *testing.T, root, relative, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q): %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}
