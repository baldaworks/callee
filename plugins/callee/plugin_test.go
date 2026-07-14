package callee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

	want := []string{"--yes", "@baldaworks/callee@0.4.1", "mcp-server"}
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
		"callee.role.list",
		"callee.subagent.prompt",
		"callee.subagent.reply",
		"@baldaworks/callee@0.4.1",
		"--new",
		"<role> <task> [--new]",
		"When the user supplies a role ID, dispatch it directly.",
		"only when the user asks what roles exist",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
	}

	if strings.Contains(text, "First discover roles") {
		t.Error("skill must not list roles before dispatching a known role")
	}

	if strings.Contains(text, "whenever the `callee.role.list` tool is available") {
		t.Error("skill must use the prompt tool to select MCP mode")
	}
}

func TestCodexStarterPromptUsesCalleeRoleSyntax(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(".codex-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(data), "$callee reviewer Review the current changes.") {
		t.Error("Codex plugin does not include the Callee reviewer starter prompt")
	}
}

func TestClaudeCommandsUseCalleeSkill(t *testing.T) {
	for _, name := range []string{"subagent.md", "setup.md"} {
		data, err := os.ReadFile(filepath.Join("commands", name))
		if err != nil {
			t.Fatal(err)
		}

		if !strings.Contains(string(data), "callee") {
			t.Errorf("%s does not reference the Callee skill", name)
		}
	}
}

func TestMarketplaceCatalogsReferenceCalleePlugin(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", ".agents", "plugins", "marketplace.json"),
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
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
