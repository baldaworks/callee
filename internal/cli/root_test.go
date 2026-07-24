package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/normahq/codex-acp-bridge/pkg/cobracmd"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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
		"bridge":    true,
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

	wantAgent := map[string]bool{"run": true, "list": true, "schema": true, "view": true, "validate": true}
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

	if root.PersistentFlags().Lookup(agentRootFlagName) == nil {
		t.Fatalf("root is missing --%s", agentRootFlagName)
	}
}

func TestRootCommandEmbedsCodexBridge(t *testing.T) {
	t.Parallel()

	root := NewRootCommand()

	codex, args, err := root.Find([]string{"bridge", "codex"})
	if err != nil {
		t.Fatalf("find bridge codex command: %v", err)
	}

	if len(args) != 0 {
		t.Fatalf("remaining args = %q, want none", args)
	}

	if codex.Name() != "codex" {
		t.Errorf("command name = %q, want codex", codex.Name())
	}

	upstream := cobracmd.New()
	if codex.Long != upstream.Long {
		t.Errorf("command long description differs from upstream: got %q, want %q", codex.Long, upstream.Long)
	}

	wantExample := strings.ReplaceAll(upstream.Example, "codex-acp-bridge", "callee bridge codex")
	if codex.Example != wantExample {
		t.Errorf("command examples = %q, want %q", codex.Example, wantExample)
	}

	assertCommandFlagParity(t, codex, upstream)

	for _, flag := range []string{"name", "message-streaming", "reasoning-streaming", "reasoning-thoughts", "reasoning-summary", "codex-args", "debug"} {
		if codex.Flags().Lookup(flag) == nil {
			t.Errorf("bridge codex flag %q is missing", flag)
		}
	}

	version, _, err := root.Find([]string{"bridge", "codex", "version"})
	if err != nil {
		t.Fatalf("find bridge codex version command: %v", err)
	}

	if version.Name() != "version" {
		t.Errorf("version command name = %q, want version", version.Name())
	}
}

func assertCommandFlagParity(t *testing.T, got, want *cobra.Command) {
	t.Helper()

	want.Flags().VisitAll(func(wantFlag *pflag.Flag) {
		gotFlag := got.Flags().Lookup(wantFlag.Name)
		if gotFlag == nil {
			t.Errorf("mounted bridge is missing upstream flag %q", wantFlag.Name)

			return
		}

		if gotFlag.DefValue != wantFlag.DefValue || gotFlag.Value.Type() != wantFlag.Value.Type() || gotFlag.Usage != wantFlag.Usage {
			t.Errorf("mounted bridge flag %q differs from upstream", wantFlag.Name)
		}
	})

	got.Flags().VisitAll(func(gotFlag *pflag.Flag) {
		if want.Flags().Lookup(gotFlag.Name) == nil {
			t.Errorf("mounted bridge has non-upstream flag %q", gotFlag.Name)
		}
	})
}

func TestRunCodexBridgeVersionKeepsDiagnosticsOffStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{"bridge", "codex", "version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(bridge codex version) exit = %d, stderr = %q", code, stderr.String())
	}

	if got := strings.TrimSpace(stdout.String()); got == "" {
		t.Error("Run(bridge codex version) stdout is empty")
	}

	if stderr.Len() != 0 {
		t.Errorf("Run(bridge codex version) stderr = %q, want empty", stderr.String())
	}
}

func TestRunGlobalVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(--version) exit = %d, stderr = %q", code, stderr.String())
	}

	if got, want := stdout.String(), "callee version "+Version+"\n"; got != want {
		t.Errorf("Run(--version) stdout = %q, want %q", got, want)
	}

	if stderr.Len() != 0 {
		t.Errorf("Run(--version) stderr = %q, want empty", stderr.String())
	}
}

func TestRunCodexBridgeErrorsKeepStdoutEmpty(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "missing bridge name", args: []string{"bridge"}},
		{name: "unknown bridge", args: []string{"bridge", "unknown"}},
		{name: "invalid bridge flag", args: []string{"bridge", "codex", "--not-a-bridge-flag"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(context.Background(), test.args, &stdout, &stderr)

			if code != exitError {
				t.Errorf("Run(%q) exit = %d, want %d", test.args, code, exitError)
			}

			if stdout.Len() != 0 {
				t.Errorf("Run(%q) stdout = %q, want empty", test.args, stdout.String())
			}

			if !strings.Contains(stderr.String(), "Error:") {
				t.Errorf("Run(%q) stderr = %q, want command error", test.args, stderr.String())
			}
		})
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

func TestDoctorUsesExclusiveAgentRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	defaultDir := filepath.Join(project, ".callee")
	customDir := filepath.Join(project, "agents")

	writeVersionedAgent(t, defaultDir, "roles/default.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: default
  provider:
    type: codex
---
{{ .Input }}
`)
	writeVersionedAgent(t, customDir, "roles/custom.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: custom
  provider:
    type: codex
---
{{ .Input }}
`)

	oldDoctor := runAgentDoctor

	t.Cleanup(func() { runAgentDoctor = oldDoctor })

	var gotIDs []string

	runAgentDoctor = func(_ context.Context, agents []agent.Resource, _ runtime.ProcessFactory, _ time.Duration, _ io.Writer) error {
		for _, item := range agents {
			gotIDs = append(gotIDs, item.ID)
		}

		return nil
	}

	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{"--agent-root", customDir, "doctor"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(doctor) exit = %d, stderr = %q", code, stderr.String())
	}

	if got, want := strings.Join(gotIDs, ","), "roles/custom"; got != want {
		t.Fatalf("doctor agents = %q, want %q", got, want)
	}
}

func TestAgentRootRequiresExistingDirectory(t *testing.T) {
	project := isolateAgentRoots(t)

	var stdout, stderr bytes.Buffer

	code := Run(context.Background(), []string{"--agent-root", filepath.Join(project, "missing"), "agent", "list"}, &stdout, &stderr)
	if code != exitError {
		t.Fatalf("Run(agent list) exit = %d, want %d", code, exitError)
	}

	if !strings.Contains(stderr.String(), "does not exist") {
		t.Fatalf("stderr = %q, want missing root error", stderr.String())
	}
}

func isolateAgentRoots(t *testing.T) string {
	t.Helper()

	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	return project
}
