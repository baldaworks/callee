package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
)

func TestLoadAgentsDiscoversKindsAndNestedIDs(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	writeAgent(t, project, "roles/worker.yaml", yamlRoleAgent("worker"))
	writeAgent(t, project, "roles/validator.yml", yamlRoleAgent("validator"))
	writeAgent(t, project, "workflows/pipeline.md", sequentialAgent("pipeline", []string{"roles/worker", "roles/validator"}))

	registry, err := LoadAgents(AgentLoadOptions{UserDir: filepath.Join(t.TempDir(), "missing"), ProjectDir: project})
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}

	if got, want := strings.Join(registry.IDs(), ","), "roles/validator,roles/worker,workflows/pipeline"; got != want {
		t.Errorf("registry.IDs() = %q, want %q", got, want)
	}
}

func TestNewAgentRegistryResolveAndRequiredParams(t *testing.T) {
	t.Parallel()

	worker := decodeAgent(t, "roles/worker", roleAgent("worker", map[string]string{"language": "Implementation language"}))
	validator := decodeAgent(t, "roles/validator", roleAgent("validator", nil))
	pipeline := decodeAgent(t, "workflows/pipeline", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: pipeline
  children:
    - ref: roles/worker
      alias: worker
      params:
        language: '{{ default "Go" .State.language }}'
    - ref: roles/validator
      alias: validator
---
{{ .Input }}
`)

	registry, err := NewAgentRegistry([]agent.Resource{worker, validator, pipeline})
	if err != nil {
		t.Fatalf("NewAgentRegistry() error: %v", err)
	}

	root, err := registry.Resolve("workflows/pipeline")
	if err != nil {
		t.Fatalf("Resolve() error: %v", err)
	}

	if got, want := root.Children[0].EffectiveID, "worker"; got != want {
		t.Errorf("first child effective ID = %q, want %q", got, want)
	}

	if got, want := strings.Join(root.Children[0].Path, " -> "), "workflows/pipeline -> worker"; got != want {
		t.Errorf("first child path = %q, want %q", got, want)
	}

	if got := RequiredParams(root); len(got) != 0 {
		t.Errorf("RequiredParams() = %v, want none", got)
	}
}

func TestNewAgentRegistryRejectsCycleAndEffectiveIDCollision(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		resources []agent.Resource
		want      string
	}{
		{
			name: "cycle",
			resources: []agent.Resource{
				decodeAgent(t, "a", sequentialAgent("a", []string{"b"})),
				decodeAgent(t, "b", sequentialAgent("b", []string{"a"})),
			},
			want: "cycle",
		},
		{
			name: "collision",
			resources: []agent.Resource{
				decodeAgent(t, "worker", roleAgent("worker", nil)),
				decodeAgent(t, "pipeline", sequentialAgent("pipeline", []string{"worker", "worker"})),
			},
			want: "duplicate effective ID",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := NewAgentRegistry(test.resources)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewAgentRegistry() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestLoadAgentsRejectsDuplicateIDAcrossRoots(t *testing.T) {
	t.Parallel()

	user := t.TempDir()
	project := t.TempDir()
	writeAgent(t, user, "roles/worker.md", roleAgent("user worker", nil))
	writeAgent(t, project, "roles/worker.md", roleAgent("project worker", nil))

	_, err := LoadAgents(AgentLoadOptions{UserDir: user, ProjectDir: project})
	if err == nil || !strings.Contains(err.Error(), "duplicate agent ID") {
		t.Fatalf("LoadAgents() error = %v, want duplicate agent ID", err)
	}
}

func TestLoadAgentsRejectsDuplicateIDAcrossFormats(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	writeAgent(t, project, "roles/worker.md", roleAgent("Markdown worker", nil))
	writeAgent(t, project, "roles/worker.yaml", yamlRoleAgent("YAML worker"))

	_, err := LoadAgents(AgentLoadOptions{UserDir: filepath.Join(t.TempDir(), "missing"), ProjectDir: project})
	if err == nil {
		t.Fatal("LoadAgents() error = nil, want duplicate agent ID")
	}

	for _, want := range []string{`duplicate agent ID "roles/worker"`, "roles/worker.md", "roles/worker.yaml"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadAgents() error = %v, want containing %q", err, want)
		}
	}
}

func TestLoadAgentsIgnoresUnsupportedExtensions(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	writeAgent(t, project, "roles/worker.yaml", yamlRoleAgent("worker"))
	writeAgent(t, project, "roles/ignored.json", "not valid JSON")
	writeAgent(t, project, "roles/ignored.YAML", "not valid YAML")

	configured, err := LoadAgents(AgentLoadOptions{UserDir: filepath.Join(t.TempDir(), "missing"), ProjectDir: project})
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}

	if got, want := strings.Join(configured.IDs(), ","), "roles/worker"; got != want {
		t.Errorf("registry.IDs() = %q, want %q", got, want)
	}
}

func TestLoadAgentsAggregatesStaticDiagnostics(t *testing.T) {
	t.Parallel()

	project := t.TempDir()
	writeAgent(t, project, "roles/a.md", "---\nkind: Role\nspec: {}\n---\n{{ .Input }}\n")
	writeAgent(t, project, "roles/b.md", "---\napiVersion: unsupported/v1\nkind: Role\nspec: {}\n---\n{{ .Input }}\n")

	_, err := LoadAgents(AgentLoadOptions{UserDir: filepath.Join(t.TempDir(), "missing"), ProjectDir: project})
	if err == nil {
		t.Fatal("LoadAgents() error = nil, want aggregated diagnostics")
	}

	for _, want := range []string{"roles/a.md", "roles/b.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("LoadAgents() error = %v, want containing %q", err, want)
		}
	}
}

func TestNewAgentRegistryAggregatesIndependentGraphDiagnostics(t *testing.T) {
	t.Parallel()

	first := decodeAgent(t, "workflows/a", sequentialAgent("first", []string{"roles/missing-a"}))
	second := decodeAgent(t, "workflows/b", sequentialAgent("second", []string{"roles/missing-b"}))

	_, err := NewAgentRegistry([]agent.Resource{first, second})
	if err == nil {
		t.Fatal("NewAgentRegistry() error = nil, want aggregated diagnostics")
	}

	for _, want := range []string{"roles/missing-a", "roles/missing-b"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("NewAgentRegistry() error = %v, want containing %q", err, want)
		}
	}
}

func TestResourceIDRemovesSupportedExtension(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"roles/worker.md":   "roles/worker",
		"roles/worker.yaml": "roles/worker",
		"roles/worker.yml":  "roles/worker",
	}

	for path, want := range tests {
		got, err := resourceID(filepath.FromSlash(path))
		if err != nil {
			t.Errorf("resourceID(%q) error: %v", path, err)

			continue
		}

		if got != want {
			t.Errorf("resourceID(%q) = %q, want %q", path, got, want)
		}
	}

	if _, err := resourceID("roles/worker.json"); err == nil || !strings.Contains(err.Error(), "unsupported agent file extension") {
		t.Fatalf("resourceID(.json) error = %v", err)
	}
}

func writeAgent(t *testing.T, root, relative, content string) {
	t.Helper()

	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("os.MkdirAll(%q): %v", path, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("os.WriteFile(%q): %v", path, err)
	}
}

func decodeAgent(t *testing.T, id, content string) agent.Resource {
	t.Helper()

	resource, err := agent.DecodeMarkdown(id, id+".md", []byte(content))
	if err != nil {
		t.Fatalf("agent.DecodeMarkdown(%q): %v", id, err)
	}

	return resource
}

func roleAgent(description string, params map[string]string) string {
	var declarations strings.Builder
	if len(params) > 0 {
		declarations.WriteString("  params:\n")

		for name, description := range params {
			declarations.WriteString("    " + name + ": " + description + "\n")
		}
	}

	return "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: " + description + "\n  provider:\n    type: codex\n" + declarations.String() + "---\n{{ .Input }}\n"
}

func sequentialAgent(description string, children []string) string {
	var declarations strings.Builder
	for _, child := range children {
		declarations.WriteString("    - " + child + "\n")
	}

	return "---\napiVersion: callee.metalagman.dev/v1alpha1\nkind: Sequential\nspec:\n  description: " + description + "\n  children:\n" + declarations.String() + "---\n{{ .Input }}\n"
}

func yamlRoleAgent(description string) string {
	return "apiVersion: callee.metalagman.dev/v1alpha1\nkind: Role\nspec:\n  description: " + description + "\n  provider:\n    type: codex\n  body: |\n    {{ .Input }}\n"
}
