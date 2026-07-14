# Callee

## Versioned Markdown roles for AI coding agents

Callee lets teams using Claude Code, Codex, OpenCode, or Copilot keep
specialist roles alongside their code, then call those roles as subagents.
Define a reviewer, explorer, architect, implementer, or tester once in
Markdown; version it with the project; and dispatch it when the work calls for
that perspective.

Unlike a prompt pasted into a chat, a Callee role is a reusable, project-local
artifact with a clear runtime. One flat Markdown frontmatter format selects an
ACP runtime, and Callee exposes the role through the host agent's plugin/MCP
surface. Conversation continuation is deliberately limited to the same active
Callee MCP server process, so a `threadId` never implies durable or cross-session
state.

## Why Callee

- Keep roles in version control and override shared roles per project.
- Use one role library for distinct specialist workflows instead of maintaining
  ad hoc prompts in every agent conversation.
- Let the host agent discover and dispatch roles through Callee's native plugin
  and MCP surface.

## Try it in 3 steps

1. Create a project-local reviewer role from the
   [`reviewer` template](examples/roles/reviewer.md):

   ```bash
   mkdir -p .callee/roles
   cp examples/roles/reviewer.md .callee/roles/reviewer.md
   ```

2. Install the [Claude Code](#claude-code) or [Codex](#codex) plugin below.

3. Ask the host agent to review the current change:

   ```text
   /callee:subagent reviewer Review the current changes
   ```

   In Codex, invoke the `$callee:callee` skill with `reviewer Review the
   current changes`.

The reviewer returns findings ordered by severity, each with a location,
concrete evidence, expected impact, and a recommended fix or test.

## Start with one role; grow into a team

Begin with a reviewer, then add roles as your workflow needs them:

- [`reviewer`](examples/roles/reviewer.md) — independently checks changes for
  defects, regressions, security issues, and missing tests.
- [`explorer`](examples/roles/explorer.md) — maps relevant code paths without
  changing files.
- [`architect`](examples/roles/architect.md) — produces an implementation-ready
  design for a bounded change.
- [`implementer`](examples/roles/implementer.md) — makes a focused change and
  runs relevant validation.
- [`tester`](examples/roles/tester.md) — identifies missing coverage and
  concrete test cases.
- [`custom reviewer`](examples/roles/custom-reviewer.md) — starts from a
  review role with a more specific reporting contract.

## Reference

### Installation

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

Or run the published CLI without installing Go:

```bash
npx --yes @baldaworks/callee@0.4.0 --version
```

### Agent plugins

Callee is available as a Claude Code and Codex plugin. Both bundle a skill that
uses Callee MCP when it is available and falls back to the npx CLI runner when
it is not.

#### Claude Code

```text
/plugin marketplace add baldaworks/callee
/plugin install callee@callee
/reload-plugins
```

Run a role with `/callee:subagent <role> <task>`. Repeated requests for the
same role in one parent conversation reuse its MCP thread; add `--new` to start
another role conversation. Use `/callee:setup` for the MCP configuration of
the current host.

#### Codex

```bash
codex plugin marketplace add baldaworks/callee --sparse .agents/plugins
```

Install the plugin from `/plugins`, then invoke `$callee:callee`. For a manual
MCP setup, run:

```bash
codex mcp add callee -- npx --yes @baldaworks/callee@0.4.0 mcp-server
```

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

Supported types: `codex`, `claude`, `opencode`, `copilot`, `generic_acp`.
`generic_acp` requires `path`.

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

Registry and threads are process-local. Callee has no role hot reload, provider
configuration, profiles, background jobs, remote/HTTP MCP transport, MCP
forwarding, thread listing or closing, or Gemini support.
