package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
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
	if Version != "0.6.0" {
		t.Fatalf("Version = %q, want 0.6.0", Version)
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

	roleBody := "---\ndescription: test reviewer\ntype: codex\n---\nReview: {{ prompt }}\n"
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

func TestListCommand(t *testing.T) {
	rolesDir := t.TempDir()

	roles := map[string]string{
		"reviewer.md": "---\ndescription: Reviews code changes.\ntype: codex\n---\nReview {{ prompt }}\n",
		"explorer.md": "---\ndescription: >\n  Explores the codebase.\ntype: codex\n---\nExplore {{ prompt }}\n",
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
			args: []string{"list", "--roles-dir", rolesDir},
			want: "ID        DESCRIPTION\nexplorer  Explores the codebase.\nreviewer  Reviews code changes.\n",
		},
		{
			name: "json",
			args: []string{"list", "--roles-dir", rolesDir, "--json"},
			want: "{\"roles\":[{\"id\":\"explorer\",\"description\":\"Explores the codebase.\\n\"},{\"id\":\"reviewer\",\"description\":\"Reviews code changes.\"}]}\n",
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

func TestListCommandReturnsRoleLoadingErrors(t *testing.T) {
	rolesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rolesDir, "invalid.md"), []byte("not frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"list", "--roles-dir", rolesDir})

	if err := cmd.Execute(); err == nil {
		t.Fatal("list succeeded with an invalid role")
	}
}

func TestListCommandWithNoRoles(t *testing.T) {
	rolesDir := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "table", args: []string{"list", "--roles-dir", rolesDir}, want: "ID  DESCRIPTION\n"},
		{name: "json", args: []string{"list", "--roles-dir", rolesDir, "--json"}, want: "{\"roles\":[]}\n"},
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

		if message != "Review: inspect the change\n" {
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
	cmd.SetArgs([]string{"list", "--roles-dir", rolesDir, "--debug", "--json"})

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

func TestPromptCommandTextOutputAndExplicitTimeout(t *testing.T) {
	rolesDir := writePromptRole(t)
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

func TestPromptCommandRejectsNonPositiveTimeout(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"prompt", "--role", "reviewer", "--message", "hello", "--timeout", "0"})

	if err := cmd.Execute(); err == nil || err.Error() != "timeout must be greater than zero" {
		t.Fatalf("error = %v", err)
	}
}

func TestLegacyRootAndRoleListCommandsAreUnavailable(t *testing.T) {
	for _, args := range [][]string{{"--role", "reviewer", "--prompt", "hello"}, {"role", "list"}} {
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
	if err := os.WriteFile(filepath.Join(rolesDir, "reviewer.md"), []byte("---\ndescription: test reviewer\ntype: codex\n---\nReview: {{ prompt }}\n"), 0o600); err != nil {
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
