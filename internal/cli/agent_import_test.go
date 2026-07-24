package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

func TestAgentImportCommandImportsDefaultRemoteRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	repo := writeImportRepository(t, map[string]string{
		".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Imported worker.
  provider:
    type: codex
---
{{ .Input }}
`,
		".callee/workflows/pipeline.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Imported pipeline.
  children:
    - ref: roles/worker
      alias: worker
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	for _, want := range []string{
		"Created imported agents:",
		filepath.Join(defaultProjectAgentDir, "roles", "worker.md"),
		filepath.Join(defaultProjectAgentDir, "workflows", "pipeline.md"),
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("stdout = %q, want containing %q", stdout.String(), want)
		}
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}

	configured, err := registry.LoadAgents(registry.AgentLoadOptions{
		UserDir:    filepath.Join(project, "missing-user"),
		ProjectDir: filepath.Join(project, defaultProjectAgentDir),
	})
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}

	if got, want := configured.IDs(), []string{"roles/worker", "workflows/pipeline"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("agent IDs = %v, want %v", got, want)
	}
}

func TestAgentImportCommandSupportsNestedRemotePath(t *testing.T) {
	isolateAgentRoots(t)
	repo := writeImportRepository(t, map[string]string{
		"catalog/team-a/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Nested worker.
  provider:
    type: codex
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git", "--path", "catalog/team-a"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got := stdout.String(); !strings.Contains(got, filepath.Join(defaultProjectAgentDir, "roles", "worker.md")) {
		t.Fatalf("stdout = %q, want imported worker path", got)
	}
}

func TestAgentImportCommandExpandsGitHubRepositoryShorthand(t *testing.T) {
	isolateAgentRoots(t)
	repo := writeImportRepository(t, map[string]string{
		".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Imported worker.
  provider:
    type: codex
---
{{ .Input }}
`,
	})

	var gotRepoURL string

	stubCloneAgentImportRepository(t, repo, func(_ context.Context, repoURL, ref string) (string, error) {
		gotRepoURL = repoURL

		return repo, nil
	})

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "acme/platform-agents"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got, want := gotRepoURL, "https://github.com/acme/platform-agents.git"; got != want {
		t.Fatalf("clone repo URL = %q, want %q", got, want)
	}
}

func TestAgentImportCommandRewritesPrefixedInternalRefsOnly(t *testing.T) {
	project := isolateAgentRoots(t)
	writeVersionedAgent(t, filepath.Join(project, defaultProjectAgentDir), "roles/external.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: External local role.
  provider:
    type: codex
---
{{ .Input }}
`)

	repo := writeImportRepository(t, map[string]string{
		".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Imported worker.
  provider:
    type: codex
---
{{ .Input }}
`,
		".callee/workflows/pipeline.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Imported pipeline.
  children:
    - ref: roles/worker
      alias: worker
    - ref: roles/external
      alias: external
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git", "--prefix", "vendor"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	configured, err := registry.LoadAgents(registry.AgentLoadOptions{
		UserDir:    filepath.Join(project, "missing-user"),
		ProjectDir: filepath.Join(project, defaultProjectAgentDir),
	})
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}

	imported, err := configured.GetAgent("vendor/workflows/pipeline")
	if err != nil {
		t.Fatalf("GetAgent() error: %v", err)
	}

	if got, want := imported.Spec.Children[0].Ref, "vendor/roles/worker"; got != want {
		t.Fatalf("internal child ref = %q, want %q", got, want)
	}

	if got, want := imported.Spec.Children[1].Ref, "roles/external"; got != want {
		t.Fatalf("external child ref = %q, want %q", got, want)
	}
}

func TestAgentImportCommandPreservesExistingDestinationsWithoutForce(t *testing.T) {
	project := isolateAgentRoots(t)
	writeVersionedAgent(t, filepath.Join(project, defaultProjectAgentDir), "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Existing worker.
  provider:
    type: codex
---
{{ .Input }}
`)

	repo := writeImportRepository(t, map[string]string{
		".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Imported worker.
  provider:
    type: codex
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got := stdout.String(); !strings.Contains(got, "Existing imported agents left unchanged:") || !strings.Contains(got, filepath.Join(defaultProjectAgentDir, "roles", "worker.md")) {
		t.Fatalf("stdout = %q, want unchanged destination report", got)
	}

	content, err := os.ReadFile(filepath.Join(project, defaultProjectAgentDir, "roles", "worker.md"))
	if err != nil {
		t.Fatalf("os.ReadFile() error: %v", err)
	}

	if !strings.Contains(string(content), "description: Existing worker.") {
		t.Fatalf("content = %q, want preserved local file", string(content))
	}
}

func TestAgentImportCommandForceOverwritesOnlySelectedFiles(t *testing.T) {
	project := isolateAgentRoots(t)
	root := filepath.Join(project, defaultProjectAgentDir)
	writeVersionedAgent(t, root, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Existing worker.
  provider:
    type: codex
---
{{ .Input }}
`)
	writeVersionedAgent(t, root, "roles/untouched.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Untouched worker.
  provider:
    type: codex
---
{{ .Input }}
`)

	repo := writeImportRepository(t, map[string]string{
		".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Overwritten worker.
  provider:
    type: codex
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git", "--force"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if got := stdout.String(); !strings.Contains(got, "Overwritten imported agents:") || !strings.Contains(got, filepath.Join(defaultProjectAgentDir, "roles", "worker.md")) {
		t.Fatalf("stdout = %q, want overwritten destination report", got)
	}

	workerContent, err := os.ReadFile(filepath.Join(root, "roles", "worker.md"))
	if err != nil {
		t.Fatalf("os.ReadFile(worker) error: %v", err)
	}

	if !strings.Contains(string(workerContent), "description: Overwritten worker.") {
		t.Fatalf("worker content = %q, want overwritten file", string(workerContent))
	}

	untouchedContent, err := os.ReadFile(filepath.Join(root, "roles", "untouched.md"))
	if err != nil {
		t.Fatalf("os.ReadFile(untouched) error: %v", err)
	}

	if !strings.Contains(string(untouchedContent), "description: Untouched worker.") {
		t.Fatalf("untouched content = %q, want unchanged unrelated file", string(untouchedContent))
	}
}

func TestAgentImportCommandValidationFailureLeavesTreeUnchanged(t *testing.T) {
	project := isolateAgentRoots(t)
	repo := writeImportRepository(t, map[string]string{
		".callee/workflows/broken.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Broken workflow.
  children:
    - ref: roles/missing
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git"}, &stdout, &stderr)
	if exitCode != exitError {
		t.Fatalf("agent import exit = %d, want %d; stderr = %q", exitCode, exitError, stderr.String())
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), `validate imported agents`) {
		t.Fatalf("stderr = %q, want validation failure", stderr.String())
	}

	if _, err := os.Stat(filepath.Join(project, defaultProjectAgentDir, "workflows", "broken.md")); !os.IsNotExist(err) {
		t.Fatalf("broken workflow exists after failed import: %v", err)
	}
}

func TestAgentImportCommandRejectsConflictsWithUserRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	userRoot := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "callee")
	writeVersionedAgent(t, userRoot, "roles/worker.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: User worker.
  provider:
    type: codex
---
{{ .Input }}
`)

	repo := writeImportRepository(t, map[string]string{
		".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Imported worker.
  provider:
    type: codex
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git"}, &stdout, &stderr)
	if exitCode != exitError {
		t.Fatalf("agent import exit = %d, want %d; stderr = %q", exitCode, exitError, stderr.String())
	}

	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), `duplicate agent ID "roles/worker"`) {
		t.Fatalf("stderr = %q, want duplicate ID error", stderr.String())
	}

	if _, err := os.Stat(filepath.Join(project, defaultProjectAgentDir, "roles", "worker.md")); !os.IsNotExist(err) {
		t.Fatalf("project worker exists after failed import: %v", err)
	}
}

func TestAgentImportCommandReportsFlagAndRemoteErrors(t *testing.T) {
	isolateAgentRoots(t)

	t.Run("invalid prefix", func(t *testing.T) {
		stubCloneAgentImportRepository(t, "", func(context.Context, string, string) (string, error) {
			t.Fatal("cloneAgentImportRepository was called for invalid prefix")

			return "", nil
		})

		var stdout, stderr bytes.Buffer

		exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git", "--prefix", "../bad"}, &stdout, &stderr)
		if exitCode != exitError {
			t.Fatalf("agent import exit = %d, want %d", exitCode, exitError)
		}

		if !strings.Contains(stderr.String(), "invalid --prefix") {
			t.Fatalf("stderr = %q, want invalid prefix error", stderr.String())
		}
	})

	t.Run("missing remote path", func(t *testing.T) {
		repo := writeImportRepository(t, map[string]string{
			".callee/roles/worker.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Worker.
  provider:
    type: codex
---
{{ .Input }}
`,
		})
		stubCloneAgentImportRepository(t, repo, nil)

		var stdout, stderr bytes.Buffer

		exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git", "--path", "missing"}, &stdout, &stderr)
		if exitCode != exitError {
			t.Fatalf("agent import exit = %d, want %d", exitCode, exitError)
		}

		if !strings.Contains(stderr.String(), `remote path "missing" does not exist`) {
			t.Fatalf("stderr = %q, want missing remote path error", stderr.String())
		}
	})

	t.Run("git unavailable", func(t *testing.T) {
		stubCloneAgentImportRepository(t, "", func(context.Context, string, string) (string, error) {
			return "", fmt.Errorf("git is required to import remote agents: executable file not found in $PATH")
		})

		var stdout, stderr bytes.Buffer

		exitCode := Run(context.Background(), []string{"agent", "import", "https://example.invalid/repo.git"}, &stdout, &stderr)
		if exitCode != exitError {
			t.Fatalf("agent import exit = %d, want %d", exitCode, exitError)
		}

		if !strings.Contains(stderr.String(), "git is required to import remote agents") {
			t.Fatalf("stderr = %q, want git unavailable error", stderr.String())
		}
	})
}

func TestAgentImportCommandSupportsExclusiveAgentRoot(t *testing.T) {
	project := isolateAgentRoots(t)
	customRoot := filepath.Join(project, "agents")
	writeVersionedAgent(t, customRoot, "roles/external.md", `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: External role in exclusive root.
  provider:
    type: codex
---
{{ .Input }}
`)

	repo := writeImportRepository(t, map[string]string{
		".callee/workflows/pipeline.md": `---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Pipeline.
  children:
    - ref: roles/external
      alias: external
---
{{ .Input }}
`,
	})
	stubCloneAgentImportRepository(t, repo, nil)

	var stdout, stderr bytes.Buffer

	exitCode := Run(context.Background(), []string{"--agent-root", customRoot, "agent", "import", "https://example.invalid/repo.git"}, &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("agent import exit = %d, stderr = %q", exitCode, stderr.String())
	}

	if !strings.Contains(stdout.String(), filepath.Join(customRoot, "workflows", "pipeline.md")) {
		t.Fatalf("stdout = %q, want custom root destination", stdout.String())
	}

	configured, err := registry.LoadAgents(registry.AgentLoadOptions{ExclusiveDir: customRoot})
	if err != nil {
		t.Fatalf("LoadAgents() error: %v", err)
	}

	if _, err := configured.GetAgent("workflows/pipeline"); err != nil {
		t.Fatalf("GetAgent() error: %v", err)
	}
}

func stubCloneAgentImportRepository(t *testing.T, repo string, fn func(context.Context, string, string) (string, error)) {
	t.Helper()

	original := cloneAgentImportRepository

	t.Cleanup(func() { cloneAgentImportRepository = original })

	if fn != nil {
		cloneAgentImportRepository = fn

		return
	}

	cloneAgentImportRepository = func(context.Context, string, string) (string, error) {
		return repo, nil
	}
}

func writeImportRepository(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()
	for relative, content := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("os.MkdirAll(%q): %v", path, err)
		}

		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("os.WriteFile(%q): %v", path, err)
		}
	}

	return root
}

func TestCleanImportPrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{name: "empty", input: "", want: ""},
		{name: "single", input: "vendor", want: "vendor"},
		{name: "nested", input: "vendor/team", want: "vendor/team"},
		{name: "dot", input: ".", wantErr: "empty or dot segment"},
		{name: "parent", input: "../bad", wantErr: "empty or dot segment"},
		{name: "dot segment", input: "vendor/../bad", wantErr: "empty or dot segment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := cleanImportPrefix(test.input)
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("cleanImportPrefix(%q) error = %v, want containing %q", test.input, err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("cleanImportPrefix(%q) error: %v", test.input, err)
			}

			if got != test.want {
				t.Fatalf("cleanImportPrefix(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestNormalizeAgentImportRepository(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "github shorthand",
			input: "acme/platform-agents",
			want:  "https://github.com/acme/platform-agents.git",
		},
		{
			name:  "https url unchanged",
			input: "https://github.com/acme/platform-agents.git",
			want:  "https://github.com/acme/platform-agents.git",
		},
		{
			name:  "ssh url unchanged",
			input: "git@github.com:acme/platform-agents.git",
			want:  "git@github.com:acme/platform-agents.git",
		},
		{
			name:  "dot relative path unchanged",
			input: "./platform-agents",
			want:  "./platform-agents",
		},
		{
			name:  "parent relative path unchanged",
			input: "../platform-agents",
			want:  "../platform-agents",
		},
		{
			name:  "absolute path unchanged",
			input: "/tmp/platform-agents",
			want:  "/tmp/platform-agents",
		},
		{
			name:  "git suffix unchanged",
			input: "acme/platform-agents.git",
			want:  "acme/platform-agents.git",
		},
		{
			name:  "deep path unchanged",
			input: "acme/platform-agents/catalog",
			want:  "acme/platform-agents/catalog",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeAgentImportRepository(test.input); got != test.want {
				t.Fatalf("normalizeAgentImportRepository(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestEncodeImportedAgentFilePreservesYAMLExtension(t *testing.T) {
	t.Parallel()

	decoded := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.RoleKind,
		Spec: agent.Spec{
			Description: "Worker",
			Provider:    &agent.Provider{Type: "codex"},
			Body:        "{{ .Input }}",
		},
		ID: "roles/worker",
	}

	for _, extension := range []string{".yaml", ".yml"} {
		content, err := encodeImportedAgentFile(decoded, extension)
		if err != nil {
			t.Fatalf("encodeImportedAgentFile(%q) error: %v", extension, err)
		}

		if !strings.Contains(string(content), "body:") {
			t.Fatalf("encoded YAML = %q, want inline spec.body", string(content))
		}
	}
}
