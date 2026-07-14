package cli

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestSetupCommandInstallsPluginAndCreatesReviewer(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantCommands [][]string
		wantType     string
	}{
		{
			name:   "codex",
			target: "codex",
			wantCommands: [][]string{
				{"codex", "plugin", "marketplace", "add", "baldaworks/callee", "--sparse", ".agents/plugins"},
				{"codex", "plugin", "add", "callee@callee"},
			},
			wantType: "type: codex",
		},
		{
			name:   "claude",
			target: "claude",
			wantCommands: [][]string{
				{"claude", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"claude", "plugin", "install", "callee@callee", "--scope", "project"},
			},
			wantType: "type: claude",
		},
		{
			name:   "grok",
			target: "grok",
			wantCommands: [][]string{
				{"grok", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"grok", "plugin", "install", "callee@callee", "--trust"},
			},
			wantType: "type: grok",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			original := runSetupCommand

			t.Cleanup(func() { runSetupCommand = original })

			var gotCommands [][]string

			runSetupCommand = func(_ context.Context, _ io.Writer, _ io.Writer, name string, args ...string) error {
				gotCommands = append(gotCommands, append([]string{name}, args...))

				return nil
			}

			cmd := NewRootCommand()

			var stdout bytes.Buffer
			cmd.SetOut(&stdout)
			cmd.SetArgs([]string{"setup", test.target})

			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(gotCommands, test.wantCommands) {
				t.Fatalf("commands = %#v, want %#v", gotCommands, test.wantCommands)
			}

			role, err := os.ReadFile(filepath.FromSlash(reviewerRolePath))
			if err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(string(role), test.wantType) || !strings.Contains(string(role), "{{ prompt }}") {
				t.Fatalf("reviewer role = %q", role)
			}

			if !strings.Contains(stdout.String(), "Created "+reviewerRolePath) {
				t.Fatalf("stdout = %q", stdout.String())
			}
		})
	}
}

func TestSetupCommandLeavesExistingReviewerUntouched(t *testing.T) {
	t.Chdir(t.TempDir())

	path := filepath.FromSlash(reviewerRolePath)
	if err := os.MkdirAll(filepath.Dir(path), reviewerDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("existing"), reviewerFileMode); err != nil {
		t.Fatal(err)
	}

	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(context.Context, io.Writer, io.Writer, string, ...string) error { return nil }

	cmd := NewRootCommand()

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"setup", "codex"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	role, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(role) != "existing" {
		t.Fatalf("reviewer role = %q, want existing", role)
	}

	if !strings.Contains(stdout.String(), "already exists") {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func TestSetupCommandRejectsUnknownTarget(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "other"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported setup target") {
		t.Fatalf("setup error = %v", err)
	}
}
