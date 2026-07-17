package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/logging"
)

func TestLoggingLevel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		commandName string
		debug       bool
		trace       bool
		want        string
	}{
		{name: "default", commandName: "agent", want: logging.LevelInfo},
		{name: "doctor", commandName: "doctor", want: logging.LevelError},
		{name: "debug", commandName: "agent", debug: true, want: logging.LevelDebug},
		{name: "trace", commandName: "agent", debug: true, trace: true, want: logging.LevelTrace},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := loggingLevel(test.commandName, test.debug, test.trace); got != test.want {
				t.Errorf("loggingLevel(%q, %t, %t) = %q, want %q", test.commandName, test.debug, test.trace, got, test.want)
			}
		})
	}
}

func TestRootCommandExposesOnlyWorkflowSurface(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()
	wantRoot := map[string]bool{
		"agent":     true,
		"doctor":    true,
		"promptkit": true,
		"setup":     true,
	}

	for _, command := range root.Commands() {
		if !wantRoot[command.Name()] {
			t.Errorf("unexpected root command %q", command.Name())
		}

		delete(wantRoot, command.Name())
	}

	if len(wantRoot) != 0 {
		t.Errorf("missing root commands: %v", wantRoot)
	}

	agentCommand, _, err := root.Find([]string{"agent"})
	if err != nil {
		t.Fatalf("find agent command: %v", err)
	}

	wantAgent := map[string]bool{"run": true, "list": true, "view": true, "validate": true}
	for _, command := range agentCommand.Commands() {
		if !wantAgent[command.Name()] {
			t.Errorf("unexpected agent command %q", command.Name())
		}

		delete(wantAgent, command.Name())
	}

	if len(wantAgent) != 0 {
		t.Errorf("missing agent commands: %v", wantAgent)
	}

	for _, flag := range []string{"role", "roles-dir", "thread-id", "message-file"} {
		if root.PersistentFlags().Lookup(flag) != nil || agentCommand.Flags().Lookup(flag) != nil {
			t.Errorf("legacy flag %q is still exposed", flag)
		}
	}
}

func TestRunRejectsLegacyCommands(t *testing.T) {
	for _, command := range []string{"exec", "role"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(context.Background(), []string{command}, &stdout, &stderr)

			if code != exitError {
				t.Errorf("Run(%q) exit = %d, want %d", command, code, exitError)
			}

			if !strings.Contains(stderr.String(), "unknown command") {
				t.Errorf("Run(%q) stderr = %q, want unknown command", command, stderr.String())
			}
		})
	}
}

func TestRunWritesJSONErrorsForJSONCommands(t *testing.T) {
	isolateAgentRoots(t)

	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{"agent", "view", "missing", "--json"}, &stdout, &stderr)

	if code != exitError {
		t.Fatalf("Run() exit = %d, want %d", code, exitError)
	}

	var event map[string]any
	if err := json.Unmarshal(stderr.Bytes(), &event); err != nil {
		t.Fatalf("decode JSON error %q: %v", stderr.String(), err)
	}

	if event["level"] != "error" {
		t.Errorf("JSON error = %#v, want error level", event)
	}
}

func TestSignalExitCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		cause error
		want  int
	}{
		{name: "interrupt", cause: ErrInterrupt, want: exitInterrupt},
		{name: "terminate", cause: ErrTerminate, want: exitTerminate},
		{name: "ordinary cancellation", cause: context.Canceled, want: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancelCause(context.Background())
			cancel(test.cause)

			if got := signalExitCode(ctx); got != test.want {
				t.Errorf("signalExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRootRequiresCommand(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs(nil)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "a command is required") {
		t.Fatalf("Execute() error = %v, want command required", err)
	}
}

func isolateAgentRoots(t *testing.T) string {
	t.Helper()

	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	return project
}
