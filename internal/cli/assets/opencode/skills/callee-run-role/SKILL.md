---
name: callee-run-role
description: Run and combine project-defined Callee agents and workflows for coding work. Use when the user asks to delegate investigation, review, implementation, testing, or a named workflow through Callee.
---

# Run Callee agents

Use `callee` when available. Otherwise use the pinned fallback `npx --yes @baldaworks/callee@0.10.0` for every command in the task.

## Discover and select

For each fresh task, inspect the versioned catalog:

```bash
callee agent list --json
```

Resolve a naturally named agent against its exact resource ID or unambiguous description. Inspect the selected tree and every required parameter before execution:

```bash
callee agent view "<agent-id>" --json
```

The selected ID may identify a `Role`, `Sequential`, or `Loop`. Treat all kinds as the same run boundary. Do not invent a separate workflow command.

## Execute

Every run requires a real controlling PTY, even when `--message` supplies the initial prompt. Keep terminal interaction separate from stdout and stderr.

```bash
callee agent run "<agent-id>" \
  --message "<task>" \
  --param "<effective-node-id>.<name>=<value>"
```

Use `--param-file "<effective-node-id>.<name>=<path>"` for exact multiline values. Supply known parameters shown by `agent view`; let Callee collect the rest just in time through the PTY.

Read questions and permission requests from the terminal and answer through the same terminal. A Role inside a workflow may enter REPL mode; its stderr lifecycle has one `entering repl` / `exiting repl` pair, with every `await` turn inside that pair. Do not send `quit`, `exit`, `/done`, or a synthetic completion marker; the Role selects control through Callee's injected protocol.

Wait for automatic root completion. The sole successful root artifact is written to stdout only after provider cleanup succeeds. Info lifecycle events and diagnostics are written to stderr, so use the exit status rather than stderr emptiness to determine success. Treat empty stdout on failure as intentional.

Callee v1alpha1 does not define `Parallel`; do not imply parallel workflow semantics or merge PTYs.

## Report results

Return the final stdout artifact and a concise capability trace. Do not expose provider session IDs, internal handles, or raw terminal transcripts.

## Setup

```bash
callee setup <codex|claude|grok|copilot|opencode>
```

Do not add Gemini, a server transport, a Callee thread store, or handle binding.
