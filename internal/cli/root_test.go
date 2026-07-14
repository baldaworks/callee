package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/role"
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

func TestRoleListCommand(t *testing.T) {
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
			args: []string{"role", "list", "--roles-dir", rolesDir},
			want: "ID        DESCRIPTION\nexplorer  Explores the codebase.\nreviewer  Reviews code changes.\n",
		},
		{
			name: "json",
			args: []string{"role", "list", "--roles-dir", rolesDir, "--json"},
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

func TestRoleListCommandReturnsRoleLoadingErrors(t *testing.T) {
	rolesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rolesDir, "invalid.md"), []byte("not frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"role", "list", "--roles-dir", rolesDir})

	if err := cmd.Execute(); err == nil {
		t.Fatal("role list succeeded with an invalid role")
	}
}

func TestRoleListCommandWithNoRoles(t *testing.T) {
	rolesDir := t.TempDir()

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "table", args: []string{"role", "list", "--roles-dir", rolesDir}, want: "ID  DESCRIPTION\n"},
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
