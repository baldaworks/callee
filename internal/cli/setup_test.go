package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

func TestSetupCommandInstallsPluginAndStarterAgents(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantCommands [][]string
		wantProvider string
	}{
		{
			name:   "codex",
			target: "codex",
			wantCommands: [][]string{
				{"codex", "plugin", "marketplace", "remove", "callee"},
				{"codex", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"codex", "plugin", "add", "callee@callee"},
			},
			wantProvider: "codex",
		},
		{
			name:   "claude",
			target: "claude",
			wantCommands: [][]string{
				{"claude", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"claude", "plugin", "install", "callee@callee", "--scope", "project"},
			},
			wantProvider: "claude",
		},
		{
			name:   "grok",
			target: "grok",
			wantCommands: [][]string{
				{"grok", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"grok", "plugin", "install", "callee@callee", "--trust"},
			},
			wantProvider: "grok",
		},
		{
			name:   "copilot",
			target: "copilot",
			wantCommands: [][]string{
				{"copilot", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"copilot", "plugin", "install", "callee@callee"},
			},
			wantProvider: "copilot",
		},
		{
			name:         "opencode",
			target:       "opencode",
			wantProvider: "opencode",
		},
		{
			name:         "cursor",
			target:       "cursor",
			wantProvider: "cursor",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)

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

			configured, err := registry.LoadAgents(registry.AgentLoadOptions{
				UserDir:    filepath.Join(root, "missing-user-agents"),
				ProjectDir: filepath.Join(root, ".callee"),
			})
			if err != nil {
				t.Fatal(err)
			}

			wantIDs := []string{
				"roles/architect",
				"roles/explorer",
				"roles/implementer",
				"roles/reviewer",
				"workflows/goalkeeper",
				"workflows/investigate",
			}
			if got := configured.IDs(); !reflect.DeepEqual(got, wantIDs) {
				t.Fatalf("agent IDs = %#v, want %#v", got, wantIDs)
			}

			assertStarterRoleProviders(t, configured, wantIDs[:4], test.wantProvider)

			assertStarterWorkflowTree(t, configured, "workflows/investigate", agent.SequentialKind, []string{"explorer", "architect"})
			assertStarterWorkflowTree(t, configured, "workflows/goalkeeper", agent.LoopKind, []string{"worker", "validator"})
			assertStarterInstallOutput(t, stdout.String())
		})
	}
}

func assertStarterRoleProviders(t *testing.T, configured *registry.AgentRegistry, ids []string, providerType string) {
	t.Helper()

	for _, id := range ids {
		role, err := configured.GetAgent(id)
		if err != nil {
			t.Fatal(err)
		}

		want := &agent.Provider{Type: providerType}
		if !reflect.DeepEqual(role.Spec.Provider, want) {
			t.Errorf("%s provider = %#v, want %#v", id, role.Spec.Provider, want)
		}
	}
}

func assertStarterInstallOutput(t *testing.T, output string) {
	t.Helper()

	for _, want := range []string{
		"Installed starter agents for ",
		".callee/roles/reviewer.md",
		".callee/workflows/investigate.md",
		"callee agent list",
		"callee agent view workflows/investigate",
		"callee agent run workflows/investigate",
		"workflows/goalkeeper for iterative implementation and review",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("stdout = %q, want containing %q", output, want)
		}
	}

	if strings.Contains(output, "resource") {
		t.Fatalf("stdout = %q", output)
	}
}

func assertStarterWorkflowTree(t *testing.T, configured *registry.AgentRegistry, id string, kind agent.Kind, childIDs []string) {
	t.Helper()

	root, err := configured.Resolve(id)
	if err != nil {
		t.Fatal(err)
	}

	if root.Kind != kind {
		t.Errorf("%s kind = %q, want %q", id, root.Kind, kind)
	}

	got := make([]string, 0, len(root.Children))
	for _, child := range root.Children {
		got = append(got, child.EffectiveID)
	}

	if !reflect.DeepEqual(got, childIDs) {
		t.Errorf("%s children = %#v, want %#v", id, got, childIDs)
	}
}

func TestPrepareCodexMarketplaceIgnoresMissingRegistration(t *testing.T) {
	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(_ context.Context, _ io.Writer, stderr io.Writer, _ string, _ ...string) error {
		_, _ = fmt.Fprintln(stderr, "Error: marketplace `callee` is not configured or installed")

		return errors.New("exit status 1")
	}

	var stderr bytes.Buffer
	if err := prepareCodexMarketplace(context.Background(), &stderr); err != nil {
		t.Fatal(err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestPrepareCodexMarketplaceReportsUnexpectedFailure(t *testing.T) {
	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(_ context.Context, _ io.Writer, stderr io.Writer, _ string, _ ...string) error {
		_, _ = fmt.Fprintln(stderr, "permission denied")

		return errors.New("exit status 1")
	}

	var stderr bytes.Buffer

	err := prepareCodexMarketplace(context.Background(), &stderr)
	if err == nil || !strings.Contains(err.Error(), "remove existing Codex marketplace") {
		t.Fatalf("prepareCodexMarketplace() error = %v", err)
	}

	if !strings.Contains(stderr.String(), "permission denied") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestOpenCodeSetupInstallsSkillsAndCommands(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "opencode"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, asset := range openCodeAssetFiles {
		want, err := localIntegrationAssets.ReadFile(asset.source)
		if err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(filepath.FromSlash(asset.destination))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from embedded asset %s", asset.destination, asset.source)
		}
	}
}

func TestOpenCodeSetupPreservesAssetsUnlessForced(t *testing.T) {
	testLocalIntegrationPreservesAssetsUnlessForced(
		t,
		".opencode/commands/callee.md",
		"assets/opencode/commands/callee.md",
		openCodeAssetFiles,
		writeOpenCodeIntegration,
	)
}

func TestCursorSetupInstallsSkills(t *testing.T) {
	t.Chdir(t.TempDir())

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "cursor"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, asset := range cursorAssetFiles {
		want, err := localIntegrationAssets.ReadFile(asset.source)
		if err != nil {
			t.Fatal(err)
		}

		got, err := os.ReadFile(filepath.FromSlash(asset.destination))
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from embedded asset %s", asset.destination, asset.source)
		}
	}
}

func TestCursorSetupPreservesSkillsUnlessForced(t *testing.T) {
	testLocalIntegrationPreservesAssetsUnlessForced(
		t,
		".cursor/skills/callee-run-agent/SKILL.md",
		"assets/cursor/skills/callee-run-agent/SKILL.md",
		cursorAssetFiles,
		writeCursorIntegration,
	)
}

func testLocalIntegrationPreservesAssetsUnlessForced(
	t *testing.T,
	destination string,
	source string,
	assets []localIntegrationAsset,
	writeIntegration func(bool) (setupInstallResult, error),
) {
	t.Helper()
	t.Chdir(t.TempDir())

	path := filepath.FromSlash(destination)
	if err := os.MkdirAll(filepath.Dir(path), setupDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("custom"), setupFileMode); err != nil {
		t.Fatal(err)
	}

	result, err := writeIntegration(false)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.created) != len(assets)-1 || len(result.unchanged) != 1 {
		t.Fatalf("first install result = %#v", result)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "custom" {
		t.Fatalf("custom asset = %q", got)
	}

	result, err = writeIntegration(true)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.created) != len(assets) || len(result.unchanged) != 0 {
		t.Fatalf("forced install result = %#v", result)
	}

	want, err := localIntegrationAssets.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}

	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("forced asset = %q, want %q", got, want)
	}
}

func TestLocalSkillAssetsMatchPluginSkills(t *testing.T) {
	for source, pluginPath := range map[string]string{
		"assets/opencode/skills/callee-run-agent/SKILL.md":    filepath.Join("..", "..", "plugins", "callee", "prefixed-skills", "callee-run-agent", "SKILL.md"),
		"assets/opencode/skills/callee-create-agent/SKILL.md": filepath.Join("..", "..", "plugins", "callee", "prefixed-skills", "callee-create-agent", "SKILL.md"),
		"assets/cursor/skills/callee-run-agent/SKILL.md":      filepath.Join("..", "..", "plugins", "callee", "prefixed-skills", "callee-run-agent", "SKILL.md"),
		"assets/cursor/skills/callee-create-agent/SKILL.md":   filepath.Join("..", "..", "plugins", "callee", "prefixed-skills", "callee-create-agent", "SKILL.md"),
	} {
		want, err := os.ReadFile(pluginPath)
		if err != nil {
			t.Fatal(err)
		}

		got, err := localIntegrationAssets.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("local integration asset %s differs from %s", source, pluginPath)
		}
	}
}

func TestOpenCodeCommandAssetsLoadTheMatchingSkill(t *testing.T) {
	for source, skill := range map[string]string{
		"assets/opencode/commands/callee.md":              "callee-run-agent",
		"assets/opencode/commands/callee-create-agent.md": "callee-create-agent",
	} {
		data, err := localIntegrationAssets.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range []string{"Load the `" + skill + "` skill", "$ARGUMENTS"} {
			if !strings.Contains(string(data), want) {
				t.Errorf("OpenCode command %s is missing %q", source, want)
			}
		}
	}
}

func TestSetupCommandPreservesExistingAgentAndAddsMissingAgents(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	path := filepath.FromSlash(".callee/roles/reviewer.md")
	if err := os.MkdirAll(filepath.Dir(path), setupDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("existing"), setupFileMode); err != nil {
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

	for _, file := range starterAgentFiles {
		if file.destination == ".callee/roles/reviewer.md" {
			continue
		}

		if _, err := os.Stat(filepath.FromSlash(file.destination)); err != nil {
			t.Errorf("missing starter agent %s: %v", file.destination, err)
		}
	}

	for _, want := range []string{"Existing starter agents left unchanged:", ".callee/roles/reviewer.md"} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout = %q, want containing %q", stdout.String(), want)
		}
	}

	if _, err := registry.LoadAgents(registry.AgentLoadOptions{
		UserDir:    filepath.Join(root, "missing-user-agents"),
		ProjectDir: filepath.Join(root, ".callee"),
	}); err == nil || !strings.Contains(err.Error(), "reviewer") {
		t.Fatalf("LoadAgents() error = %v, want preserved invalid reviewer to remain visible to doctor", err)
	}
}

func TestSetupCommandForceReplacesExistingStarterAgents(t *testing.T) {
	t.Chdir(t.TempDir())

	path := filepath.FromSlash(".callee/roles/reviewer.md")
	if err := os.MkdirAll(filepath.Dir(path), setupDirMode); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("existing"), setupFileMode); err != nil {
		t.Fatal(err)
	}

	original := runSetupCommand

	t.Cleanup(func() { runSetupCommand = original })

	runSetupCommand = func(context.Context, io.Writer, io.Writer, string, ...string) error { return nil }

	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "codex", "--force"})

	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	role, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if string(role) == "existing" {
		t.Fatal("reviewer role was not replaced")
	}
}

func TestStarterAgentAssetsMatchExamples(t *testing.T) {
	for _, file := range starterAgentFiles {
		want, err := starterAgentAssets.ReadFile(file.source)
		if err != nil {
			t.Fatal(err)
		}

		examplePath := filepath.Join("..", "..", "examples", strings.TrimPrefix(file.source, starterAssetRoot))

		got, err := os.ReadFile(examplePath)
		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(got, want) {
			t.Errorf("example %s differs from embedded starter agent %s", examplePath, file.source)
		}
	}
}

func TestCheckedInExamplesFormValidAgentRegistry(t *testing.T) {
	configured, err := registry.LoadAgents(registry.AgentLoadOptions{
		UserDir:    filepath.Join(t.TempDir(), "missing-user-agents"),
		ProjectDir: filepath.Join("..", "..", "examples"),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertStarterWorkflowTree(t, configured, "workflows/investigate", agent.SequentialKind, []string{"explorer", "architect"})
	assertStarterWorkflowTree(t, configured, "workflows/goalkeeper", agent.LoopKind, []string{"worker", "validator"})
}

func TestWriteStarterAgentsIsRepeatable(t *testing.T) {
	t.Chdir(t.TempDir())

	first, err := writeStarterAgents("codex", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(first.created) != len(starterAgentFiles) || len(first.unchanged) != 0 {
		t.Fatalf("first install result = %#v", first)
	}

	second, err := writeStarterAgents("codex", false)
	if err != nil {
		t.Fatal(err)
	}

	if len(second.created) != 0 || len(second.unchanged) != len(starterAgentFiles) {
		t.Fatalf("second install result = %#v", second)
	}
}

func TestWriteStarterAgentsValidatesBeforeWriting(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	if _, err := writeStarterAgents("unknown", false); err == nil || !strings.Contains(err.Error(), "embedded starter agent") {
		t.Fatalf("writeStarterAgents() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, ".callee")); !os.IsNotExist(err) {
		t.Fatalf("starter directory exists after validation failure: %v", err)
	}
}

func TestSetupCommandRejectsUnknownTarget(t *testing.T) {
	cmd := NewRootCommand()
	cmd.SetArgs([]string{"setup", "other"})

	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "unsupported setup target") {
		t.Fatalf("setup error = %v", err)
	}
}
