---
name: callee-run-agent
description: Run and combine project-defined Callee agents and deterministic workflows for coding work. Use when the user asks to delegate investigation, review, implementation, testing, or a named workflow through Callee.
---

# Run Callee agents

Use `callee` when available. Otherwise use the pinned fallback `npx --yes @baldaworks/callee@0.20.1` for every command in the task.

## Discover and select

For each fresh task, inspect the versioned catalog:

```bash
callee agent list --json
```

Resolve a naturally named agent against its exact agent ID or unambiguous description. Inspect the selected tree and every required parameter before execution:

```bash
callee agent view "<agent-id>" --json
```

The selected ID may identify a `Role`, `Script`, `Human`, `Sequential`, `Loop`, or `Router`. Treat all kinds as the same run boundary. Do not invent a separate workflow command.

## Select the execution path

Inspect the authored tree first with `agent view --json`. Preserve its
permission policy unless the user explicitly requests an override. When using
`--permissions`, inspect the effective projection with the same override before
running:

```bash
callee --permissions="<ask|allow|deny>" agent view "<agent-id>" --json
```

Read top-level `specDrivenInteractive` as the authored baseline and top-level
`interactive` as the effective whole-run mode after the permission override.
For every Role, inspect `authoredInteractive`, effective `interactive`,
`authoredPermissions`, and effective `permissions`. Keep permissions and the
Role protocol independent.

Choose the host execution path from this matrix. Apply an explicit
`--interactive` row first; otherwise use the effective tree rows:

| Condition | Whole-run mode and Role protocol | Host path |
| --- | --- | --- |
| One-shot Roles with effective `ask`, no Human | Interactive run; Roles remain one-shot | Controlling PTY |
| One-shot Roles with effective `allow` or `deny`, no Human | Non-interactive run | Direct, without a PTY |
| Any effective interactive Role or any Human | Interactive run | Controlling PTY |
| Explicit `--interactive=true` with any permission mode | Interactive run; every Role uses REPL | Controlling PTY |
| Explicit `--interactive=false` with effective `allow` or `deny` | Non-interactive run; every Role is one-shot | Direct after preflight |

Reject explicit `--interactive=false` with effective `ask`. Before every
non-interactive run, supply a nonblank message and every required parameter and
confirm that the complete resolved tree contains no Human. Callee performs the
same checks before creating a provider, including Human nodes beneath an
unselected Router branch.

For an interactive run, use a real controlling PTY. Keep terminal interaction separate from stdout and stderr. Verify `/dev/tty` in the same shell invocation
before launching Callee; a host tool's `tty` option alone may not create a
controlling terminal. If `test -r /dev/tty && test -w /dev/tty` fails on Linux,
allocate one with util-linux `script` and unique capture paths:

```bash
callee_capture_dir="$(mktemp -d)"
script -qefc "callee agent run \"<agent-id>\" --message \"<task>\" > \"${callee_capture_dir}/artifact\" 2> \"${callee_capture_dir}/diagnostics\"" /dev/null
```

On BSD/macOS, use `script -q /dev/null /bin/sh -c '<callee command with
redirections into the unique capture directory>'`. Keep `/dev/tty` attached for
prompts, inspect the wrapper's exit status, and read the artifact and
diagnostics files separately. Remove the capture directory after consuming
both files.

For a non-interactive run, invoke Callee directly without allocating a PTY.

```bash
callee agent run "<agent-id>" \
  --message "<task>" \
  --param "<effective-node-id>.<name>=<value>"
```

To override authored Role policy for one run, pass an explicit boolean:

```bash
callee agent run "<agent-id>" --message "<task>" --interactive=true
callee agent run "<agent-id>" --message "<task>" --interactive=false --permissions=deny
callee --permissions=allow agent run "<agent-id>" --message "<task>"
```

`--interactive=true` forces every Role visit in the selected run, including
nested, aliased, and repeated Loop visits, through the existing REPL protocol.
`--interactive=false` forces every Role visit through the existing one-shot
protocol. When omitted, each Role keeps its authored `spec.interactive` (or
legacy `spec.repl`) setting. This override is runtime-only: it does not rewrite
specs or resources.

The root-persistent `--permissions=ask|allow|deny` flag separately overrides
every Role's ACP policy for the invocation. Use the same permission override
for inspection and execution. It does not enable or disable Role REPL. Without
an explicit `--interactive`, Callee derives whole-run mode after the permissions
override: any effective interactive Role, `ask`, or Human makes the run
interactive; otherwise it is non-interactive.

Use `--param-file "<effective-node-id>.<name>=<path>"` for exact multiline values. Supply known parameters shown by `agent view`; only interactive mode can collect the rest just in time through the PTY.

Read Human prompts, Human responses, Role questions, and permission requests through the same terminal. A Role inside a workflow may enter REPL mode; its stderr lifecycle has one `entering repl` / `exiting repl` pair, with every `await` turn inside that pair. Do not send `quit`, `exit`, `/done`, or a synthetic completion marker; the Role selects control through Callee's injected protocol.

Wait for automatic root completion. The sole successful root artifact is written to stdout only after provider cleanup succeeds. Info lifecycle events and diagnostics are written to stderr, so use the exit status rather than stderr emptiness to determine success. Treat empty stdout on failure as intentional.

For a Router, read the named-route or `default=true` selection from lifecycle diagnostics. A default child handles only a blank or unknown route key. A route-template error, no-match without default, or selected-child failure is a failed run; never retry another Router branch or describe default as failure failover.

Callee v1alpha1 does not define `Parallel`; do not imply parallel workflow semantics or merge PTYs.

## Report results

Return the final stdout artifact and a concise capability trace. Then read the structured lifecycle events from stderr and include these execution metrics in the final response:

- From the final `agent run finished` event, report `agent_duration`, `agent_wait_duration`, `agent_token_usage`, and every emitted numeric `agent_*_tokens` field.
- From every Role `agent finished` event, report its effective ID and visit together with `role_provider`, `role_model`, `role_reasoning`, `role_token_usage`, every emitted numeric `role_*_tokens` field, and `role_duration` plus `role_wait_duration` when present.

Keep repeated and nested Role visits separate. Preserve `complete`, `partial`, or `unavailable` token status and do not invent numeric or duration fields that the event omitted. On failure, report the exit status and any metrics emitted before termination. Do not expose provider session IDs, internal handles, or raw terminal transcripts.

## Setup

```bash
callee setup <codex|claude|grok|copilot|opencode|cursor>
```

Do not add Gemini, a server transport, a Callee thread store, or handle binding.
