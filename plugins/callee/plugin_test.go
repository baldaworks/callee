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

	want := []string{"--yes", "@baldaworks/callee@0.3.0", "mcp-server"}
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
		"callee.role.list",
		"callee.subagent.prompt",
		"callee.subagent.reply",
		"@baldaworks/callee@0.3.0",
		"--new",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
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
