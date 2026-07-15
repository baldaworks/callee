package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
	acp "github.com/coder/acp-go-sdk"
)

func TestLoggingLevel(t *testing.T) {
	tests := []struct {
		name        string
		commandName string
		debug       bool
		trace       bool
		want        string
	}{
		{name: "default command", want: logging.LevelInfo},
		{name: "doctor", commandName: "doctor", want: logging.LevelError},
		{name: "doctor debug", commandName: "doctor", debug: true, want: logging.LevelDebug},
		{name: "doctor trace", commandName: "doctor", debug: true, trace: true, want: logging.LevelTrace},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := loggingLevel(test.commandName, test.debug, test.trace); got != test.want {
				t.Fatalf("loggingLevel(%q, %t, %t) = %q, want %q", test.commandName, test.debug, test.trace, got, test.want)
			}
		})
	}
}

func TestVersionMatchesNextRelease(t *testing.T) {
	if Version != "0.9.0" {
		t.Fatalf("Version = %q, want 0.9.0", Version)
	}
}

func TestRootCommandUsesProviderAwarePositioning(t *testing.T) {
	if got, want := NewRootCommand().Short, "Run provider-aware subagent roles described in Markdown."; got != want {
		t.Fatalf("root command description = %q, want %q", got, want)
	}
}

func TestMCPServerCommandIsUnavailable(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"mcp-server"})

	if err := cmd.Execute(); err == nil {
		t.Fatal("mcp-server command succeeded")
	}
}

func TestDoctorCommandLoadsRolesAndPassesTimeout(t *testing.T) {
	rolesDir := t.TempDir()
	rolePath := filepath.Join(rolesDir, "reviewer.md")

	roleBody := "---\ndescription: test reviewer\nprovider:\n  type: codex\n---\nReview: {{ prompt }}\n"
	if err := os.WriteFile(rolePath, []byte(roleBody), 0o600); err != nil {
		t.Fatal(err)
	}

	original := runDoctor

	t.Cleanup(func() { runDoctor = original })

	runDoctor = func(_ context.Context, roles []role.Role, _ doctor.Checker, timeout time.Duration, stdout io.Writer) error {
		if len(roles) != 1 || roles[0].ID != "reviewer" {
			t.Fatalf("roles = %#v", roles)
		}

		if timeout != 2*time.Second {
			t.Fatalf("timeout = %s, want 2s", timeout)
		}

		_, _ = fmt.Fprintln(stdout, "callee doctor: ok")

		return nil
	}

	cmd := NewRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"doctor", "--roles-dir", rolesDir, "--timeout", "2s"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), "callee doctor: ok\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestRoleListCommand(t *testing.T) {
	rolesDir := t.TempDir()

	roles := map[string]string{
		"reviewer.md": "---\ndescription: Reviews code changes.\nprovider:\n  type: codex\nparams:\n  focus: What to review\n---\nReview {{ prompt }} for {{ focus }}\n",
		"explorer.md": "---\ndescription: >\n  Explores the codebase.\nprovider:\n  type: codex\n---\nExplore {{ prompt }}\n",
	}
	for name, body := range roles {
		if err := os.WriteFile(filepath.Join(rolesDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "table",
			args: []string{"role", "list", "--roles-dir", rolesDir},
			want: "ID        DESCRIPTION             PARAMETERS\nexplorer  Explores the codebase.  -\nreviewer  Reviews code changes.   focus\n",
		},
		{
			name: "json",
			args: []string{"role", "list", "--roles-dir", rolesDir, "--json"},
			want: "{\"roles\":[{\"id\":\"explorer\",\"description\":\"Explores the codebase.\",\"params\":{}},{\"id\":\"reviewer\",\"description\":\"Reviews code changes.\",\"params\":{\"focus\":\"What to review\"}}]}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewRootCommand()

			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if got := stdout.String(); got != test.want {
				t.Errorf("stdout = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRoleListCommandReturnsRoleLoadingErrors(t *testing.T) {
	rolesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rolesDir, "invalid.md"), []byte("not frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"role", "list", "--roles-dir", rolesDir})

	if err := cmd.Execute(); err == nil {
		t.Fatal("list succeeded with an invalid role")
	}
}

func TestRoleListCommandWithNoRoles(t *testing.T) {
	rolesDir := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "table", args: []string{"role", "list", "--roles-dir", rolesDir}, want: "ID  DESCRIPTION  PARAMETERS\n"},
		{name: "json", args: []string{"role", "list", "--roles-dir", rolesDir, "--json"}, want: "{\"roles\":[]}\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewRootCommand()

			var stdout, stderr bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetErr(&stderr)
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if got := stdout.String(); got != test.want {
				t.Errorf("stdout = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPromptCommandRendersMessageAndPassesThreadHandle(t *testing.T) {
	rolesDir := writePromptRole(t)
	original := runRole

	t.Cleanup(func() { runRole = original })

	runRole = func(ctx context.Context, _ runtime.Factory, gotRole role.Role, message, threadID string) (runtime.Result, error) {
		if gotRole.ID != "reviewer" {
			t.Fatalf("role = %q", gotRole.ID)
		}

		if message != "inspect the change" {
			t.Fatalf("message = %q", message)
		}

		if threadID != "acp-old" {
			t.Fatalf("thread ID = %q", threadID)
		}

		deadline, ok := ctx.Deadline()
		if !ok {
			t.Fatal("prompt context has no deadline")
		}

		if remaining := time.Until(deadline); remaining < defaultPromptTimeout-time.Second || remaining > defaultPromptTimeout {
			t.Fatalf("prompt timeout remaining = %s", remaining)
		}

		return runtime.Result{ThreadID: "acp-old", Content: "review complete"}, nil
	}

	cmd := NewRootCommand()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "inspect the change", "--thread-id", "acp-old", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), "{\"threadId\":\"acp-old\",\"content\":\"review complete\",\"resumed\":true}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestJSONOutputUsesJSONDiagnostics(t *testing.T) {
	rolesDir := t.TempDir()
	cmd := NewRootCommand()

	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"role", "list", "--roles-dir", rolesDir, "--debug", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), "{\"roles\":[]}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}

	assertJSONLines(t, stderr.String())
}

func TestRunWritesJSONErrorsWhenRequested(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if exitCode := Run(context.Background(), []string{"prompt", "--role", "reviewer", "--message", "hello", "--timeout", "0", "--json"}, &stdout, &stderr); exitCode != 1 {
		t.Fatalf("exit code = %d, want 1", exitCode)
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	events := assertJSONLines(t, stderr.String())
	if len(events) != 1 || events[0]["level"] != "error" || events[0]["error"] != "timeout must be greater than zero" {
		t.Fatalf("events = %#v", events)
	}
}

func TestPromptCommandReturnsReplacementThreadWhenResumeFallsBack(t *testing.T) {
	rolesDir := writePromptRole(t)
	original := runRole

	t.Cleanup(func() { runRole = original })

	runRole = func(_ context.Context, _ runtime.Factory, _ role.Role, _ string, threadID string) (runtime.Result, error) {
		if threadID != "acp-expired" {
			t.Fatalf("thread ID = %q", threadID)
		}

		return runtime.Result{ThreadID: "acp-replacement", Content: "fresh response"}, nil
	}

	cmd := NewRootCommand()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "continue", "--thread-id", "acp-expired", "--json"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), "{\"threadId\":\"acp-replacement\",\"content\":\"fresh response\",\"resumed\":false}\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestPromptCommandRendersMessageAndParametersOnNewThread(t *testing.T) {
	rolesDir := writeParameterizedRole(t)
	messagePath := filepath.Join(t.TempDir(), "message.txt")
	contextPath := filepath.Join(t.TempDir(), "context.txt")
	message := "inspect the change\nwithout trimming\n"
	contextValue := "repository\ncontext\n"

	if err := os.WriteFile(messagePath, []byte(message), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) returned unexpected error: %v", messagePath, err)
	}

	if err := os.WriteFile(contextPath, []byte(contextValue), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) returned unexpected error: %v", contextPath, err)
	}

	original := runRole

	t.Cleanup(func() { runRole = original })

	runRole = func(_ context.Context, _ runtime.Factory, _ role.Role, got, threadID string) (runtime.Result, error) {
		if threadID != "" {
			t.Errorf("runRole() thread ID = %q, want empty", threadID)
		}

		want := "Review: " + message + "\nAudience: \nContext: " + contextValue + "\nLiteral: {{ example }}\n"
		if got != want {
			t.Errorf("runRole() message = %q, want %q", got, want)
		}

		return runtime.Result{ThreadID: "thread-new", Content: "done"}, nil
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"prompt", "--roles-dir", rolesDir, "--role", "reviewer",
		"--message-file", messagePath,
		"--param", "audience=",
		"--param-file", "context=" + contextPath,
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(prompt with params) returned unexpected error: %v", err)
	}
}

func TestPromptCommandResumeSendsRawMessage(t *testing.T) {
	rolesDir := writeParameterizedRole(t)
	original := runRole

	t.Cleanup(func() { runRole = original })

	runRole = func(_ context.Context, _ runtime.Factory, _ role.Role, got, threadID string) (runtime.Result, error) {
		if got != "follow up {{ audience }}" {
			t.Errorf("runRole() resumed message = %q, want raw input", got)
		}

		if threadID != "thread-old" {
			t.Errorf("runRole() thread ID = %q, want thread-old", threadID)
		}

		return runtime.Result{ThreadID: threadID, Content: "done"}, nil
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"prompt", "--roles-dir", rolesDir, "--role", "reviewer",
		"--thread-id", "thread-old", "--message", "follow up {{ audience }}",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(resumed prompt) returned unexpected error: %v", err)
	}
}

func TestPromptCommandParameterErrors(t *testing.T) {
	rolesDir := writeParameterizedRole(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing",
			args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello"},
			want: "missing=[audience context]",
		},
		{
			name: "unknown",
			args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello", "--param", "audience=x", "--param", "context=y", "--param", "extra=z"},
			want: "unknown=[extra]",
		},
		{
			name: "duplicate",
			args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello", "--param", "audience=x", "--param", "audience=y"},
			want: "more than once",
		},
		{
			name: "resume param",
			args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--thread-id", "old", "--message", "hello", "--param", "audience=x"},
			want: "only be supplied when starting a thread",
		},
		{
			name: "param stdin",
			args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello", "--param-file", "audience=-"},
			want: "not stdin",
		},
		{
			name: "param empty file path",
			args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello", "--param-file", "audience="},
			want: "non-empty file path",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewRootCommand()
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Execute(%q) error = %v, want containing %q", test.args, err, test.want)
			}
		})
	}
}

func TestPromptCommandMessageFileErrors(t *testing.T) {
	rolesDir := writePromptRole(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "both", args: []string{"prompt", "--role", "reviewer", "--message", "hello", "--message-file", "message.txt"}, want: "none of the others can be"},
		{name: "neither", args: []string{"prompt", "--role", "reviewer"}, want: "at least one"},
		{name: "empty", args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message-file", ""}, want: "non-empty file path"},
		{name: "stdin", args: []string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message-file", "-"}, want: "not stdin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewRootCommand()
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("Execute(%q) error = %v, want containing %q", test.args, err, test.want)
			}
		})
	}
}

func TestRoleListJSONIncludesParameterDescriptions(t *testing.T) {
	rolesDir := writeParameterizedRole(t)
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"role", "list", "--roles-dir", rolesDir, "--json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(list --json) returned unexpected error: %v", err)
	}

	for _, want := range []string{`"params"`, `"audience":"Intended readers"`, `"context":"Relevant context"`} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("list --json output does not contain %q: %s", want, stdout.String())
		}
	}
}

func TestPromptCommandTextOutputAndExplicitTimeout(t *testing.T) {
	rolesDir := writeRoleWithProvider(t, "  type: codex\n  timeout: 30s\n")
	original := runRole

	t.Cleanup(func() { runRole = original })

	runRole = func(ctx context.Context, _ runtime.Factory, _ role.Role, _ string, _ string) (runtime.Result, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < time.Second || time.Until(deadline) > 2*time.Second {
			t.Fatalf("prompt deadline = %v, remaining = %s", ok, time.Until(deadline))
		}

		return runtime.Result{ThreadID: "acp-1", Content: "plain text"}, nil
	}

	cmd := NewRootCommand()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello", "--timeout", "2s"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), "plain text\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestPromptCommandUsesProviderTimeout(t *testing.T) {
	rolesDir := writeRoleWithProvider(t, "  type: codex\n  timeout: 3s\n")
	original := runRole

	t.Cleanup(func() { runRole = original })

	runRole = func(ctx context.Context, _ runtime.Factory, _ role.Role, _ string, _ string) (runtime.Result, error) {
		deadline, ok := ctx.Deadline()

		remaining := time.Until(deadline)
		if !ok || remaining < 2*time.Second || remaining > 3*time.Second {
			t.Fatalf("prompt deadline = %v, remaining = %s", ok, remaining)
		}

		return runtime.Result{Content: "done"}, nil
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

type replConversation struct {
	prompts   []string
	threadIDs []string
	timeouts  []time.Duration
	closed    bool
	runErr    error
	closeErr  error
}

func (c *replConversation) Run(ctx context.Context, _ role.Role, prompt, threadID string) (runtime.Result, error) {
	c.prompts = append(c.prompts, prompt)
	c.threadIDs = append(c.threadIDs, threadID)

	deadline, ok := ctx.Deadline()
	if !ok {
		return runtime.Result{}, errors.New("turn context has no deadline")
	}

	c.timeouts = append(c.timeouts, time.Until(deadline))
	if c.runErr != nil {
		return runtime.Result{}, c.runErr
	}

	return runtime.Result{ThreadID: "thread-1", Content: fmt.Sprintf("response %d", len(c.prompts))}, nil
}

func (c *replConversation) Close() error {
	c.closed = true

	return c.closeErr
}

func TestPromptCommandRunsOptInREPLWithOneConversation(t *testing.T) {
	rolesDir := writeRoleWithProvider(t, "  type: codex\n  repl: true\n  timeout: 3s\n")
	conversation := &replConversation{}
	original := openRole

	t.Cleanup(func() { openRole = original })

	openRole = func(ctx context.Context, _ runtime.Factory, configuredRole role.Role) (runtime.Conversation, error) {
		if ctx.Err() != nil {
			t.Fatalf("runtime lifetime context is already cancelled: %v", ctx.Err())
		}

		if !configuredRole.Metadata.Provider.REPL {
			t.Fatal("provider.repl = false")
		}

		return conversation, nil
	}

	var stdout bytes.Buffer

	cmd := NewRootCommand()
	cmd.SetIn(strings.NewReader("follow up\n\nquit\n"))
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "initial", "--thread-id", "thread-old"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !conversation.closed {
		t.Fatal("REPL conversation was not closed")
	}

	if got, want := conversation.prompts, []string{"initial", "follow up"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prompts = %#v, want %#v", got, want)
	}

	if got, want := conversation.threadIDs, []string{"thread-old", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("thread IDs = %#v, want %#v", got, want)
	}

	for _, remaining := range conversation.timeouts {
		if remaining < 2*time.Second || remaining > 3*time.Second {
			t.Fatalf("turn timeout = %s, want independent 3s budget", remaining)
		}
	}

	if got, want := stdout.String(), "response 1\n> response 2\n> > "; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestPromptCommandRejectsJSONForREPLRole(t *testing.T) {
	rolesDir := writeRoleWithProvider(t, "  type: codex\n  repl: true\n")
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello", "--json"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--json is not supported") {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestPromptCommandClosesREPLAfterTurnAndCloseErrors(t *testing.T) {
	rolesDir := writeRoleWithProvider(t, "  type: codex\n  repl: true\n")
	turnErr := errors.New("turn failed")
	closeErr := errors.New("close failed")
	conversation := &replConversation{runErr: turnErr, closeErr: closeErr}
	original := openRole

	t.Cleanup(func() { openRole = original })

	openRole = func(context.Context, runtime.Factory, role.Role) (runtime.Conversation, error) {
		return conversation, nil
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello"})

	err := cmd.Execute()
	if !errors.Is(err, turnErr) || !errors.Is(err, closeErr) {
		t.Fatalf("Execute() error = %v, want turn and close errors", err)
	}

	if !conversation.closed {
		t.Fatal("REPL conversation was not closed")
	}
}

func TestPromptCommandTimesOutREPLStartup(t *testing.T) {
	rolesDir := writeRoleWithProvider(t, "  type: codex\n  repl: true\n  timeout: 20ms\n")
	original := openRole

	t.Cleanup(func() { openRole = original })

	stopped := make(chan struct{})
	openRole = func(ctx context.Context, _ runtime.Factory, _ role.Role) (runtime.Conversation, error) {
		defer close(stopped)

		<-ctx.Done()

		return nil, ctx.Err()
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"prompt", "--roles-dir", rolesDir, "--role", "reviewer", "--message", "hello"})

	err := cmd.Execute()
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Execute() error = %v, want deadline exceeded", err)
	}

	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("runtime startup did not stop after timeout")
	}
}

func TestPermissionHandlerSelectsOrCancels(t *testing.T) {
	request := acp.RequestPermissionRequest{Options: []acp.PermissionOption{
		{Name: "Allow once", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
		{Name: "Reject", Kind: acp.PermissionOptionKindRejectOnce, OptionId: "reject"},
	}}

	for _, test := range []struct {
		name  string
		input string
		want  string
	}{
		{name: "selected", input: "2\n", want: "reject"},
		{name: "invalid", input: "nope\n"},
		{name: "out of range", input: "3\n"},
		{name: "EOF"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer

			response, err := permissionHandler(bufio.NewReader(strings.NewReader(test.input)), &output)(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}

			if test.want == "" {
				if response.Outcome.Cancelled == nil {
					t.Fatalf("outcome = %#v, want cancelled", response.Outcome)
				}
			} else if response.Outcome.Selected == nil || string(response.Outcome.Selected.OptionId) != test.want {
				t.Fatalf("outcome = %#v, want selected %q", response.Outcome, test.want)
			}

			for _, want := range []string{"1) Allow once [allow_once]", "2) Reject [reject_once]", "Select: "} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("permission output missing %q: %s", want, output.String())
				}
			}
		})
	}
}

func TestPermissionHandlerHonorsTurnTimeout(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	request := acp.RequestPermissionRequest{Options: []acp.PermissionOption{
		{Name: "Allow", Kind: acp.PermissionOptionKindAllowOnce, OptionId: "allow"},
	}}

	_, err := permissionHandler(bufio.NewReader(reader), io.Discard)(ctx, request)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("permission error = %v, want deadline exceeded", err)
	}
}

func TestPromptCommandRejectsNonPositiveTimeout(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"prompt", "--role", "reviewer", "--message", "hello", "--timeout", "0"})

	if err := cmd.Execute(); err == nil || err.Error() != "timeout must be greater than zero" {
		t.Fatalf("error = %v", err)
	}
}

func TestLegacyRootCommandsAreUnavailable(t *testing.T) {
	for _, args := range [][]string{{"--role", "reviewer", "--prompt", "hello"}, {"list"}} {
		cmd := NewRootCommand()
		cmd.SetArgs(args)

		if err := cmd.Execute(); err == nil {
			t.Fatalf("legacy command %#v succeeded", args)
		}
	}
}

func writePromptRole(t *testing.T) string {
	t.Helper()

	rolesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rolesDir, "reviewer.md"), []byte("---\ndescription: test reviewer\nprovider:\n  type: codex\n---\nReview: {{ prompt }}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	return rolesDir
}

func writeParameterizedRole(t *testing.T) string {
	t.Helper()

	rolesDir := t.TempDir()

	body := "---\ndescription: test reviewer\nprovider:\n  type: codex\nparams:\n  audience: Intended readers\n  context: Relevant context\n---\nReview: {{ prompt }}\nAudience: {{ audience }}\nContext: {{ context }}\nLiteral: {{ example }}\n"
	if err := os.WriteFile(filepath.Join(rolesDir, "reviewer.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("os.WriteFile(parameterized role) returned unexpected error: %v", err)
	}

	return rolesDir
}

func writeRoleWithProvider(t *testing.T, provider string) string {
	t.Helper()

	rolesDir := t.TempDir()

	body := "---\ndescription: test reviewer\nprovider:\n" + provider + "---\n{{ prompt }}\n"
	if err := os.WriteFile(filepath.Join(rolesDir, "reviewer.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return rolesDir
}

func assertJSONLines(t *testing.T, output string) []map[string]any {
	t.Helper()

	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")

	events := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("diagnostic is not JSON: %v\n%s", err, output)
		}

		events = append(events, event)
	}

	return events
}
