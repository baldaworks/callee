package callee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseVersion = "0.10.0"

func TestSkillUsesOnlyTheCLI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "run-role", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: run-role",
		"npx --yes @baldaworks/callee@" + releaseVersion,
		"callee agent list --json",
		"callee agent view \"<agent-id>\" --json",
		"callee agent run \"<agent-id>\"",
		"--param \"<effective-node-id>.<name>=<value>\"",
		"real controlling PTY",
		"Keep terminal interaction separate from stdout and stderr.",
		"Do not send `quit`, `exit`, `/done`",
		"artifact is written to stdout only after provider cleanup succeeds",
		"does not define `Parallel`",
		"setup <codex|claude|grok|copilot|opencode>",
		"Do not add Gemini",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{"callee exec", "callee role", "--thread-id", "--role", "type: gemini"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("skill retains removed or user-visible syntax %q", forbidden)
		}
	}
}

func TestPromptKitSkillAuthorsRolesThroughTheCLI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-role", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: create-role",
		"npx --yes @baldaworks/callee@" + releaseVersion,
		"callee promptkit role create",
		"apiVersion: callee.metalagman.dev/v1alpha1",
		"kind: Role",
		"spec.provider",
		"{{ .Input }}",
		"{{ .Params.focus }}",
		"exactly one unconditional bare",
		"callee agent view \"<resource-id>\" --json",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("PromptKit skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{"type: gemini", "api: callee", "kind: role", "{{ prompt }}"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("PromptKit skill contains forbidden syntax %q", forbidden)
		}
	}
}

func TestPluginContainsOnlyTheNamedSkills(t *testing.T) {
	for directory, names := range map[string][]string{
		"skills":          {"create-role", "run-role"},
		"prefixed-skills": {"callee-create-role", "callee-run-role"},
	} {
		entries, err := os.ReadDir(directory)
		if err != nil {
			t.Fatal(err)
		}

		if len(entries) != len(names) {
			t.Fatalf("%s contains %d entries, want %d", directory, len(entries), len(names))
		}

		for i, entry := range entries {
			if entry.Name() != names[i] {
				t.Errorf("%s entry %d = %q, want %q", directory, i, entry.Name(), names[i])
			}
		}
	}
}

func TestSkillVariantsHaveMatchingBodies(t *testing.T) {
	for shortName, prefixedName := range map[string]string{
		"create-role": "callee-create-role",
		"run-role":    "callee-run-role",
	} {
		paths := []string{
			filepath.Join("skills", shortName, "SKILL.md"),
			filepath.Join("prefixed-skills", prefixedName, "SKILL.md"),
			filepath.Join("..", "..", "internal", "cli", "assets", "opencode", "skills", prefixedName, "SKILL.md"),
		}

		var want string

		for _, path := range paths {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			body := skillBody(t, data)
			if want == "" {
				want = body

				continue
			}

			if body != want {
				t.Errorf("%s body differs from %s", path, paths[0])
			}
		}
	}
}

func skillBody(t *testing.T, data []byte) string {
	t.Helper()

	parts := strings.SplitN(string(data), "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		t.Fatal("skill has invalid YAML frontmatter delimiters")
	}

	return strings.TrimSpace(parts[2])
}

func TestCodexSkillMetadataUsesPublicNames(t *testing.T) {
	for path, fields := range map[string][]string{
		filepath.Join("skills", "run-role", "agents", "openai.yaml"): {
			`display_name: "Callee Run Role"`,
			`short_description: "Run and combine project-defined Callee roles"`,
			`default_prompt: "Use $callee:run-role to review the current changes."`,
		},
		filepath.Join("skills", "create-role", "agents", "openai.yaml"): {
			`display_name: "Callee Create Role"`,
			`short_description: "Create Callee roles from PromptKit templates"`,
			`default_prompt: "Use $callee:create-role to create a Go code-review role for Codex."`,
		},
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		for _, field := range fields {
			if !strings.Contains(string(data), field) {
				t.Errorf("%s is missing %q", path, field)
			}
		}
	}
}

func TestPublicMetadataUsesProviderAwarePositioning(t *testing.T) {
	for path, want := range map[string]string{
		filepath.Join("..", "..", "README.md"):                             "## Markdown-defined agents and deterministic workflows",
		filepath.Join(".claude-plugin", "plugin.json"):                     "Run provider-aware subagent roles described in Markdown.",
		filepath.Join(".codex-plugin", "plugin.json"):                      "Provider-aware subagent roles in Markdown.",
		filepath.Join(".grok-plugin", "plugin.json"):                       "Run provider-aware subagent roles described in Markdown.",
		filepath.Join(".plugin", "plugin.json"):                            "Run provider-aware subagent roles described in Markdown.",
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"):    "Provider-aware Markdown subagent roles for Claude Code.",
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"):      "Marketplace for provider-aware Markdown subagent roles in Grok Build.",
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"): "Provider-aware Markdown subagent roles for GitHub Copilot CLI.",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(data), want) {
			t.Errorf("%s is missing positioning %q", path, want)
		}
	}

	for _, path := range []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		for _, outdated := range []string{
			"Versioned Markdown roles for AI coding agents",
			"Route natural-language tasks through Callee roles.",
		} {
			if strings.Contains(string(data), outdated) {
				t.Errorf("%s retains outdated positioning %q", path, outdated)
			}
		}
	}
}

func TestREADMEPresentsHostsEqually(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	hosts := []struct {
		name   string
		target string
	}{
		{name: "Codex", target: "codex"},
		{name: "Claude Code", target: "claude"},
		{name: "Grok Build", target: "grok"},
		{name: "Copilot CLI", target: "copilot"},
		{name: "OpenCode", target: "opencode"},
	}

	previous := -1

	for _, host := range hosts {
		row := "| " + host.name + " | `callee setup " + host.target + "` |"
		index := strings.Index(text, row)

		if index < 0 {
			t.Errorf("README is missing setup row %q", row)

			continue
		}

		if index <= previous {
			t.Errorf("README places %s outside the canonical host order", host.name)
		}

		previous = index
	}

	for _, forbidden := range []string{
		"--sparse",
		"setup <host>",
		"@0.10.0 setup",
		"Flat frontmatter",
		"For Codex:",
		"callee exec --role",
		"{{ prompt }}",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("README contains host-biased or stale text %q", forbidden)
		}
	}

	for _, want := range []string{
		"apiVersion: callee.metalagman.dev/v1alpha1",
		"kind: Role",
		"callee agent run workflows/goalkeeper",
		"callee doctor --graph mermaid",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("README is missing audited text %q", want)
		}
	}
}

func TestPluginHasNoLegacyCommands(t *testing.T) {
	for _, name := range []string{"role.md", "reset.md", "setup.md", "subagent.md"} {
		if _, err := os.Stat(filepath.Join("commands", name)); !os.IsNotExist(err) {
			t.Errorf("removed command %s exists", name)
		}
	}
}

func TestPluginManifestsExposeHostAppropriateSkills(t *testing.T) {
	for path, want := range map[string]string{
		filepath.Join(".claude-plugin", "plugin.json"): `"skills": "./skills/"`,
		filepath.Join(".codex-plugin", "plugin.json"):  `"skills": "./skills/"`,
		filepath.Join(".grok-plugin", "plugin.json"):   `"skills": "./prefixed-skills/"`,
		filepath.Join(".plugin", "plugin.json"):        `"skills": "./prefixed-skills/"`,
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(data), want) {
			t.Errorf("%s does not expose %s", path, want)
		}
	}
}

func TestPluginAssetsHaveNoMCPConfiguration(t *testing.T) {
	paths := []string{
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(strings.ToLower(string(data)), "mcp") {
			t.Errorf("%s retains MCP configuration", path)
		}
	}

	if _, err := os.Stat(".mcp.json"); !os.IsNotExist(err) {
		t.Fatal("removed MCP configuration exists")
	}
}

func TestCodexStarterPromptsUseNaturalLanguage(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"$callee:run-role Review the current changes.",
		"$callee:run-role Review the changes and fix verified findings.",
		"$callee:create-role Create a Go code-review role for Codex.",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("Codex plugin is missing starter prompt %q", want)
		}
	}

	if strings.Contains(string(data), "role:") {
		t.Error("Codex plugin retains role-specific starter syntax")
	}

	var manifest struct {
		Interface struct {
			DefaultPrompt []string `json:"defaultPrompt"`
		} `json:"interface"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}

	if len(manifest.Interface.DefaultPrompt) != 3 {
		t.Errorf("Codex plugin has %d starter prompts, want 3", len(manifest.Interface.DefaultPrompt))
	}
}

func TestDistributionMetadataMatchesRelease(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join("skills", "run-role", "SKILL.md"),
		filepath.Join("skills", "create-role", "SKILL.md"),
		filepath.Join("prefixed-skills", "callee-run-role", "SKILL.md"),
		filepath.Join("prefixed-skills", "callee-create-role", "SKILL.md"),
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(data), "0.4.1") || !strings.Contains(string(data), releaseVersion) {
			t.Errorf("%s does not match release version %s", path, releaseVersion)
		}
	}
}

func TestMarketplaceCatalogsReferenceCalleePlugin(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", ".agents", "plugins", "marketplace.json"),
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		var catalog any
		if err := json.Unmarshal(data, &catalog); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}

		if !strings.Contains(string(data), "\"callee\"") || !strings.Contains(string(data), "./plugins/callee") {
			t.Errorf("%s does not reference the Callee plugin", path)
		}

		if strings.Contains(strings.ToLower(string(data)), "mcp") {
			t.Errorf("%s retains MCP metadata", path)
		}
	}
}
