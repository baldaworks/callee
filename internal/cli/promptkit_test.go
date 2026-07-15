package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/promptkitty"
)

func TestPromptKitCommandMountsReusableCLI(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"promptkit", "--help"})

	var stdout bytes.Buffer
	cmd.SetOut(&stdout)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(promptkit --help) returned unexpected error: %v", err)
	}

	for _, command := range []string{"assemble", "list", "role", "search", "show"} {
		if !strings.Contains(stdout.String(), command) {
			t.Errorf("promptkit help does not contain %q:\n%s", command, stdout.String())
		}
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
		"--type", "codex",
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

	generated, err := role.Parse("reviewer", data)
	if err != nil {
		t.Fatalf("role.Parse(generated role) returned unexpected error: %v", err)
	}

	wantParams := map[string]string{
		"additional_protocols": "Optional — specific protocols to apply (e.g., memory-safety-c, thread-safety)",
		"review_focus":         "What to focus on — e.g., correctness, security, performance, all",
	}
	if !reflect.DeepEqual(generated.Metadata.Params, wantParams) {
		t.Errorf("generated params = %#v, want %#v", generated.Metadata.Params, wantParams)
	}

	if got := strings.Count(generated.Template, "{{ prompt }}"); got != 1 {
		t.Errorf("generated prompt placeholder count = %d, want 1", got)
	}

	for _, name := range sortedKeys(wantParams) {
		if got := strings.Count(generated.Template, "{{ "+name+" }}"); got != 1 {
			t.Errorf("generated %q placeholder count = %d, want 1", name, got)
		}
	}

	for _, want := range []string{contextValue, "the user message supplied in the Runtime Input section", "the `review_focus` value supplied in the Runtime Input section"} {
		if !strings.Contains(generated.Template, want) {
			t.Errorf("generated role does not contain %q", want)
		}
	}

	if strings.Contains(generated.Template, "{{ language }}") || strings.Contains(generated.Template, "{{ context }}") {
		t.Errorf("generated role retained a compile-time binding:\n%s", generated.Template)
	}
}

func TestPromptKitRoleCreateDryRun(t *testing.T) {
	output := filepath.Join(t.TempDir(), "reviewer.md")
	cmd := NewRootCommand()
	cmd.SetArgs([]string{
		"promptkit", "role", "create", "reviewer",
		"--template", "review-code",
		"--description", "Reviews code changes.",
		"--type", "codex",
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

	if !strings.Contains(stdout.String(), "# Runtime Input") || !strings.Contains(stdout.String(), "{{ prompt }}") {
		t.Errorf("dry-run output is not a role:\n%s", stdout.String())
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

			generated := role.Role{
				ID:       template.Name,
				Metadata: role.Metadata{Description: template.Description, Type: "codex", Params: params},
				Template: promptKitRoleBody(promptParam, params, assembled.Markdown),
			}
			if err := generated.Validate(); err != nil {
				t.Errorf("generated Role.Validate(%q) returned unexpected error: %v", template.Name, err)
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
