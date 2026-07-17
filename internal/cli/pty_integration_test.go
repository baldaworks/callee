//go:build linux || darwin

package cli

import (
	"bytes"
	"context"
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
			if mode == "repl" {
				responses = []string{
					"Which target?\n\ncallee.control.v1.await",
					"Final implementation\n\ncallee.control.v1.return",
				}
			}

			return ptyTestFactory{process: &ptyTestProcess{responses: responses}}
		}

		code := Run(context.Background(), []string{"agent", "run", "roles/worker", "--message", "build"}, os.Stdout, os.Stderr)
		os.Exit(code)
	}

	for _, test := range []struct {
		name       string
		repl       bool
		terminalIn string
		want       string
	}{
		{name: "non-REPL", want: "implemented"},
		{name: "REPL await", repl: true, terminalIn: "linux\n", want: "Final implementation"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runAgentPTYTest(t, test.repl, test.terminalIn, test.want)
		})
	}
}

func runAgentPTYTest(t *testing.T, repl bool, terminalInput, want string) {
	t.Helper()

	project := t.TempDir()

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

	terminal, childTerminal, err := pty.Open()
	if err != nil {
		t.Fatalf("pty.Open() error: %v", err)
	}
	defer terminal.Close()
	defer childTerminal.Close()

	command := exec.Command(os.Args[0], "-test.run=^TestAgentRunUsesControllingPTYWithSeparateStreams$")
	command.Dir = project

	mode := "direct"
	if repl {
		mode = "repl"
	}

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
	for _, expected := range []string{"INF running agent", "id=roles/worker", "kind=Role", "visit=1", "INF agent finished", "status=completed", "outcome=return"} {
		if !strings.Contains(diagnostics, expected) {
			t.Errorf("stderr = %q, want containing %q", stderr.String(), expected)
		}
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

type ptyTestFactory struct{ process *ptyTestProcess }

func (f ptyTestFactory) Start(context.Context, runtime.Provider) (runtime.ProviderProcess, error) {
	return f.process, nil
}

type ptyTestProcess struct{ responses []string }

func (p *ptyTestProcess) NewSession(context.Context, agent.Resource) (runtime.AgentSession, error) {
	return &ptyTestSession{process: p}, nil
}

func (p *ptyTestProcess) Close() error { return nil }

type ptyTestSession struct{ process *ptyTestProcess }

func (s *ptyTestSession) Prepare(context.Context) error { return nil }

func (s *ptyTestSession) Turn(context.Context, string) (string, error) {
	if len(s.process.responses) == 0 {
		return "", io.EOF
	}

	response := s.process.responses[0]
	s.process.responses = s.process.responses[1:]

	return response, nil
}
