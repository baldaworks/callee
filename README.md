# Callee

## Versioned Markdown specialist roles for AI coding agents

Callee gives developers and teams using Claude Code, Codex, Grok Build,
OpenCode, or Copilot a project-local library of specialist roles they can call
as subagents. Define a reviewer, explorer, architect, implementer, or tester
once in Markdown; keep it with the repository; and reuse it whenever the work
needs that perspective.

```text
.callee/roles/reviewer.md  →  host plugin / MCP  →  reviewer subagent
```

## The wedge: roles belong in the repository

A pasted prompt is a one-off conversation detail. A Callee role is a reusable,
reviewable project artifact with an explicit runtime. The same flat Markdown
format works across specialist workflows; the host discovers and dispatches
roles through its plugin and MCP surface.

- Version control roles alongside code, and override shared roles per project.
- Use one role library for review, exploration, design, implementation, and
  testing instead of rebuilding prompts in every chat.
- Keep follow-ups scoped honestly: a `threadId` continues only within the same
  active Callee MCP server process, never across restarts or sessions.

## Quickstart: ask a reviewer in 3 steps

From the repository you want to work in:

1. Install the host plugin and create `.callee/roles/reviewer.md`.

   ```bash
   npx --yes @baldaworks/callee@0.4.1 setup codex
   # or: npx --yes @baldaworks/callee@0.4.1 setup claude
   # or: npx --yes @baldaworks/callee@0.4.1 setup grok
   ```

2. Start a fresh host session so it loads the plugin and its MCP configuration.

3. Ask the host to review the current change.

   Claude Code:

   ```text
   /callee:subagent reviewer Review the current changes
   ```

   Codex:

   ```text
   $callee reviewer Review the current changes
   ```

   Grok Build:

   ```text
   /callee reviewer Review the current changes
   ```

The reviewer returns findings in a useful handoff shape:

```text
<severity> — <finding summary>
evidence: <file:line and observed behavior>
impact: <why the behavior matters>
suggested fix: <concrete code or test change>
```

## Start with a reviewer; grow into a team

The [`reviewer`](examples/roles/reviewer.md) is the fastest first role. Add a
specialist when the workflow calls for it:

- [`explorer`](examples/roles/explorer.md) maps relevant code paths without
  changing files.
- [`architect`](examples/roles/architect.md) prepares an implementation-ready
  design for a bounded change.
- [`implementer`](examples/roles/implementer.md) makes a focused change and
  runs relevant validation.
- [`tester`](examples/roles/tester.md) identifies missing coverage and concrete
  test cases.
- [`custom reviewer`](examples/roles/custom-reviewer.md) is a starting point
  for a project-specific review contract.

See the [`Grok reviewer`](examples/roles/grok-reviewer.md) template when your
project uses the Grok Build runtime.

## Reference

The sections below cover installation choices, the Markdown format, CLI and MCP
usage, discovery, and limitations.

### Installation and host plugins

The quickstart above is the fastest path: `callee setup <host>` installs the
matching host plugin and creates a reviewer role. Existing reviewer roles are
left unchanged; pass `--force` to replace one deliberately.

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

Or run the published CLI without installing Go:

```bash
npx --yes @baldaworks/callee@0.4.1 --version
```

Callee is available as a Claude Code, Codex, and Grok Build plugin. Each bundles a skill that
uses Callee MCP when it is available and falls back to the npx CLI runner when
it is not.

#### Manual plugin installation

Use these commands when you want to install the plugin or configure MCP without
creating the sample role.

##### Claude Code

```text
/plugin marketplace add baldaworks/callee
/plugin install callee@callee
/reload-plugins
```

Run a role with `/callee:subagent <role> <task>`. Repeated requests for the
same role in one parent conversation reuse its MCP thread; add `--new` to start
another role conversation. Use `/callee:setup` for the MCP configuration of
the current host.

##### Codex

```bash
codex plugin marketplace add baldaworks/callee --sparse .agents/plugins
codex plugin add callee@callee
```

Start a new Codex session, then invoke `$callee <role> <task>`. The first
argument after `$callee` is the project role ID, for example:

```text
$callee reviewer Review the current changes
```

For a manual MCP setup, run:

```bash
codex mcp add callee -- npx --yes @baldaworks/callee@0.4.1 mcp-server
```

##### Grok Build

```bash
grok plugin marketplace add baldaworks/callee
grok plugin install callee@callee --trust
```

Start a new Grok Build session, then invoke `/callee <role> <task>`, for
example:

```text
/callee reviewer Review the current changes
```

The plugin bundles the MCP server. For a project-local manual configuration:

```bash
grok mcp add --scope project callee -- npx --yes @baldaworks/callee@0.4.1 mcp-server
```

The runtime itself needs a local `grok login` session or `XAI_API_KEY`.

### Role format

`~/.config/callee/roles/reviewer.md`:

```md
---
description: Reviews code changes for correctness and regressions.
type: codex
model: gpt-5-codex
reasoning: high
mode: review
---

You are an independent code reviewer.

## Task

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
```

### One-shot CLI

```bash
callee --role reviewer --prompt "Review the current changes"
```

Use `--roles-dir ./examples/roles` to load only a specific directory.

### Role list

List the configured role IDs and descriptions without starting an ACP runtime:

```bash
callee role list
```

Use `--json` for machine-readable output compatible with the MCP role-list
response:

```bash
callee role list --json
```

As with other commands, `--roles-dir` loads roles only from the specified
directory:

```bash
callee role list --roles-dir ./examples/roles
```

### Doctor

Check that every loaded role can start and initialize its ACP runtime without
sending a model prompt:

```bash
callee doctor
```

Provider processes are checked sequentially with a 60 second timeout per
provider. Roles with the same `type`, resolved command, and extra arguments
share one check; Callee still reports an outcome for every role. Use
`--timeout` to override it and `--roles-dir` to check only one role directory:

```bash
callee doctor --roles-dir ./examples/roles --timeout 90s
```

`doctor` reports every failed role and exits non-zero if any runtime cannot be
initialized. It closes each successfully initialized runtime before continuing.

### MCP server

```json
{
  "mcpServers": {
    "callee": {
      "command": "callee",
      "args": ["mcp-server"]
    }
  }
}
```

With the `callee` server name in the configuration above, hosts expose three
tools:

- `callee.role.list` returns the available role IDs and descriptions.
- `callee.subagent.prompt` starts a conversation.
- `callee.subagent.reply` continues a conversation.

The raw MCP tool names are `role.list`, `subagent.prompt`, and
`subagent.reply`; the host prefixes them with the configured server name. This
avoids displaying a duplicate `callee.callee` namespace in host tool calls.

List roles before selecting one:

```json
{}
```

The response contains:

```json
{"roles":[{"id":"reviewer","description":"Reviews code changes for correctness and regressions."}]}
```

First call:

```json
{"role":"reviewer","prompt":"Review the current changes"}
```

Follow-up with `callee.subagent.reply`:

```json
{"threadId":"cal_01JXYZ123","prompt":"Recheck the first finding."}
```

Both responses contain `structuredContent: { "threadId", "content" }` and legacy text `content`.

Within one MCP server process, roles sharing the same `type`, resolved command,
and `extra_args` share one ACP provider process. Each
`callee.subagent.prompt` call creates an independent ACP session with that
role's model, mode, reasoning, and prompt.

### Frontmatter reference

| Field | Required | Meaning |
|---|---:|---|
| `description` | yes | Role description shown in MCP |
| `type` | yes | Built-in Callee runtime type |
| `cmd` | no | Executable override |
| `model` | no | Model identifier |
| `reasoning` | no | Norma Runtime `reasoning_effort` |
| `mode` | no | ACP session mode |
| `extra_args` | no | Arguments appended by Norma Runtime |

The Markdown body must contain exactly one `{{ prompt }}`. No other template
expressions are supported.

Supported types: `codex`, `claude`, `opencode`, `copilot`, `grok`, `generic_acp`.
`generic_acp` requires `cmd`.

`grok` starts `grok agent stdio`. Its `model`, `mode`, and `reasoning` fields
are forwarded as ACP configuration values. When the installed Grok ACP server
does not support one, Callee continues and the ACP adapter records a warning on
stderr.

```md
---
description: Runs a custom ACP-compatible reviewer.
type: generic_acp
cmd: /usr/local/bin/company-review-agent
model: reviewer-v2
reasoning: high
mode: review
extra_args:
  - --stdio
---

Review the following task:

{{ prompt }}
```

### Role discovery

Callee loads user roles first from `$XDG_CONFIG_HOME/callee/roles` (or
`~/.config/callee/roles`), then project roles from `.callee/roles`. Project
roles override user roles with the same path-relative ID. Nested files use
slash-separated IDs, such as `code/explorer`.

### Current limitations

Callee serves MCP over stdio. Its role registry and conversation threads are
process-local: restarting the server loses active threads, and changed role
files take effect when a new server process starts.
