# Callee

## Versioned Markdown roles for AI coding agents

Callee lets developers and teams keep reusable specialist roles beside their
code and run them as subagents. Roles are project-local Markdown files: review
them in pull requests, override them per repository, and use them from the
coding agent already working on the task.

Built for Codex, Claude Code, Grok Build, Copilot CLI, and OpenCode.

```text
.callee/roles/reviewer.md  →  host plugin / MCP  →  reviewer subagent
```

Roles are not prompts you need to recreate in every conversation. They are
shared project tools: reviewable in pull requests, overridable per repository,
and available to the agent already working on the task.

## Get started

From the repository you want to work in:

1. Install the host plugin and create `.callee/roles/reviewer.md`.

   ```bash
   npx --yes @baldaworks/callee@0.5.0 setup codex
   # or: npx --yes @baldaworks/callee@0.5.0 setup claude
   # or: npx --yes @baldaworks/callee@0.5.0 setup grok
   # or: npx --yes @baldaworks/callee@0.5.0 setup copilot
   ```

2. Start a fresh host session.

3. Ask the reviewer to inspect the current change.

   Codex:

   ```text
   $callee role:reviewer Review the current changes
   ```

   Claude Code:

   ```text
   /callee:role reviewer Review the current changes
   ```

   Grok Build:

   ```text
   /callee role:reviewer Review the current changes
   ```

   Copilot CLI:

   ```text
   /callee role:reviewer Review the current changes
   ```

When MCP is available, later calls to the same role in the same host
conversation continue its active Callee thread. To make the next call start
fresh, reset the role first: `$callee reset:reviewer`, `/callee:reset reviewer`,
or `/callee reset:reviewer`.

In the CLI fallback, every role invocation is one-shot: it always starts fresh,
and `reset:<role>` has no persistent conversation to clear.

The reviewer returns findings with severity, evidence, impact, and a suggested
fix or test.

## Add specialists when you need them

Start with the [`reviewer`](examples/roles/reviewer.md). Add a specialist for a
specific kind of work:

- [`explorer`](examples/roles/explorer.md) maps relevant code paths without
  changing files.
- [`architect`](examples/roles/architect.md) prepares an implementation-ready
  design for a bounded change.
- [`implementer`](examples/roles/implementer.md) makes a focused change and
  runs relevant validation.
- [`tester`](examples/roles/tester.md) identifies missing coverage and concrete
  test cases.

## Reference

The sections below cover installation choices, the Markdown format, CLI and MCP
usage, discovery, and limitations.

### Installation and host plugins

The quickstart above is the fastest path: `callee setup <host>` installs the
matching Codex, Claude Code, Grok Build, or Copilot CLI plugin and creates a
reviewer role. Existing reviewer roles are left unchanged; pass `--force` to
replace one deliberately.

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

Or run the published CLI without installing Go:

```bash
npx --yes @baldaworks/callee@0.5.0 --version
```

Callee is available as a Codex, Claude Code, Grok Build, and Copilot CLI
plugin. Each bundles a skill that uses Callee MCP when it is available and
falls back to the npx CLI runner when it is not.

#### Manual plugin installation

Use these commands when you want to install the plugin or configure MCP without
creating the sample role.

##### Codex

```bash
codex plugin marketplace add baldaworks/callee --sparse .agents/plugins
codex plugin add callee@callee
```

Start a new Codex session, then invoke `$callee role:<role> <task>`, for
example:

```text
$callee role:reviewer Review the current changes
```

For a manual MCP setup, run:

```bash
codex mcp add callee -- npx --yes @baldaworks/callee@0.5.0 mcp-server
```

##### Claude Code

```text
/plugin marketplace add baldaworks/callee
/plugin install callee@callee
/reload-plugins
```

Run a role with `/callee:role <role> <task>`. Repeated requests for the same
role in one parent conversation reuse its MCP thread. Use `/callee:reset <role>`
when the next request should start fresh, or `/callee:setup` for MCP
configuration of the current host.

##### Grok Build

```bash
grok plugin marketplace add baldaworks/callee
grok plugin install callee@callee --trust
```

Start a new Grok Build session, then invoke `/callee role:<role> <task>`, for
example:

```text
/callee role:reviewer Review the current changes
```

Use `/callee reset:<role>` when the next request should start fresh.

The plugin bundles the MCP server. For a project-local manual configuration:

```bash
grok mcp add --scope project callee -- npx --yes @baldaworks/callee@0.5.0 mcp-server
```

The runtime itself needs a local `grok login` session or `XAI_API_KEY`.

##### Copilot CLI

```bash
copilot plugin marketplace add baldaworks/callee
copilot plugin install callee@callee
```

Start a new Copilot CLI session, then invoke `/callee role:<role> <task>`, for
example:

```text
/callee role:reviewer Review the current changes
```

Use `/callee reset:<role>` when the next request should start fresh.

`reset` only forgets the plugin's active thread for that role in the current
host conversation. It does not close the old ACP session, which remains
process-local until the MCP server exits.

#### Manual MCP timeouts

The bundled plugin configuration intentionally has no timeout: the supported
hosts do not share one portable setting. These limits apply only when you
configure Callee MCP manually; they do not change `callee doctor --timeout`.

For Codex, add the server to `~/.codex/config.toml` with a one-hour tool-call
timeout. The plugin-provided server does not expose this setting:

```toml
[mcp_servers.callee]
command = "npx"
args = ["--yes", "@baldaworks/callee@0.5.0", "mcp-server"]
startup_timeout_sec = 3600
tool_timeout_sec = 3600
```

For Claude Code, set the per-server timeout in a manual `.mcp.json` entry:

```json
{
  "mcpServers": {
    "callee": {
      "command": "npx",
      "args": ["--yes", "@baldaworks/callee@0.5.0", "mcp-server"],
      "timeout": 3600000
    }
  }
}
```

For Copilot CLI, use `--timeout` in milliseconds when adding a manual server:

```bash
copilot mcp add --timeout 3600000 callee -- npx --yes @baldaworks/callee@0.5.0 mcp-server
```

When the MCP server starts, Callee initializes one ACP runtime for every
unique configured provider (type, command, and extra arguments). If any
provider cannot start, the MCP server does not become available.

Grok Build has no verified timeout override for this configuration; use its host
defaults rather than adding an unsupported field to `.mcp.json`.

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

To adapt the reporting contract for your project, start from the
[`custom reviewer`](examples/roles/custom-reviewer.md) template.

### One-shot CLI

```bash
callee --role reviewer --prompt "Review the current changes"
```

Use `--roles-dir ./examples/roles` to load only a specific directory.

### Inspect roles

Use the role list when you want to browse configured role IDs and descriptions;
it is not required before starting a known role:

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
share one check; Callee still reports an outcome for every role. This controls
only ACP runtime initialization, not MCP tool calls. Use `--timeout` to override
it and `--roles-dir` to check only one role directory:

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

- `callee.role` starts a conversation.
- `callee.role.reply` continues a conversation.
- `callee.role.list` returns the available role IDs and descriptions.

The raw MCP tool names are `role`, `role.reply`, and `role.list`; the host
prefixes them with the configured server name. This avoids displaying a
duplicate `callee.callee` namespace in host tool calls.

Start a known role directly:

```json
{"role":"reviewer","prompt":"Review the current changes"}
```

Inspect roles only when you need to browse the library:

```json
{}
```

The response contains:

```json
{"roles":[{"id":"reviewer","description":"Reviews code changes for correctness and regressions."}]}
```

Follow-up with `callee.role.reply`:

```json
{"threadId":"cal_01JXYZ123","prompt":"Recheck the first finding."}
```

Both responses contain `structuredContent: { "threadId", "content" }` and legacy text `content`.

Within one MCP server process, roles sharing the same `type`, resolved command,
and `extra_args` share one ACP provider process. Each
`callee.role` call creates an independent ACP session with that
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
