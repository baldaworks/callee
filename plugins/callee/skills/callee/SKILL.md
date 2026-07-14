---
name: callee
description: Run Markdown-defined Callee roles as subagents. Use when the user asks to run a Callee role, delegate work to a named Callee role, continue a role conversation, or configure Callee MCP.
---

# Callee

Run Callee roles through MCP whenever the `callee.role.list` tool is available.
Use the CLI fallback only when that MCP tool is unavailable or fails.

## Dispatch

Interpret the first token of a role request as the role ID and the remaining text as the task.
Treat `--new` as a request to start an independent conversation; remove it before sending the task.
Role IDs can include `/` but cannot include spaces.

First discover roles. In MCP mode, call `callee.role.list` with `{}`. In CLI
fallback mode, show the role list when the user needs it:

```bash
npx --yes @baldaworks/callee@0.3.0 role list
```

In CLI mode, let the role runner report an unknown role and its available IDs.

### MCP mode

Maintain a Callee thread ledger in the current parent conversation, keyed by role
ID. Do not persist it outside this conversation.

- With no saved thread for the role, or with `--new`, call
  `callee.subagent.prompt` with `{"role":"<role>","prompt":"<task>"}`.
- Save the returned `threadId` for that role and return the returned content.
- With a saved thread for the role, call `callee.subagent.reply` with
  `{"threadId":"<saved thread ID>","prompt":"<task>"}` and return its content.
- If reply reports that the thread is unavailable, remove the saved entry, say
  that the prior Callee conversation was lost, and start a new prompt using only
  the current task.

Threads are process-local to Callee. A restarted MCP server cannot continue an
old thread.

### CLI fallback

Announce that MCP is unavailable and that the CLI run cannot be continued.
Run one role invocation only:

```bash
npx --yes @baldaworks/callee@0.3.0 --role "<role>" --prompt "<task>"
```

Do not launch `mcp-server` from fallback mode and do not retain a thread ledger
for CLI results.

## Setup

Explain that the plugin bundles an MCP server configuration. Ask the user to
reload the plugin and verify that Callee appears in the host's MCP tool list.
For a manual Codex setup, provide:

```bash
codex mcp add callee -- npx --yes @baldaworks/callee@0.3.0 mcp-server
```

For a manual Claude Code project setup, provide this `.mcp.json` entry:

```json
{
  "mcpServers": {
    "callee": {
      "command": "npx",
      "args": ["--yes", "@baldaworks/callee@0.3.0", "mcp-server"]
    }
  }
}
```

Do not add MCP forwarding, provider configuration, Gemini support, or nested
role frontmatter.
