package callee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseVersion = "0.5.0"

func TestSkillUsesOnlyTheCLI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "callee", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: callee",
		"user-invocable: true",
		"Use `$callee <task>`",
		"@baldaworks/callee@" + releaseVersion + " list --json",
		"@baldaworks/callee@" + releaseVersion + " prompt --role \"<selected-role-id>\"",
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

	for _, forbidden := range []string{"role:<", "mcp", "reset:", "acp"} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("skill retains removed or user-visible syntax %q", forbidden)
		}
	}

	if strings.Contains(text, "--thread-id <thread-id>") {
		t.Fatal("skill exposes an explicit user thread syntax")
	}

	if !strings.Contains(text, "--message \"<stage task>\" --json") {
		t.Fatal("every internal prompt must use JSON output")
	}
}

func TestPluginHasNoLegacyCommands(t *testing.T) {
	for _, name := range []string{"role.md", "reset.md", "setup.md", "subagent.md"} {
		if _, err := os.Stat(filepath.Join("commands", name)); !os.IsNotExist(err) {
			t.Errorf("removed command %s exists", name)
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
		"$callee Review the current changes.",
		"$callee Review the current changes and fix any verified findings.",
		"$callee With the reviewer role, review the current changes.",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("Codex plugin is missing starter prompt %q", want)
		}
	}

	if strings.Contains(string(data), "role:") {
		t.Error("Codex plugin retains role-specific starter syntax")
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
		filepath.Join("skills", "callee", "SKILL.md"),
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
