package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const viewRoleMarkdown = `---
description: |
  Reviews changes carefully.
repl: true
provider:
  type: generic_acp
  cmd: review-agent
  model: review-model
  reasoning: high
  mode: read-only
  extra_args:
    - --strict
  timeout: 20m
params:
  audience: Intended readers
  focus: What to review
---
Review {{ prompt }} for {{ audience }}, focusing on {{ focus }}.
`

func TestRoleViewCommand(t *testing.T) {
	rolesDir := writeViewRole(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "human",
			args: []string{"role", "view", "reviewer", "--roles-dir", rolesDir},
			want: "ID: reviewer\n" +
				"API: callee.metalagman.dev\n" +
				"Kind: role\n" +
				"Description: Reviews changes carefully.\n" +
				"Provider type: generic_acp\n" +
				"REPL: true\n" +
				"Timeout: 20m (provider)\n" +
				"Command: review-agent\n" +
				"Model: review-model\n" +
				"Reasoning: high\n" +
				"Mode: read-only\n" +
				"Extra args: [\"--strict\"]\n" +
				"Parameters:\n" +
				"  audience: Intended readers\n" +
				"  focus: What to review\n",
		},
		{
			name: "json",
			args: []string{"role", "view", "reviewer", "--roles-dir", rolesDir, "--json"},
			want: "{\"id\":\"reviewer\",\"api\":\"callee.metalagman.dev\",\"kind\":\"role\",\"description\":\"Reviews changes carefully.\",\"repl\":true," +
				"\"provider\":{\"type\":\"generic_acp\",\"cmd\":\"review-agent\",\"model\":\"review-model\",\"reasoning\":\"high\",\"mode\":\"read-only\"," +
				"\"extraArgs\":[\"--strict\"],\"timeout\":\"20m\",\"effectiveTimeout\":\"20m\",\"timeoutSource\":\"provider\"},\"params\":{\"audience\":\"Intended readers\",\"focus\":\"What to review\"}}\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewRootCommand()

			var stdout bytes.Buffer

			cmd.SetOut(&stdout)
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

func TestRoleViewMarkdownReturnsNormalizedRole(t *testing.T) {
	rolesDir := writeViewRole(t)
	cmd := NewRootCommand()

	var stdout bytes.Buffer

	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"role", "view", "reviewer", "--roles-dir", rolesDir, "--markdown"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	want := "---\n" +
		"api: callee.metalagman.dev\n" +
		"kind: role\n" +
		"description: |\n" +
		"    Reviews changes carefully.\n" +
		"repl: true\n" +
		"provider:\n" +
		"    type: generic_acp\n" +
		"    cmd: review-agent\n" +
		"    model: review-model\n" +
		"    reasoning: high\n" +
		"    mode: read-only\n" +
		"    extra_args:\n" +
		"        - --strict\n" +
		"    timeout: 20m\n" +
		"params:\n" +
		"    audience: Intended readers\n" +
		"    focus: What to review\n" +
		"---\n\n" +
		"Review {{ prompt }} for {{ audience }}, focusing on {{ focus }}.\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestRoleViewRejectsConflictingOutputFlags(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"role", "view", "reviewer", "--json", "--markdown"})

	err := cmd.Execute()
	if err == nil || err.Error() != "--json and --markdown are mutually exclusive" {
		t.Fatalf("error = %v", err)
	}
}

func TestRoleViewReturnsRoleLoadingAndLookupErrors(t *testing.T) {
	invalidDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(invalidDir, "invalid.md"), []byte("not frontmatter"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		rolesDir string
		roleID   string
		want     string
	}{
		{name: "load", rolesDir: invalidDir, roleID: "invalid", want: "frontmatter"},
		{name: "lookup", rolesDir: t.TempDir(), roleID: "missing", want: "role \"missing\" was not found"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cmd := NewRootCommand()
			cmd.SetArgs([]string{"role", "view", test.roleID, "--roles-dir", test.rolesDir})

			err := cmd.Execute()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRoleViewWithoutOptionalMetadata(t *testing.T) {
	rolesDir := writePromptRole(t)
	cmd := NewRootCommand()

	var stdout bytes.Buffer

	cmd.SetOut(&stdout)
	cmd.SetArgs([]string{"role", "view", "reviewer", "--roles-dir", rolesDir})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, want := stdout.String(), "ID: reviewer\nAPI: callee.metalagman.dev\nKind: role\nDescription: test reviewer\nProvider type: codex\nREPL: false\nTimeout: 15m (cli default)\nParameters: none\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func writeViewRole(t *testing.T) string {
	t.Helper()

	rolesDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(rolesDir, "reviewer.md"), []byte(viewRoleMarkdown), 0o600); err != nil {
		t.Fatal(err)
	}

	return rolesDir
}
