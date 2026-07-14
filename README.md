# Callee

Turn Markdown roles into callable ACP agents.

Callee combines Markdown instructions with flat runtime metadata. Roles use Codex, Claude Code, OpenCode, Copilot, or a generic ACP executable. Run a role once from the CLI, or expose the same registry through two persistent MCP tools. An opaque Callee `threadId` continues a conversation.

## Installation

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

## Role

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

## One-shot CLI

```bash
callee --role reviewer --prompt "Review the current changes"
```

Use `--roles-dir ./examples/roles` to load only a specific directory.

## Doctor

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

## MCP server

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

The server exposes two tools: `callee` starts a conversation and `callee-reply` continues one.

First call:

```json
{"role":"reviewer","prompt":"Review the current changes"}
```

Follow-up with `callee-reply`:

```json
{"threadId":"cal_01JXYZ123","prompt":"Recheck the first finding."}
```

Both responses contain `structuredContent: { "threadId", "content" }` and legacy text `content`.

Within one MCP server process, roles sharing the same `type`, resolved command,
and `extra_args` share one ACP provider process. Each `callee` call creates an
independent ACP session with that role's model, mode, reasoning, and prompt.

## Frontmatter reference

| Field | Required | Meaning |
|---|---:|---|
| `description` | yes | Role description shown in MCP |
| `type` | yes | Built-in Callee runtime type |
| `cmd` | no | Executable override |
| `model` | no | Model identifier |
| `reasoning` | no | Norma Runtime `reasoning_effort` |
| `mode` | no | ACP session mode |
| `extra_args` | no | Arguments appended by Norma Runtime |

The Markdown body must contain exactly one `{{ prompt }}`. No other template expressions are supported.

Supported types: `codex`, `claude`, `opencode`, `copilot`, `generic_acp`. `generic_acp` requires `path`.

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

## Role discovery

Callee loads user roles first from `$XDG_CONFIG_HOME/callee/roles` (or `~/.config/callee/roles`), then project roles from `.callee/roles`. Project roles override user roles with the same path-relative ID. Nested files use slash-separated IDs, such as `code/explorer`.

## Current limitations

Registry and threads are process-local. Callee has no role hot reload, provider configuration, profiles, background jobs, remote/HTTP MCP transport, MCP forwarding, thread listing or closing, or Gemini support.
