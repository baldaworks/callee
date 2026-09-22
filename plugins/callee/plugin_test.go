package callee

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
)

const releaseVersion = "0.22.0"

func TestSkillUsesOnlyTheCLI(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "run-agent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: run-agent",
		"npx --yes @baldaworks/callee@" + releaseVersion,
		"callee agent list --json",
		"callee agent view \"<agent-id>\" --json",
		"callee --permissions=\"<ask|allow|deny>\" agent view \"<agent-id>\" --json",
		"`specDrivenInteractive` as the authored baseline",
		"`authoredInteractive`",
		"Apply an explicit",
		"One-shot Roles with effective `ask`, no Human",
		"One-shot Roles with effective `allow` or `deny`, no Human",
		"Explicit `--interactive=true` with any permission mode",
		"Explicit `--interactive=false` with effective `allow` or `deny`",
		"including Human nodes beneath an",
		"callee agent run \"<agent-id>\"",
		"--permissions=ask|allow|deny",
		"Reject explicit `--interactive=false` with effective `ask`",
		"invoke Callee directly without allocating a PTY",
		"--param \"<effective-node-id>.<name>=<value>\"",
		"real controlling PTY",
		"Keep terminal interaction separate from stdout and stderr.",
		"mktemp -d",
		"callee_capture_dir",
		"`Role`, `DynamicRole`, `Script`, `Human`, `TypeSafeJev`, `OpenRouterDecision`, `Sequential`, `Parallel`, `Loop`, or `Router`",
		"Read Human prompts, Human responses",
		"Do not send `quit`, `exit`, `/done`",
		"artifact is written to stdout only after workflow execution and cleanup succeed",
		"For a Parallel",
		"`parallel_joined`",
		"setup <codex|claude|grok|copilot|opencode|cursor>",
		"Do not add Gemini",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{
		"callee exec",
		"callee role",
		"callee --version",
		"/tmp/callee-artifact.txt",
		"/tmp/callee-diagnostics.txt",
		"--thread-id",
		"--role",
		"type: gemini",
	} {
		if strings.Contains(strings.ToLower(text), forbidden) {
			t.Fatalf("skill retains removed or user-visible syntax %q", forbidden)
		}
	}
}

func TestCreateAgentSkillAuthorsEverySupportedKind(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-agent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	for _, want := range []string{
		"name: create-agent",
		"npx --yes @baldaworks/callee@" + releaseVersion,
		"callee agent list --json",
		"callee promptkit search",
		"callee promptkit show \"<template>\" --json",
		"callee promptkit role create",
		"Treat every declared PromptKit parameter as required",
		"Reuse values already present in the user request or available task context.",
		"Ask only for missing values.",
		"Choose exactly one declared non-`persona` parameter as `--prompt-param`.",
		"Do not use `persona` as `--prompt-param`",
		"configurable persona must use `--persona`",
		"If two candidates are materially plausible",
		"Bind only author-time-stable values",
		"protocol, taxonomy, and format composition",
		"parameter resolution and composition selection, not manual prompt reconstruction",
		"metadata.mode",
		"spec.interactive: true",
		"questions and confirmation gates",
		"callee agent run",
		"apiVersion: callee.metalagman.dev/v1alpha1",
		"kind: Role",
		"kind: Script",
		"kind: Human",
		"kind: OpenRouterDecision",
		"responseKey: approval",
		"A Human has no provider, permissions, parameters, or REPL setting.",
		"## Choose the authored interaction profile",
		"Supervised one-shot (default)",
		"Conversational",
		"Unattended restricted",
		"Unattended approved",
		"Do not turn `ask` into `allow` or `deny`",
		"including beneath",
		"an unselected Router branch",
		"spec.provider",
		"{{ .Input }}",
		"{{ .Params.focus }}",
		"exactly one unconditional bare",
		"For every `Sequential`, `Parallel`, `Loop`, `Router`, or nested-composite request",
		"[references/workflows.md](references/workflows.md)",
		"callee agent validate \"<written-agent-path>\"",
		"actual generated `.md`, `.yaml`, or `.yml` path",
		"callee agent view \"<agent-id>\" --json",
		"top-level `specDrivenInteractive` and effective `interactive`",
		"authored baseline and the latter as the agent's default execution path",
		"`authoredInteractive`, effective `interactive`",
		"`authoredPermissions`, and effective `permissions`",
		"unattended profiles are effectively non-interactive",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("create-agent skill is missing %q", want)
		}
	}

	for _, forbidden := range []string{"type: gemini", "api: callee", "kind: role", "{{ prompt }}"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("create-agent skill contains forbidden syntax %q", forbidden)
		}
	}
}

func TestCreateAgentHumanExampleValidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-agent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	encoded := readmeFence(t, string(data), "## Author a Human", "## Author a typed evaluation", "markdown")

	resource, err := agent.DecodeMarkdown("humans/approver", "humans/approver.md", []byte(encoded))
	if err != nil {
		t.Fatalf("decode Create Agent Human example: %v", err)
	}

	if resource.Kind != agent.HumanKind {
		t.Errorf("Create Agent Human example kind = %s, want %s", resource.Kind, agent.HumanKind)
	}
}

func TestCreateAgentDynamicRoleExampleValidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-agent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	encoded := readmeFence(t, string(data), "## Author a DynamicRole", "## Author a Script", "markdown")

	resource, err := agent.DecodeMarkdown("roles/dynamic", "roles/dynamic.md", []byte(encoded))
	if err != nil {
		t.Fatalf("decode Create Agent DynamicRole example: %v", err)
	}

	if resource.Kind != agent.DynamicRoleKind {
		t.Errorf("Create Agent DynamicRole example kind = %s, want %s", resource.Kind, agent.DynamicRoleKind)
	}
}

func TestCreateAgentEvaluationExampleValidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-agent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	encoded := readmeFence(t, string(data), "## Author a typed evaluation", "## Author a workflow", "markdown")

	resource, err := agent.DecodeMarkdown("judges/urgent", "judges/urgent.md", []byte(encoded))
	if err != nil {
		t.Fatalf("decode Create Agent evaluation example: %v", err)
	}

	if resource.Kind != agent.OpenRouterDecisionKind {
		t.Errorf("Create Agent evaluation example kind = %s, want %s", resource.Kind, agent.OpenRouterDecisionKind)
	}
}

func TestCreateAgentWorkflowReferenceCoversSupportedSemantics(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-agent", "references", "workflows.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := strings.Join(strings.Fields(string(data)), " ")
	for _, want := range []string{
		"## Contents",
		"[Place and represent files](#place-and-represent-files)",
		"[Compose the resolved tree](#compose-the-resolved-tree)",
		"[Author a Sequential workflow](#author-a-sequential-workflow)",
		"[Author a Parallel workflow](#author-a-parallel-workflow)",
		"[Author a Router workflow](#author-a-router-workflow)",
		"[Author a Loop workflow](#author-a-loop-workflow)",
		"[Finish the workflow](#finish-the-workflow)",
		"below `.callee/`",
		"`.md`, `.yaml`, or `.yml`",
		"do not also write `spec.body`",
		"kind: Sequential",
		"kind: Parallel",
		"kind: Router",
		"kind: Loop",
		"`Role`, `DynamicRole`, `Script`, `Human`, `TypeSafeJev`, `OpenRouterDecision`, `Sequential`, `Parallel`, `Loop`, and `Router`",
		"workflow child may reference any supported kind",
		"unique across the entire resolved tree",
		"`params` only when that child resolves directly to a `Role` or `DynamicRole`",
		"Never author the reserved `outputs`, `scripts`, or `evaluations` keys",
		"selected by `spec.responseKey`",
		"{{ index .State.outputs \"validator\" }}",
		"{{ with index .State.outputs \"validator\" }}",
		"maxIterations: 5",
		"onExhausted: fail",
		"set it to `complete` only when",
		"escalate to finish the loop",
		"Reserve `fail` for an unrecoverable condition",
		"direct Loop child completes that Loop immediately",
		"nested `Loop` is an ordinary child",
		"A route-template error never selects default",
		"does not retry or fail over to default",
		"Do not use a provider to choose an undeclared child",
		"callee agent validate",
		"callee agent view",
		"`specDrivenInteractive` and effective `interactive`",
		"authored/effective interactive and permission fields",
		"only effective `allow`/`deny` policies",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow reference is missing %q", want)
		}
	}
}

func TestCreateAgentWorkflowReferenceVariantsMatch(t *testing.T) {
	paths := []string{
		filepath.Join("skills", "create-agent", "references", "workflows.md"),
		filepath.Join("prefixed-skills", "callee-create-agent", "references", "workflows.md"),
		filepath.Join("..", "..", "internal", "cli", "assets", "opencode", "skills", "callee-create-agent", "references", "workflows.md"),
		filepath.Join("..", "..", "internal", "cli", "assets", "cursor", "skills", "callee-create-agent", "references", "workflows.md"),
	}

	want, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range paths[1:] {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s differs from %s", path, paths[0])
		}
	}
}

func TestCreateAgentWorkflowReferenceExamplesValidate(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "create-agent", "references", "workflows.md"))
	if err != nil {
		t.Fatal(err)
	}

	sections := strings.Split(string(data), "```markdown\n")
	if len(sections) != 5 {
		t.Fatalf("workflow reference contains %d Markdown examples, want 4", len(sections)-1)
	}

	wantKinds := []agent.Kind{agent.SequentialKind, agent.ParallelKind, agent.RouterKind, agent.LoopKind}

	for index, section := range sections[1:] {
		example, _, ok := strings.Cut(section, "\n```")
		if !ok {
			t.Fatalf("Markdown example %d has no closing fence", index)
		}

		resource, err := agent.Decode("reference/example", "example.md", []byte(example))
		if err != nil {
			t.Fatalf("decode Markdown example %d: %v", index, err)
		}

		if resource.Kind != wantKinds[index] {
			t.Errorf("Markdown example %d kind = %q, want %q", index, resource.Kind, wantKinds[index])
		}
	}
}

func TestPluginContainsOnlyTheNamedSkills(t *testing.T) {
	for directory, names := range map[string][]string{
		"skills":          {"create-agent", "run-agent"},
		"prefixed-skills": {"callee-create-agent", "callee-run-agent"},
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
		"create-agent": "callee-create-agent",
		"run-agent":    "callee-run-agent",
	} {
		paths := []string{
			filepath.Join("skills", shortName, "SKILL.md"),
			filepath.Join("prefixed-skills", prefixedName, "SKILL.md"),
			filepath.Join("..", "..", "internal", "cli", "assets", "opencode", "skills", prefixedName, "SKILL.md"),
			filepath.Join("..", "..", "internal", "cli", "assets", "cursor", "skills", prefixedName, "SKILL.md"),
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
		filepath.Join("skills", "run-agent", "agents", "openai.yaml"): {
			`display_name: "Callee Run Agent"`,
			`short_description: "Run and combine project-defined Callee agents"`,
			`default_prompt: "Use $callee:run-agent to review the current changes."`,
		},
		filepath.Join("skills", "create-agent", "agents", "openai.yaml"): {
			`display_name: "Callee Create Agent"`,
			`short_description: "Create Callee agents and deterministic workflows"`,
			`default_prompt: "Use $callee:create-agent to create a Go code-review agent for Codex."`,
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

func TestPublicMetadataUsesAgentPositioning(t *testing.T) {
	for path, want := range map[string]string{
		filepath.Join("..", "..", "README.md"):                             "## Put repeatable agent work in the repository",
		filepath.Join(".claude-plugin", "plugin.json"):                     "Run Markdown-defined agents and deterministic workflows.",
		filepath.Join(".codex-plugin", "plugin.json"):                      "Markdown agents and deterministic workflows.",
		filepath.Join(".cursor-plugin", "plugin.json"):                     "Run Markdown-defined agents and deterministic workflows.",
		filepath.Join(".grok-plugin", "plugin.json"):                       "Run Markdown-defined agents and deterministic workflows.",
		filepath.Join(".plugin", "plugin.json"):                            "Run Markdown-defined agents and deterministic workflows.",
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"):    "Markdown-defined agents and deterministic workflows for Claude Code.",
		filepath.Join("..", "..", ".cursor-plugin", "marketplace.json"):    "Markdown-defined agents and deterministic workflows for Cursor.",
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"):      "Marketplace for Markdown-defined agents and deterministic workflows in Grok Build.",
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"): "Markdown-defined agents and deterministic workflows for GitHub Copilot CLI.",
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
		filepath.Join(".cursor-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".cursor-plugin", "marketplace.json"),
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

func TestCursorManifestsUseSupportedFields(t *testing.T) {
	pluginPath := filepath.Join(".cursor-plugin", "plugin.json")
	plugin := readJSONObject(t, pluginPath)
	assertJSONFields(t, pluginPath, plugin, []string{
		"name",
		"displayName",
		"version",
		"description",
		"author",
		"publisher",
		"homepage",
		"repository",
		"license",
		"keywords",
		"category",
		"tags",
		"skills",
	})
	assertJSONString(t, pluginPath, plugin, "name", "callee")
	assertJSONString(t, pluginPath, plugin, "version", releaseVersion)
	assertJSONString(t, pluginPath, plugin, "skills", "./prefixed-skills/")

	marketplacePath := filepath.Join("..", "..", ".cursor-plugin", "marketplace.json")
	marketplace := readJSONObject(t, marketplacePath)
	assertJSONFields(t, marketplacePath, marketplace, []string{"name", "owner", "metadata", "plugins"})
	assertJSONString(t, marketplacePath, marketplace, "name", "callee")

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(marketplace["plugins"], &entries); err != nil {
		t.Fatalf("parse %s plugins: %v", marketplacePath, err)
	}

	if len(entries) != 1 {
		t.Fatalf("%s has %d plugins, want 1", marketplacePath, len(entries))
	}

	assertJSONFields(t, marketplacePath+" plugin", entries[0], []string{"name", "source", "description"})
	assertJSONString(t, marketplacePath, entries[0], "name", "callee")
	assertJSONString(t, marketplacePath, entries[0], "source", "./plugins/callee")
}

func readJSONObject(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	return object
}

func assertJSONFields(t *testing.T, path string, object map[string]json.RawMessage, fields []string) {
	t.Helper()

	allowed := make(map[string]bool, len(fields))
	for _, field := range fields {
		allowed[field] = true
	}

	for field := range object {
		if !allowed[field] {
			t.Errorf("%s contains unsupported field %q", path, field)
		}
	}
}

func assertJSONString(t *testing.T, path string, object map[string]json.RawMessage, field, want string) {
	t.Helper()

	var got string
	if err := json.Unmarshal(object[field], &got); err != nil {
		t.Fatalf("parse %s field %s: %v", path, field, err)
	}

	if got != want {
		t.Errorf("%s field %s = %q, want %q", path, field, got, want)
	}
}

func TestREADMEPresentsProductBeforeSetup(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	productIndex := strings.Index(text, "## Put repeatable agent work in the repository")

	setupIndex := strings.Index(text, "## Try it")
	if productIndex < 0 || setupIndex < 0 || productIndex >= setupIndex {
		t.Error("README must explain the product before setup")
	}

	for _, codingAgent := range []string{
		"Codex",
		"Claude Code",
		"Grok Build",
		"Copilot CLI",
		"OpenCode",
		"Cursor",
	} {
		if !strings.Contains(text, codingAgent) {
			t.Errorf("README is missing supported coding agent %q", codingAgent)
		}
	}

	for _, forbidden := range []string{
		"--sparse",
		"setup <host>",
		"coding host",
		"Coding-host",
		"@0.22.0 setup",
		"Flat frontmatter",
		"For Codex:",
		"callee exec --role",
		"{{ prompt }}",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("README contains stale or internal text %q", forbidden)
		}
	}

	for _, want := range []string{
		"$callee Run workflows/investigate",
		"docs/getting-started/quickstart.md",
		"docs/guides/running-agents.md",
		"docs/guides/coding-agent-integrations.md",
		"docs/reference/agent-resources.md",
		"docs/examples/index.md",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("README is missing audited text %q", want)
		}
	}

	for _, forbidden := range []string{
		"## Agent format",
		"## PromptKit",
		"## Doctor and graphs",
		"## OpenAI Build Week",
		"go test ./...",
		"TYPESAFE_API_KEY",
	} {
		if strings.Contains(text, forbidden) {
			t.Errorf("README contains long-form or contributor detail %q", forbidden)
		}
	}
}

func TestDocumentationInformationArchitecture(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	required := []string{
		"docs/getting-started/installation.md",
		"docs/getting-started/quickstart.md",
		"docs/guides/coding-agent-integrations.md",
		"docs/guides/coding-host-integrations.md",
		"docs/guides/running-agents.md",
		"docs/guides/importing-agents.md",
		"docs/guides/promptkit.md",
		"docs/guides/acp-providers.md",
		"docs/guides/acp-permissions.md",
		"docs/concepts/architecture.md",
		"docs/reference/cli.md",
		"docs/reference/agent-resources.md",
		"docs/reference/workflow-semantics.md",
		"docs/reference/execution-metrics.md",
		"docs/examples/index.md",
		"docs/contributing/development.md",
		"docs/contributing/release.md",
		"docs/project/build-week.md",
	}

	for _, relative := range required {
		if _, err := os.Stat(filepath.Join(repositoryRoot, relative)); err != nil {
			t.Errorf("required documentation path %s: %v", relative, err)
		}
	}

	pointer, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "guides", "cli.md"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(pointer), "../reference/cli.md") {
		t.Error("legacy CLI guide does not point to the canonical CLI reference")
	}

	integrationPointer, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "guides", "coding-host-integrations.md"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(integrationPointer), "coding-agent-integrations.md") {
		t.Error("legacy coding-host guide does not point to the coding-agent guide")
	}
}

func TestDocumentationLocalLinksResolve(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..")
	linkPattern := regexp.MustCompile(`\[[^]]*\]\(([^)[:space:]]+)`)
	paths := []string{filepath.Join(repositoryRoot, "README.md")}

	err := filepath.WalkDir(filepath.Join(repositoryRoot, "docs"), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !entry.IsDir() && filepath.Ext(path) == ".md" {
			paths = append(paths, path)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		for _, match := range linkPattern.FindAllStringSubmatch(string(data), -1) {
			target := strings.Trim(match[1], "<>")
			if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
				continue
			}

			if filepath.IsAbs(target) {
				t.Errorf("%s contains non-portable absolute link %q", path, target)

				continue
			}

			target, _, _ = strings.Cut(target, "#")
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), filepath.FromSlash(target))); err != nil {
				t.Errorf("%s contains unresolved local link %q: %v", path, match[1], err)
			}
		}
	}
}

func readmeFence(t *testing.T, text, startHeading, endHeading, language string) string {
	t.Helper()

	start := strings.Index(text, startHeading)
	if start < 0 {
		t.Fatalf("README is missing section %q", startHeading)
	}

	section := text[start:]

	end := strings.Index(section, "\n"+endHeading)
	if end < 0 {
		t.Fatalf("README section %q is missing end heading %q", startHeading, endHeading)
	}

	section = section[:end]
	opener := "```" + language + "\n"

	codeStart := strings.Index(section, opener)
	if codeStart < 0 {
		t.Fatalf("README section %q is missing a %s fence", startHeading, language)
	}

	code := section[codeStart+len(opener):]

	codeEnd := strings.Index(code, "\n```")
	if codeEnd < 0 {
		t.Fatalf("README section %q has an unterminated %s fence", startHeading, language)
	}

	return code[:codeEnd]
}

func TestPluginHasNoLegacyCommands(t *testing.T) {
	for _, name := range []string{"role.md", "reset.md", "setup.md", "subagent.md"} {
		if _, err := os.Stat(filepath.Join("commands", name)); !os.IsNotExist(err) {
			t.Errorf("removed command %s exists", name)
		}
	}
}

func TestPluginManifestsExposeCodingAgentAppropriateSkills(t *testing.T) {
	for path, want := range map[string]string{
		filepath.Join(".claude-plugin", "plugin.json"): `"skills": "./skills/"`,
		filepath.Join(".codex-plugin", "plugin.json"):  `"skills": "./skills/"`,
		filepath.Join(".cursor-plugin", "plugin.json"): `"skills": "./prefixed-skills/"`,
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
		filepath.Join(".cursor-plugin", "plugin.json"),
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
		"$callee:run-agent Review the current changes.",
		"$callee:run-agent Review the changes and fix verified findings.",
		"$callee:create-agent Create a Go code-review agent for Codex.",
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

func TestRunAgentSkillDocumentsControllingPTYFallback(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("skills", "run-agent", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}

	content := string(data)
	for _, required := range []string{"test -r /dev/tty", "script -qefc", "mktemp -d", "callee_capture_dir", "/artifact", "/diagnostics"} {
		if !strings.Contains(content, required) {
			t.Errorf("run-agent skill does not contain %q", required)
		}
	}

	for _, forbidden := range []string{"/tmp/callee-artifact.txt", "/tmp/callee-diagnostics.txt"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("run-agent skill retains fixed capture path %q", forbidden)
		}
	}
}

func TestRunAgentSkillReportsExecutionMetrics(t *testing.T) {
	paths := []string{
		filepath.Join("skills", "run-agent", "SKILL.md"),
		filepath.Join("prefixed-skills", "callee-run-agent", "SKILL.md"),
	}
	wants := []string{
		"agent run finished",
		"agent_duration",
		"agent_wait_duration",
		"agent_token_usage",
		"agent_*_tokens",
		"role_provider",
		"role_model",
		"role_reasoning",
		"role_duration",
		"role_wait_duration",
		"role_token_usage",
		"role_*_tokens",
	}

	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		content := string(data)
		for _, want := range wants {
			if !strings.Contains(content, want) {
				t.Errorf("%s does not contain %q", path, want)
			}
		}
	}
}

func TestDistributionMetadataMatchesRelease(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".grok-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".github", "plugin", "marketplace.json"),
		filepath.Join(".claude-plugin", "plugin.json"),
		filepath.Join(".codex-plugin", "plugin.json"),
		filepath.Join(".cursor-plugin", "plugin.json"),
		filepath.Join(".grok-plugin", "plugin.json"),
		filepath.Join(".plugin", "plugin.json"),
		filepath.Join("skills", "run-agent", "SKILL.md"),
		filepath.Join("skills", "create-agent", "SKILL.md"),
		filepath.Join("prefixed-skills", "callee-run-agent", "SKILL.md"),
		filepath.Join("prefixed-skills", "callee-create-agent", "SKILL.md"),
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

func TestThirdPartyLicensesAreIncludedInNPMArtifacts(t *testing.T) {
	root := filepath.Join("..", "..")
	for path, wants := range map[string][]string{
		filepath.Join(root, "THIRD_PARTY_NOTICES.md"): {
			"vecgo", "v0.0.15", "third_party/vecgo/LICENSE",
			"codex-acp-bridge", "v1.7.7", "third_party/codex-acp-bridge/LICENSE",
		},
		filepath.Join(root, "third_party", "vecgo", "LICENSE"): {
			"Apache License", "Version 2.0", "Copyright 2025 Frank Hübner",
		},
		filepath.Join(root, "third_party", "codex-acp-bridge", "LICENSE"): {
			"MIT License", "Copyright (c) 2026 Alexey Samoylov",
		},
		filepath.Join(root, ".github", "workflows", "omnidist-release.yml"): {
			"Include third-party licenses in npm artifacts",
			"Third-party component: vecgo v0.0.15",
			"cat third_party/vecgo/LICENSE",
			"Third-party component: codex-acp-bridge v1.7.7",
			"cat third_party/codex-acp-bridge/LICENSE",
		},
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s is missing %q", path, want)
			}
		}
	}
}

func TestMarketplaceCatalogsReferenceCalleePlugin(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", ".agents", "plugins", "marketplace.json"),
		filepath.Join("..", "..", ".claude-plugin", "marketplace.json"),
		filepath.Join("..", "..", ".cursor-plugin", "marketplace.json"),
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
