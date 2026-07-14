---
name: callee
description: Run Markdown-defined Callee roles as subagents. Use when the user asks to run a Callee role, delegate work to a named Callee role, continue a role conversation, or configure Callee MCP.
user-invocable: true
---

# Callee

## Invocation

Use one of these actions:

- `role:<role-id> <task>` runs a role or continues its active conversation.
- `reset:<role-id>` forgets that role's active conversation.

Role IDs can include `/` but cannot include spaces. The `role:` prefix is part
of the invocation syntax, not part of the role ID.

Run Callee roles through MCP whenever `callee.role` is available. Use the CLI
fallback only when that tool is unavailable before dispatch, such as when it is
not registered or its MCP server cannot start. Do not retry through the CLI
after an MCP tool call starts and then errors, times out, or returns an invalid
result.

## Dispatch

For `role:<role-id>`, dispatch the supplied role directly. The text after the
first space is its task. Ask for the missing role or task instead of guessing.

For `reset:<role-id>`, require a non-empty role ID. Do not call MCP or the CLI;
apply the reset to the active thread ledger below.

Call `callee.role.list` only when the user explicitly asks what roles exist or
needs help choosing one. In CLI fallback mode, use the role list for the same
purpose:

```bash
npx --yes @baldaworks/callee@0.5.0 role list
```

For an unknown role in CLI mode, let the role runner report the available IDs.

### MCP mode

Maintain the active Callee thread in the current parent conversation for each
role ID. Do not persist it outside this conversation.

- For `role:<role-id>` with no saved thread, call `callee.role` with
  `{"role":"<role>","prompt":"<task>"}`.
- Save the returned `threadId` as that role's active thread and return the
  returned content.
- For `role:<role-id>` with a saved thread, call `callee.role.reply` with
  `{"threadId":"<saved thread ID>","prompt":"<task>"}` and return its content.
- If reply reports that the thread is unavailable, remove the saved entry, say
  that the prior Callee conversation was lost, and start a new prompt using only
  the current task.
- For `reset:<role-id>`, remove that role's saved entry and confirm that the
  next `role:<role-id>` call will start a fresh conversation. Do not try to
  close the old ACP session.

Threads are process-local to Callee. A restarted MCP server cannot continue an
old thread.

### CLI fallback

Announce that MCP is unavailable and that the CLI run cannot be continued.
For `role:<role-id>`, run one role invocation only:

```bash
npx --yes @baldaworks/callee@0.5.0 --role "<role>" --prompt "<task>"
```

For `reset:<role-id>`, report that CLI mode has no persistent Callee
conversation to reset; do not run the CLI. Do not launch `mcp-server` from
fallback mode and do not retain a thread ledger for CLI results.

## Setup

Explain that the plugin bundles an MCP server configuration. Ask the user to
reload the plugin and verify that Callee appears in the host's MCP tool list.
For a manual Codex setup, provide:

```bash
codex mcp add callee -- npx --yes @baldaworks/callee@0.5.0 mcp-server
```

For a manual Claude Code project setup, provide this `.mcp.json` entry:

```json
{
  "mcpServers": {
    "callee": {
      "command": "npx",
      "args": ["--yes", "@baldaworks/callee@0.5.0", "mcp-server"]
    }
  }
}
```

For a manual Grok Build project setup, provide:

```bash
grok mcp add --scope project callee -- npx --yes @baldaworks/callee@0.5.0 mcp-server
```

Do not add MCP forwarding, provider configuration, Gemini support, or nested
role frontmatter.
