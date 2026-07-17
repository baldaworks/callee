package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/promptkitty"
)

func TestPromptKitCommandExposesCatalogAndRoleCommands(t *testing.T) {
	cmd := NewRootCommand()

	promptKit, _, err := cmd.Find([]string{"promptkit"})
	if err != nil {
		t.Fatalf("Find(promptkit) returned unexpected error: %v", err)
	}

	commands := promptKit.Commands()

	got := make([]string, 0, len(commands))
	for _, command := range commands {
		got = append(got, command.Name())
	}

	want := []string{"list", "role", "search", "show"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("promptkit commands = %q, want %q", got, want)
	}
}

func TestPromptKitCommandGroupsShowHelpWithoutArguments(t *testing.T) {
	for _, args := range [][]string{
		{"promptkit"},
		{"promptkit", "role"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(context.Background(), args, &stdout, &stderr)
			if code != 0 {
				t.Fatalf("Run(%q) exit = %d, want 0; stderr = %q", args, code, stderr.String())
			}

			if !strings.Contains(stdout.String(), "Available Commands:") {
				t.Errorf("Run(%q) stdout does not contain command help:\n%s", args, stdout.String())
			}
		})
	}
}

func TestPromptKitCommandGroupsRejectUnknownCommands(t *testing.T) {
	for _, args := range [][]string{
		{"promptkit", "assemble"},
		{"promptkit", "setup"},
		{"promptkit", "unknown"},
		{"promptkit", "role", "unknown"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(context.Background(), args, &stdout, &stderr)
			if code != exitError {
				t.Fatalf("Run(%q) exit = %d, want %d", args, code, exitError)
			}

			if !strings.Contains(stderr.String(), "unknown command") {
				t.Errorf("Run(%q) stderr = %q, want unknown command", args, stderr.String())
			}
		})
	}
}

func TestPromptKitCatalogCommandUsesPromptKitty(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"promptkit", "show", "review-code", "--json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(promptkit show review-code --json) returned unexpected error: %v", err)
	}

	if !strings.Contains(stdout.String(), `"name": "review-code"`) {
		t.Errorf("promptkit show output does not contain review-code:\n%s", stdout.String())
	}
}

func TestPromptKitRoleCreateUsesProviderFlag(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"promptkit", "role", "create", "--help"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(promptkit role create --help) returned unexpected error: %v", err)
	}

	help := stdout.String()
	if !strings.Contains(help, "--provider string") {
		t.Errorf("promptkit role create help is missing --provider:\n%s", help)
	}

	if strings.Contains(help, "--type string") {
		t.Errorf("promptkit role create help retains --type:\n%s", help)
	}
}

func TestPromptKitRoleCreateRejectsTypeFlag(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"promptkit", "role", "create", "reviewer", "--type", "codex"})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag: --type") {
		t.Fatalf("Execute(promptkit role create --type) error = %v, want unknown flag", err)
	}
}

func TestPromptKitRoleCreateRequiresProviderFlag(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"promptkit", "role", "create", "reviewer",
		"--template", "review-code",
		"--description", "Reviews code changes.",
		"--prompt-param", "code",
	})

	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `required flag(s) "provider" not set`) {
		t.Fatalf("Execute(promptkit role create without --provider) error = %v, want required flag", err)
	}
}

func TestPromptKitSearchRanksNaturalLanguageIntent(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"promptkit", "search", "write requirements document", "--type", "template", "--json"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(promptkit search --type) returned unexpected error: %v", err)
	}

	var components []promptkitty.Component
	if err := json.Unmarshal(stdout.Bytes(), &components); err != nil {
		t.Fatalf("decode promptkit search output: %v", err)
	}

	if len(components) == 0 || components[0].Name != "author-requirements-doc" {
		t.Fatalf("promptkit search first result = %#v, want author-requirements-doc", components)
	}

	for _, component := range components {
		if component.Type != promptkitty.ComponentTemplate {
			t.Errorf("promptkit search component %q type = %q, want template", component.Name, component.Type)
		}
	}

	if strings.Contains(stdout.String(), `"score"`) {
		t.Errorf("promptkit search exposes internal score:\n%s", stdout.String())
	}
}

func TestPromptKitRoleCreate(t *testing.T) {
	t.Chdir(t.TempDir())
	contextPath := filepath.Join(t.TempDir(), "context.txt")

	contextValue := "repository context\nwith exact newline\n"
	if err := os.WriteFile(contextPath, []byte(contextValue), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) returned unexpected error: %v", contextPath, err)
	}

	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"promptkit", "role", "create", "reviewer",
		"--template", "review-code",
		"--description", "Reviews code changes.",
		"--provider", "codex",
		"--repl",
		"--prompt-param", "code",
		"--bind", "language=Go",
		"--bind-file", "context=" + contextPath,
		"--model", "gpt-5-codex",
		"--reasoning", "high",
		"--mode", "review",
		"--extra-arg", "--sandbox",
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(promptkit role create) returned unexpected error: %v", err)
	}

	if got, want := stdout.String(), "created .callee/roles/reviewer.md\n"; got != want {
		t.Errorf("promptkit role create stdout = %q, want %q", got, want)
	}

	data, err := os.ReadFile(filepath.Join(".callee", "roles", "reviewer.md"))
	if err != nil {
		t.Fatalf("os.ReadFile(generated role) returned unexpected error: %v", err)
	}

	generated, err := agent.DecodeMarkdown("reviewer", "reviewer.md", data)
	if err != nil {
		t.Fatalf("agent.DecodeMarkdown(generated role) returned unexpected error: %v", err)
	}

	if generated.APIVersion != agent.APIVersion || generated.Kind != agent.RoleKind {
		t.Fatalf("generated identity = %q/%q", generated.APIVersion, generated.Kind)
	}

	if !generated.REPL() {
		t.Fatal("generated repl = false, want true")
	}

	wantProvider := &agent.Provider{
		Type: "codex", Model: "gpt-5-codex", Reasoning: "high",
		Mode: "review", ExtraArgs: []string{"--sandbox"},
	}
	if !reflect.DeepEqual(generated.Spec.Provider, wantProvider) {
		t.Fatalf("generated provider = %#v, want %#v", generated.Spec.Provider, wantProvider)
	}

	wantParams := map[string]string{
		"additional_protocols": "Optional — specific protocols to apply (e.g., memory-safety-c, thread-safety)",
		"review_focus":         "What to focus on — e.g., correctness, security, performance, all",
	}
	if !reflect.DeepEqual(generated.Spec.Params, wantParams) {
		t.Errorf("generated params = %#v, want %#v", generated.Spec.Params, wantParams)
	}

	if got := strings.Count(generated.Spec.Body, "{{ .Input }}"); got != 1 {
		t.Errorf("generated prompt placeholder count = %d, want 1", got)
	}

	for _, name := range sortedKeys(wantParams) {
		if got := strings.Count(generated.Spec.Body, `{{ index .Params "`+name+`" }}`); got != 1 {
			t.Errorf("generated %q placeholder count = %d, want 1", name, got)
		}
	}

	for _, want := range []string{contextValue, "the user message supplied in the Runtime Input section", "the `review_focus` value supplied in the Runtime Input section"} {
		if !strings.Contains(generated.Spec.Body, want) {
			t.Errorf("generated role does not contain %q", want)
		}
	}

	if strings.Contains(generated.Spec.Body, "{{ language }}") || strings.Contains(generated.Spec.Body, "{{ context }}") {
		t.Errorf("generated role retained a compile-time binding:\n%s", generated.Spec.Body)
	}
}

func TestPromptKitRoleCreateDryRun(t *testing.T) {
	output := filepath.Join(t.TempDir(), "reviewer.md")
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"promptkit", "role", "create", "reviewer",
		"--template", "review-code",
		"--description", "Reviews code changes.",
		"--provider", "codex",
		"--prompt-param", "code",
		"--bind", "language=Go",
		"--bind", "context=repository",
		"--bind", "review_focus=all",
		"--bind", "additional_protocols=",
		"--no-format",
		"--output", output,
		"--dry-run",
	})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(promptkit role create --dry-run) returned unexpected error: %v", err)
	}

	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Errorf("dry run created %q: %v", output, err)
	}

	if !strings.Contains(stdout.String(), "# Runtime Input") || !strings.Contains(stdout.String(), "{{ .Input }}") {
		t.Errorf("dry-run output is not a role:\n%s", stdout.String())
	}

	if strings.Contains(stdout.String(), "repl:") {
		t.Errorf("dry-run output contains repl without --repl:\n%s", stdout.String())
	}
}

func TestCompilePromptKitParameters(t *testing.T) {
	descriptions := map[string]string{
		"task":     "Task",
		"audience": "Readers",
		"language": "Language",
	}

	file := filepath.Join(t.TempDir(), "language.txt")
	if err := os.WriteFile(file, []byte("Go\n"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) returned unexpected error: %v", file, err)
	}

	values, params, err := compilePromptKitParameters(descriptions, "task", []string{"audience="}, []string{"language=" + file}, "")
	if err != nil {
		t.Fatalf("compilePromptKitParameters() returned unexpected error: %v", err)
	}

	wantValues := map[string]string{"task": runtimePromptReference, "audience": "", "language": "Go\n"}
	if !reflect.DeepEqual(values, wantValues) {
		t.Errorf("compilePromptKitParameters() values = %#v, want %#v", values, wantValues)
	}

	if len(params) != 0 {
		t.Errorf("compilePromptKitParameters() runtime params = %#v, want empty", params)
	}
}

func TestCompilePromptKitParameterErrors(t *testing.T) {
	descriptions := map[string]string{"task": "Task", "audience": "Readers", "persona": "Persona"}

	tests := []struct {
		name        string
		promptParam string
		bindings    []string
		files       []string
		persona     string
		want        string
	}{
		{name: "unknown prompt", promptParam: "missing", want: "not declared"},
		{name: "persona prompt", promptParam: "persona", want: "--persona"},
		{name: "unknown binding", promptParam: "task", bindings: []string{"other=x"}, want: "not declared"},
		{name: "bound prompt", promptParam: "task", bindings: []string{"task=x"}, want: "cannot also be bound"},
		{name: "bound persona", promptParam: "task", bindings: []string{"persona=x"}, want: "--persona"},
		{name: "duplicate", promptParam: "task", bindings: []string{"audience=x", "audience=y"}, want: "more than once"},
		{name: "empty file", promptParam: "task", files: []string{"audience="}, want: "non-empty file path"},
		{name: "stdin file", promptParam: "task", files: []string{"audience=-"}, want: "not stdin"},
		{name: "missing persona", promptParam: "task", want: "configurable persona"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := compilePromptKitParameters(descriptions, test.promptParam, test.bindings, test.files, test.persona)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Errorf("compilePromptKitParameters() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestEveryPromptKitTemplateCompilesToRole(t *testing.T) {
	library, err := promptkitty.New()
	if err != nil {
		t.Fatalf("PromptKitty New() returned unexpected error: %v", err)
	}

	templates := library.List(promptkitty.Filter{Type: promptkitty.ComponentTemplate})
	if got, want := len(templates), 71; got != want {
		t.Fatalf("template count = %d, want %d", got, want)
	}

	for _, template := range templates {
		t.Run(template.Name, func(t *testing.T) {
			detail, err := library.Show(template.Name)
			if err != nil {
				t.Fatalf("Library.Show(%q) returned unexpected error: %v", template.Name, err)
			}

			descriptions, err := promptKitParameterDescriptions(detail)
			if err != nil {
				t.Fatalf("promptKitParameterDescriptions(%q) returned unexpected error: %v", template.Name, err)
			}

			promptParam := ""

			for _, name := range sortedKeys(descriptions) {
				if name != "persona" {
					promptParam = name

					break
				}
			}

			if promptParam == "" {
				t.Fatalf("template %q has no non-persona parameter", template.Name)
			}

			values, params, err := compilePromptKitParameters(descriptions, promptParam, nil, nil, "systems-engineer")
			if err != nil {
				t.Fatalf("compilePromptKitParameters(%q) returned unexpected error: %v", template.Name, err)
			}

			assembled, err := library.Assemble(promptkitty.AssembleRequest{
				Template: template.Name,
				Params:   values,
				Persona:  "systems-engineer",
			})
			if err != nil {
				t.Fatalf("Library.Assemble(%q) returned unexpected error: %v", template.Name, err)
			}

			repl := false

			generated := agent.Resource{
				ID:         template.Name,
				APIVersion: agent.APIVersion,
				Kind:       agent.RoleKind,
				Spec: agent.Spec{
					Description: template.Description,
					Provider:    &agent.Provider{Type: "codex"},
					REPL:        &repl,
					Params:      params,
					Body:        promptKitRoleBody(promptParam, params, assembled.Markdown),
				},
			}
			if err := generated.Validate(); err != nil {
				t.Errorf("generated Resource.Validate(%q) returned unexpected error: %v", template.Name, err)
			}
		})
	}
}

func TestWriteGeneratedRoleRefusesOverwrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reviewer.md")
	if err := os.WriteFile(path, []byte("existing"), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q) returned unexpected error: %v", path, err)
	}

	if err := writeGeneratedRole(path, []byte("new"), false); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("writeGeneratedRole(existing) error = %v, want already exists", err)
	}

	if err := writeGeneratedRole(path, []byte("new"), true); err != nil {
		t.Errorf("writeGeneratedRole(force) returned unexpected error: %v", err)
	}
}
