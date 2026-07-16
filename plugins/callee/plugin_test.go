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
		"Try `callee --version`.",
		"npx --yes @baldaworks/callee@" + releaseVersion,
		"callee role list --json",
		"`params` object containing",
		"role view \"<selected-role-id>\" --json",
		"top-level `repl` value as",
		"callee exec --role \"<selected-role-id>\"",
		"--param \"<name>=<value>\" --json",
		"callee agent --role \"<selected-role-id>\"",
		"Treat controlling-terminal support as a launch prerequisite, not a runtime",
		"Before invoking `callee agent`, select and configure a process runner",
		"Do not launch with a generic PTY merely to",
		"Start the role only after the runner guarantees this arrangement.",
		"terminal channel available for the lifetime of the process",
		"capture stdout and stderr separately from terminal interaction.",
		"Keep the same `callee agent` process alive",
		"final Markdown artifact to stdout",
		"supply every parameter declared by the selected role",
		"Do not pass `--param` or `--param-file` when continuing a thread.",
		"setup <codex|claude|grok|copilot|opencode>",
		"--thread-id \"<opaque-thread-handle>\" --message \"<stage task>\"",
		"Do not pass `--timeout` on the first attempt.",
		"Callee uses `provider.timeout`",
		"otherwise uses the CLI default of 15 minutes.",
		"first attempt ends specifically because its timeout expired, retry the same",
		"stage with an explicit, larger `--timeout`.",
		"Run independent discovery or review stages in parallel.",
		"When the user naturally names a role, resolve that mention against the",
		"Run that role as the required first stage. Do not silently substitute a",
		"A naturally named role is a first-stage constraint, not an exclusive lock.",
		"If a named role has no unambiguous catalog match, ask a concise clarification",
		"For a dependent stage, include the original task and a concise handoff with",
		"actionable findings, evidence and provenance, relevant files, constraints, and",
		"Keep only this short-lived routing state in the current host conversation",
		"continued in a new conversation. Do not expose handles, role IDs, or",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"role:<", "mcp", "reset:", "acp", "user-invocable:",
		"exec_command", "write_stdin", "session_id", "script -q", "/dev/tty",
		"setsid", "tiocsctty", "openpty",
	} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("skill retains removed or user-visible syntax %q", forbidden)
		}
	}

	if strings.Contains(text, "--thread-id <thread-id>") {
		t.Fatal("skill exposes an explicit user thread syntax")
	}

	if !strings.Contains(text, "--param \"<name>=<value>\" --json") {
		t.Fatal("one-shot dispatch must use JSON output")
	}

	if strings.Contains(text, "callee prompt --role") || strings.Contains(text, "provider.repl") {
		t.Fatal("skill retains the removed prompt command or nested REPL field")
	}

	if strings.Contains(text, "--timeout 15m") {
		t.Fatal("skill overrides the role and CLI timeout on its first attempt")
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
		"Try `callee --version`.",
		"npx --yes @baldaworks/callee@" + releaseVersion,
		"two to four short capability queries",
		"callee promptkit search \"<capability>\" --type template --json",
		"callee promptkit list --json",
		"callee promptkit show \"<template>\" --json",
		"Present at most three candidates.",
		"wait for the user to confirm",
		"author-requirements-doc",
		"author-design-doc",
		"author-architecture-spec",
		"callee promptkit role create",
		"--prompt-param",
		"--bind",
		"--bind-file",
		"top-level `params` map",
		"--persona",
		"--protocol",
		"--taxonomy",
		"--no-format",
		"default or infer a type.",
		"nested `provider` section",
		"## Decide the interaction mode",
		"expected to ask model-led follow-up questions",
		"Do not enable REPL merely because the role has unbound runtime parameters.",
		"Pass `--repl` only for a positive decision.",
		"PromptKit does not perform this semantic",
		"--repl",
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

func TestREADMEPresentsHostsEqually(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	quickStartStart := strings.Index(text, "## Quick start")
	wedgeStart := strings.Index(text, "## The wedge")
	integrationsStart := strings.Index(text, "## Host integrations")
	rolesStart := strings.Index(text, "## Roles")

	if quickStartStart < 0 || wedgeStart < 0 || integrationsStart < 0 || rolesStart < 0 {
		t.Fatal("README is missing a required landing-page section")
	}

	hero := text[:quickStartStart]
	quickStart := text[quickStartStart:wedgeStart]
	integrations := text[integrationsStart:rolesStart]
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

	for _, section := range []struct {
		name string
		text string
	}{
		{name: "hero", text: hero},
		{name: "host integrations", text: integrations},
	} {
		previous := -1

		for _, host := range hosts {
			row := "| " + host.name + " | `npx --yes @baldaworks/callee@latest setup " + host.target + "` |"
			index := strings.Index(section.text, row)

			if index < 0 {
				t.Errorf("README %s is missing setup row %q", section.name, row)

				continue
			}

			if index <= previous {
				t.Errorf("README %s places %s outside the canonical host order", section.name, host.name)
			}

			previous = index
		}
	}

	previous := -1

	for _, host := range hosts {
		heading := "#### " + host.name
		index := strings.Index(integrations, heading)

		if index < 0 {
			t.Errorf("README manual installation is missing heading %q", heading)

			continue
		}

		if index <= previous {
			t.Errorf("README manual installation places %s outside the canonical host order", host.name)
		}

		previous = index
	}

	if strings.Contains(strings.ToLower(quickStart), "codex") {
		t.Error("README quick start singles out Codex")
	}

	for _, forbidden := range []string{
		"--sparse",
		"setup <host>",
		"@0.10.0 setup",
		"Flat frontmatter",
		"--type codex",
		"For Codex:",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("README contains host-biased or stale text %q", forbidden)
		}
	}

	for _, want := range []string{
		"Nested provider frontmatter",
		"Supported types: `codex`, `claude`, `grok`, `copilot`, `opencode`, and",
		"--type generic_acp",
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
