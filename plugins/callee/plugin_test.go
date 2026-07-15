package callee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseVersion = "0.8.0"

func TestSkillUsesOnlyTheCLI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "callee-run-role", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: callee-run-role",
		"Use `$callee-run-role <task>`",
		"@baldaworks/callee@" + releaseVersion + " role list --json",
		"`params` object containing",
		"role view \"<selected-role-id>\" --json",
		"@baldaworks/callee@" + releaseVersion + " prompt --role \"<selected-role-id>\"",
		"--param \"<name>=<value>\" --json",
		"supply every parameter declared by the selected role",
		"Do not pass `--param` or `--param-file` when continuing a thread.",
		"setup <codex|claude|grok|copilot|opencode>",
		"--thread-id \"<opaque-thread-handle>\" --message \"<stage task>\" --json",
		"Run independent discovery or review stages in parallel.",
		"When the user naturally names a role, resolve that mention against the",
		"Run that role as the required first stage. Do not silently substitute a",
		"A naturally named role is a first-stage constraint, not an exclusive lock.",
		"If a named role has no unambiguous catalog match, ask a concise clarification",
		"For a dependent stage, include the original task and a concise handoff with",
		"actionable findings, evidence and provenance, relevant files, constraints, and",
		"Keep only this short-lived routing state in the current host conversation",
		"continued in a new conversation. Do not expose handles, role IDs, or raw",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{"role:<", "mcp", "reset:", "acp", "user-invocable:"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("skill retains removed or user-visible syntax %q", forbidden)
		}
	}

	if strings.Contains(text, "--thread-id <thread-id>") {
		t.Fatal("skill exposes an explicit user thread syntax")
	}

	if !strings.Contains(text, "--param \"<name>=<value>\" --json") {
		t.Fatal("every internal prompt must use JSON output")
	}
}

func TestPromptKitSkillAuthorsRolesThroughTheCLI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "callee-create-role", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: callee-create-role",
		"Use `$callee-create-role <role request>`",
		"`$callee-run-role <task>` to run existing roles",
		"@baldaworks/callee@" + releaseVersion + " promptkit search",
		"@baldaworks/callee@" + releaseVersion + " promptkit show",
		"@baldaworks/callee@" + releaseVersion + " promptkit role create",
		"--prompt-param",
		"--bind",
		"--bind-file",
		"top-level `params` map",
		"--persona",
		"--protocol",
		"--taxonomy",
		"--no-format",
		"Never default or infer a type.",
		"Keep Callee metadata flat.",
		"--dry-run",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("PromptKit skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{"mcp", "type: gemini", "role:<", "user-invocable:"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Errorf("PromptKit skill contains forbidden syntax %q", forbidden)
		}
	}
}

func TestPluginContainsOnlyTheNamedSkills(t *testing.T) {
	entries, err := os.ReadDir("skills")
	if err != nil {
		t.Fatal(err)
	}

	if len(entries) != 2 {
		t.Fatalf("skills directory contains %d entries, want 2", len(entries))
	}

	want := map[string]bool{
		"callee-create-role": false,
		"callee-run-role":    false,
	}
	for _, entry := range entries {
		if _, ok := want[entry.Name()]; !ok {
			t.Errorf("unexpected skill directory %q", entry.Name())

			continue
		}

		want[entry.Name()] = true
	}

	for name, found := range want {
		if !found {
			t.Errorf("skill directory %q is missing", name)
		}
	}
}

func TestCodexSkillMetadataUsesPublicNames(t *testing.T) {
	for path, fields := range map[string][]string{
		filepath.Join("skills", "callee-run-role", "agents", "openai.yaml"): {
			`display_name: "Callee Run Role"`,
			`short_description: "Run and combine project-defined Callee roles"`,
			`default_prompt: "Use $callee-run-role to review the current changes."`,
		},
		filepath.Join("skills", "callee-create-role", "agents", "openai.yaml"): {
			`display_name: "Callee Create Role"`,
			`short_description: "Create Callee roles from PromptKit templates"`,
			`default_prompt: "Use $callee-create-role to create a Go code-review role for Codex."`,
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
		filepath.Join("..", "..", "README.md"):                             "## Provider-aware subagent roles, described in Markdown",
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

func TestPluginHasNoLegacyCommands(t *testing.T) {
	for _, name := range []string{"role.md", "reset.md", "setup.md", "subagent.md"} {
		if _, err := os.Stat(filepath.Join("commands", name)); !os.IsNotExist(err) {
			t.Errorf("removed command %s exists", name)
		}
	}
}

func TestAllPluginManifestsExposeTheSharedSkills(t *testing.T) {
	for _, path := range []string{
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(data), `"skills": "./skills/"`) {
			t.Errorf("%s does not expose the shared skills directory", path)
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
		"$callee-run-role Review the current changes.",
		"$callee-run-role Review the changes and fix verified findings.",
		"$callee-create-role Create a Go code-review role for Codex.",
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
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join("skills", "callee-run-role", "SKILL.md"),
		filepath.Join("skills", "callee-create-role", "SKILL.md"),
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
