# Coding-host integrations

Callee's host integrations teach a coding assistant how to inspect, create, validate, and run project-defined resources. They are convenience and safety instructions around the same CLI; they do not change workflow semantics or satisfy ACP provider prerequisites.

## Installed capabilities

Each integration exposes two complementary skills:

| Skill | Responsibility |
| --- | --- |
| Run Agent | Inspect the catalog and selected tree, collect required parameters, run the chosen agent through a controlling terminal, and return its final artifact with a concise capability trace. |
| Create Agent | Author a `Role`, `Sequential`, or `Loop` in Markdown or YAML, optionally assemble a Role from PromptKit, validate the physical file, and resolve the complete tree. |

Setup also writes six editable starter resources below `.callee/`: four Roles (`architect`, `explorer`, `implementer`, and `reviewer`) plus the `investigate` and `goalkeeper` workflows. The starter Roles set `spec.provider.type` for the selected setup target and otherwise defer model, mode, and reasoning to that backend.

## One-command setup

Run setup from the repository root:

```bash
npx --yes @baldaworks/callee@latest setup <target>
```

Valid targets and installed surfaces are:

| Target | Host invocation | Integration method |
| --- | --- | --- |
| `codex` | `$callee`, `$callee:run-agent`, `$callee:create-agent` | Registers the repository marketplace and installs `callee@callee`. |
| `claude` | `/callee:run-agent`, `/callee:create-agent` | Registers the marketplace and installs the project-scoped plugin. |
| `grok` | `/callee-run-agent`, `/callee-create-agent` | Registers the marketplace and installs the trusted plugin. |
| `copilot` | `/callee-run-agent`, `/callee-create-agent` | Registers the marketplace and installs the plugin. |
| `opencode` | `callee-run-agent` skill or `/callee`; `callee-create-agent` skill or `/callee-create-agent` | Writes skills and command wrappers below `.opencode/`. |
| `cursor` | `callee-run-agent` and `callee-create-agent` skills | Writes skills below `.cursor/skills/`. |

Existing setup-managed files are left unchanged. `--force` replaces them, including existing starter files at the managed paths. Review customized local files before forcing setup.

Codex setup first removes an existing marketplace registration named `callee`, ignoring only the specific case where it is absent, then adds the repository marketplace and plugin. The other marketplace targets run their host commands directly.

## Manual marketplace setup

Use manual setup only when you need to control host installation separately from starter-resource creation.

Codex:

```bash
codex plugin marketplace add baldaworks/callee
codex plugin add callee@callee
```

Claude Code:

```bash
claude plugin marketplace add baldaworks/callee
claude plugin install callee@callee --scope project
```

Grok Build:

```bash
grok plugin marketplace add baldaworks/callee
grok plugin install callee@callee --trust
```

Copilot CLI:

```bash
copilot plugin marketplace add baldaworks/callee
copilot plugin install callee@callee
```

These commands install the skills from [`plugins/callee`](../../plugins/callee) but do not create starter resources.

## Manual file-based setup

Copy only the Callee-managed assets into the corresponding project paths and preserve unrelated files:

| Host asset | Repository source | Project destination |
| --- | --- | --- |
| OpenCode skills | [`internal/cli/assets/opencode/skills`](../../internal/cli/assets/opencode/skills) | `.opencode/skills/` |
| OpenCode commands | [`internal/cli/assets/opencode/commands`](../../internal/cli/assets/opencode/commands) | `.opencode/commands/` |
| Cursor skills | [`internal/cli/assets/cursor/skills`](../../internal/cli/assets/cursor/skills) | `.cursor/skills/` |
| Starter resources | [`internal/cli/assets/starter`](../../internal/cli/assets/starter) | `.callee/` |

OpenCode's `/callee` and `/callee-create-agent` command files load the matching run/create skill. Cursor distributes the same prefixed skills through both CLI setup assets and the repository's Cursor marketplace.

## Execution boundary

The host skill invokes `callee agent list --json` and `callee agent view <id> --json` to understand the current project before running or authoring. Execution must still occur through `callee agent run` attached to a real controlling terminal. The host should keep terminal interaction separate from stdout and stderr and treat the exit status as authoritative.

Host setup does not:

- install or authenticate Codex, Claude Code ACP, OpenCode, Copilot, Grok, Cursor, or a generic ACP executable;
- choose a model, mode, or reasoning value beyond the starter Role's provider type;
- create a server, durable thread, or persisted workflow state;
- add provider types or workflow kinds.

Configure runtime behavior in each Role's `spec.provider`; see [ACP provider configuration](acp-providers.md).

## Validate an integration

After setup:

```bash
callee agent list
callee agent view workflows/investigate
callee agent validate .callee/roles/reviewer.md
```

Run `callee doctor` only after the selected provider CLI and credentials are available. To validate checked-in plugin assets during development, use the project checks in [Development and validation](../contributing/development.md).
