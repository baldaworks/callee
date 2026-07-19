//go:build linux || darwin

package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/workflow"
	"github.com/creack/pty"
)

const agentPTYHelper = "CALLEE_AGENT_PTY_HELPER"

func TestAgentRunUsesControllingPTYWithSeparateStreams(t *testing.T) {
	if mode := os.Getenv(agentPTYHelper); mode != "" {
		newWorkflowFactory = func(io.Writer, *terminalInteractor, *workflow.PauseController) runtime.ProcessFactory {
			responses := []string{"implemented"}
			wantEscalate := false

			if mode == "repl" {
				responses = []string{
					"Which target?\n\ncallee.control.v1.await",
					"Final implementation\n\ncallee.control.v1.return",
				}
			} else if mode == "loop" {
				responses = []string{"implemented\n\ncallee.control.v1.escalate"}
				wantEscalate = true
			}

			return ptyTestFactory{process: &ptyTestProcess{responses: responses, wantEscalate: wantEscalate}}
		}

		agentID := "roles/worker"
		if mode == "loop" {
			agentID = "workflows/loop"
		}

		code := Run(context.Background(), []string{"agent", "run", agentID, "--message", "build"}, os.Stdout, os.Stderr)
		os.Exit(code)
	}

	for _, test := range []struct {
		name       string
		mode       string
		terminalIn string
		want       string
	}{
		{name: "root Role", mode: "direct", want: "implemented"},
		{name: "root REPL await", mode: "repl", terminalIn: "linux\n", want: "Final implementation"},
		{name: "Loop child escalation", mode: "loop", want: "implemented"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runAgentPTYTest(t, test.mode, test.terminalIn, test.want)
		})
	}
}

func runAgentPTYTest(t *testing.T, mode, terminalInput, want string) {
	t.Helper()

	project := t.TempDir()
	repl := mode == "repl"

	replDeclaration := ""
	if repl {
		replDeclaration = "  repl: true\n"
	}

	writeVersionedAgent(t, filepath.Join(project, ".callee"), "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Implements a task.
  provider:
    type: codex
`+replDeclaration+`---
{{ .Input }}
`)

	if mode == "loop" {
		writeVersionedAgent(t, filepath.Join(project, ".callee"), "workflows/loop.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Runs a worker until it escalates.
  children:
    - ref: roles/worker
      alias: worker
      canEscalate: true
  maxIterations: 2
  onExhausted: fail
  output: '{{ .State.outputs.worker }}'
---
{{ .Input }}
`)
	}

	terminal, childTerminal, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open() error: %v", err)
	}
	defer terminal.Close()
	defer childTerminal.Close()

	command := exec.Command(os.Args[0], "-test.run=^TestAgentRunUsesControllingPTYWithSeparateStreams$")
	command.Dir = project

	command.Env = append(os.Environ(), agentPTYHelper+"="+mode, "XDG_CONFIG_HOME="+filepath.Join(project, "xdg"))
	command.Stdin = childTerminal
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}

	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start PTY helper: %v", err)
	}

	if terminalInput != "" {
		if _, err := io.Copy(terminal, strings.NewReader(terminalInput)); err != nil {
			t.Fatalf("write PTY input: %v", err)
		}
	}

	if err := command.Wait(); err != nil {
		t.Fatalf("PTY helper error: %v; stdout=%q stderr=%q", err, stdout.String(), stderr.String())
	}

	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}

	diagnostics := stripANSI(stderr.String())

	roleID := "roles/worker"
	if mode == "loop" {
		roleID = "worker"
	}

	requireDiagnosticLine(t, diagnostics, "INF running agent", "id="+roleID, "kind=Role", "visit=1")

	if mode == "loop" {
		requireDiagnosticLine(t, diagnostics, "INF agent finished", "id=worker", "kind=Role", "visit=1", "status=completed", "outcome=escalate")
		requireDiagnosticLine(t, diagnostics, "INF agent finished", "id=workflows/loop", "kind=Loop", "visit=1", "status=completed", "outcome=return")

		if strings.Contains(diagnostics, "visit=2") {
			t.Errorf("Loop executed an unexpected second visit; stderr=%q", stderr.String())
		}
	} else {
		requireDiagnosticLine(t, diagnostics, "INF agent finished", "id=roles/worker", "kind=Role", "visit=1", "status=completed", "outcome=return")
	}

	for _, expected := range []string{"INF entering repl", "INF exiting repl"} {
		contains := strings.Contains(diagnostics, expected)
		if contains != repl {
			t.Errorf("stderr contains %q = %t, want %t; stderr=%q", expected, contains, repl, stderr.String())
		}
	}

	if strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want artifact only on stdout", stderr.String())
	}
}

func requireDiagnosticLine(t *testing.T, diagnostics string, fields ...string) {
	t.Helper()

	for line := range strings.SplitSeq(diagnostics, "\n") {
		matches := true

		for _, field := range fields {
			if !strings.Contains(line, field) {
				matches = false

				break
			}
		}

		if matches {
			return
		}
	}

	t.Errorf("diagnostics have no line containing %q:\n%s", fields, diagnostics)
}

type ptyTestFactory struct{ process *ptyTestProcess }

func (f ptyTestFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	return f.process, nil
}

type ptyTestProcess struct {
	responses    []string
	wantEscalate bool
}

func (p *ptyTestProcess) NewSession(context.Context, agent.Resource) (runtime.AgentSession, error) {
	return &ptyTestSession{process: p}, nil
}

func (p *ptyTestProcess) Close() error { return nil }

type ptyTestSession struct{ process *ptyTestProcess }

func (s *ptyTestSession) Prepare(context.Context) error { return nil }

func (s *ptyTestSession) Turn(_ context.Context, prompt string) (string, error) {
	hasEscalation := strings.Contains(prompt, "callee.control.v1.escalate")
	if hasEscalation != s.process.wantEscalate {
		return "", fmt.Errorf("prompt escalation presence = %t, want %t", hasEscalation, s.process.wantEscalate)
	}

	if len(s.process.responses) == 0 {
		return "", io.EOF
	}

	response := s.process.responses[0]
	s.process.responses = s.process.responses[1:]

	return response, nil
}
