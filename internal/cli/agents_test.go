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

	terminal := &splitTerminal{input: strings.NewReader("")}
	openTerminal = func() (io.ReadWriteCloser, error) { return terminal, nil }
	process := &cliTestProcess{response: "implemented"}
	newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
		return cliTestFactory{process: process}
	}

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "run", "roles/worker", "--message", "build"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent run exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got, want := stdout.String(), "implemented"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	diagnostics := stripANSI(stderr.String())
	for _, want := range []string{"INF running agent", "id=roles/worker", "kind=Role", "visit=1", "INF agent finished", "status=completed", "outcome=return"} {
		if !strings.Contains(diagnostics, want) {
			t.Errorf("stderr = %q, want containing %q", stderr.String(), want)
		}
	}

	if strings.Contains(stderr.String(), "implemented") {
		t.Errorf("stderr = %q, want artifact only on stdout", stderr.String())
	}

	if !process.closed {
		t.Errorf("provider process was not closed before command return")
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
				EffectiveID:     "worker",
				ResourceID:      "roles/worker",
				Kind:            agent.RoleKind,
				Session:         agent.SessionModeStateful,
				SessionScopeID:  "goalkeeper",
				AuthoredSession: agent.SessionModeStateful,
				REPL:            &repl,
				Children:        []*registry.ResolvedNode{},
			},
		},
	}

	var output bytes.Buffer
	if err := writeResolvedNode(&output, root, "  "); err != nil {
		t.Fatalf("writeResolvedNode() error: %v", err)
	}

	for _, want := range []string{
		"maxIterations=3 onExhausted=fail",
		"repl=false",
		"canEscalate=false",
		"permissions=ask authoredPermissions=default",
		"session=stateful authoredSession=stateful sessionScope=goalkeeper",
	} {
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

type cliTestFactory struct{ process *cliTestProcess }

func (f cliTestFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	return f.process, nil
}

type cliTestProcess struct {
	response string
	closed   bool
}

func (p *cliTestProcess) NewSession(context.Context, agent.Resource, string) (runtime.AgentSession, error) {
	return cliTestSession{response: p.response}, nil
}

func (p *cliTestProcess) Close() error {
	p.closed = true

	return nil
}

type cliTestSession struct{ response string }

func (s cliTestSession) Turn(context.Context, string) (string, error) {
	if s.response == "" {
		return "", fmt.Errorf("missing response")
	}

	return s.response, nil
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
