package callee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const releaseVersion = "0.5.0"

func TestMCPConfigUsesPublishedCalleeRunner(t *testing.T) {
	data, err := os.ReadFile(".mcp.json")
	if err != nil {
		t.Fatal(err)
	}

	var config struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}

	server, ok := config.MCPServers["callee"]
	if !ok {
		t.Fatal("Callee MCP server is missing")
	}

	if server.Command != "npx" {
		t.Fatalf("command = %q, want npx", server.Command)
	}

	want := []string{"--yes", "@baldaworks/callee@" + releaseVersion, "mcp-server"}
	if strings.Join(server.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("args = %#v, want %#v", server.Args, want)
	}
}

func TestSkillDescribesMCPAndCLIModes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "callee", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: callee",
		"user-invocable: true",
		"callee.role",
		"callee.role.reply",
		"callee.role.list",
		"@baldaworks/callee@" + releaseVersion,
		"role:<role-id> <task>",
		"reset:<role-id>",
		"For `role:<role-id>`, dispatch the supplied role directly.",
		"only when the user explicitly asks what roles exist",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
	}

	if strings.Contains(text, "First discover roles") {
		t.Error("skill must not list roles before dispatching a known role")
	}

	if strings.Contains(text, "whenever the `callee.role.list` tool is available") {
		t.Error("skill must use the role tool to select MCP mode")
	}

	if strings.Contains(text, "unavailable or fails") {
		t.Error("skill must not fall back after an MCP call failure")
	}

	for _, want := range []string{
		"unavailable before dispatch",
		"Do not retry through the CLI\nafter an MCP tool call starts",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing safe fallback policy %q", want)
		}
	}

	if strings.Contains(text, "--new") {
		t.Error("skill must use explicit role and reset actions instead of --new")
	}

	if strings.Contains(text, "reset <role-id>") {
		t.Error("skill must bind reset to the role with a colon")
	}

	if !strings.Contains(text, "Do not try to\n  close the old ACP session.") {
		t.Error("skill must document reset as a local thread-ledger operation")
	}
}

func TestCodexStarterPromptUsesCalleeRoleSyntax(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "$callee role:reviewer Review the current changes.") {
		t.Error("Codex plugin does not include the Callee reviewer starter prompt")
	}
}

func TestClaudeCommandsUseCalleeSkill(t *testing.T) {
	for _, name := range []string{"role.md", "reset.md", "setup.md"} {
		data, err := os.ReadFile(filepath.Join("commands", name))
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(data), "callee") {
			t.Errorf("%s does not reference the Callee skill", name)
		}
	}

	role, err := os.ReadFile(filepath.Join("commands", "role.md"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(role), "callee.role") {
		t.Error("role command does not select MCP mode through prompt")
	}

	if strings.Contains(string(role), "role-list tool is available") {
		t.Error("role command must not use role.list to select MCP mode")
	}

	if !strings.Contains(string(role), "`role:<role> <task>`") {
		t.Error("role command does not translate Claude arguments to role syntax")
	}

	reset, err := os.ReadFile(filepath.Join("commands", "reset.md"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(reset), "`reset:<role>`") || !strings.Contains(string(reset), "Do not call an MCP tool") {
		t.Error("reset command must not extend the MCP API")
	}

	if _, err := os.Stat(filepath.Join("commands", "subagent.md")); !os.IsNotExist(err) {
		t.Error("legacy subagent command must not exist")
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
		".mcp.json",
		filepath.Join("skills", "callee", "SKILL.md"),
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		text := string(data)
		if strings.Contains(text, "0.4.1") {
			t.Errorf("%s still refers to 0.4.1", path)
		}

		if !strings.Contains(text, releaseVersion) {
			t.Errorf("%s does not refer to %s", path, releaseVersion)
		}
	}

	for _, path := range []string{
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(data), `"license": "MIT"`) {
			t.Errorf("%s does not declare the MIT license", path)
		}
	}

	for _, path := range []string{
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if strings.Contains(string(data), "persistent MCP") {
			t.Errorf("%s overpromises persistence", path)
		}
	}
}

func TestREADMEDocumentsOneHourManualMCPTimeouts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"startup_timeout_sec = 3600",
		"tool_timeout_sec = 3600",
		`"timeout": 3600000`,
		"copilot mcp add --timeout 3600000 callee",
		"plugin-provided server does not expose this setting",
		"the MCP server does not become available",
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("README is missing %q", want)
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

		text := string(data)
		if !strings.Contains(text, "\"callee\"") || !strings.Contains(text, "./plugins/callee") {
			t.Errorf("%s does not reference the Callee plugin", path)
		}
	}
}

func TestCopilotPluginManifestUsesSharedSkillAndMCPConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}

	var manifest struct {
		Name       string `json:"name"`
		Skills     string `json:"skills"`
		MCPServers string `json:"mcpServers"`
		Commands   any    `json:"commands"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}

	if manifest.Name != "callee" || manifest.Skills != "./skills/" || manifest.MCPServers != "./.mcp.json" {
		t.Fatalf("Copilot manifest = %#v", manifest)
	}

	if manifest.Commands != nil {
		t.Error("Copilot plugin must not expose unnamespaced command files")
	}
}

func TestGrokPluginManifestUsesSharedSkillAndMCPConfig(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".grok-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}

	var manifest struct {
		Name       string `json:"name"`
		Skills     string `json:"skills"`
		MCPServers string `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}

	if manifest.Name != "callee" || manifest.Skills != "./skills/" || manifest.MCPServers != "./.mcp.json" {
		t.Fatalf("Grok manifest = %#v", manifest)
	}
}
