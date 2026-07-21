---
name: callee-run-agent
description: Run and combine project-defined Callee agents and deterministic workflows for coding work. Use when the user asks to delegate investigation, review, implementation, testing, or a named workflow through Callee.
---

# Run Callee agents

Use `callee` when available. Otherwise use the pinned fallback `npx --yes @baldaworks/callee@0.15.0` for every command in the task.

## Discover and select

For each fresh task, inspect the versioned catalog:

```bash
callee agent list --json
```

Resolve a naturally named agent against its exact agent ID or unambiguous description. Inspect the selected tree and every required parameter before execution:

```bash
callee agent view "<agent-id>" --json
```

The selected ID may identify a `Role`, `Sequential`, or `Loop`. Treat all kinds as the same run boundary. Do not invent a separate workflow command.

## Execute

Every run requires a real controlling PTY, even when `--message` supplies the initial prompt. Keep terminal interaction separate from stdout and stderr.

Verify `/dev/tty` in the same shell invocation before launching Callee; a host tool's `tty` option alone may not create a controlling terminal. If `test -r /dev/tty && test -w /dev/tty` fails on Linux, allocate one with util-linux `script`:

```bash
script -qefc 'callee agent run "<agent-id>" --message "<task>" > /tmp/callee-artifact.txt 2> /tmp/callee-diagnostics.txt' /dev/null
```

On BSD/macOS, use `script -q /dev/null /bin/sh -c '<callee command with the same redirections>'`. Keep `/dev/tty` attached for prompts, use unique temporary output paths, inspect the wrapper's exit status, and read the artifact and diagnostics files separately. Do not first launch Callee without a verified controlling terminal; that produces an intentional failed run rather than a useful capability probe.

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
callee setup <codex|claude|grok|copilot|opencode|cursor>
```

Do not add Gemini, a server transport, a Callee thread store, or handle binding.
