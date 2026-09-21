# Running agents

Use `agent run` to validate and resolve one resource tree, execute it, and
receive its root artifact.

## Run a one-shot tree

```bash
callee agent run workflows/investigate \
  --message "Explain the architecture and main entry points"
```

An explicitly blank message is invalid. If `--message` is omitted in an
interactive run, Callee reads the root prompt from the controlling terminal.

## Supply Role parameters

Inspect required keys before execution, then bind values by effective node ID:

```bash
callee agent view workflows/review
callee agent run workflows/review \
  --message "Review the current changes" \
  --param validator.focus=security \
  --param-file worker.context=./request.md
```

Both parameter flags are repeatable. Interactive mode prompts for missing
values. Non-interactive mode requires all values before provider startup.

## Choose run mode and permissions

Without an explicit override, the complete effective tree selects interactive
mode when it contains an interactive Role, an effective `ask` permission, or a
Human. Otherwise it selects non-interactive mode.

Override Role protocol and ACP permissions independently:

```bash
callee agent run workflows/investigate --message "Ask for the target" --interactive=true
callee agent run workflows/investigate --message "Return one artifact" --interactive=false --permissions=deny
```

`--interactive=true` forces every Role visit in the resolved tree into its REPL
protocol. `--interactive=false` forces one-shot Roles. The root-persistent
`--permissions=ask|allow|deny` flag overrides every Role's ACP permission policy
without rewriting resources. `--interactive=false --permissions=ask` is
invalid.

Interactive mode uses the controlling TTY for the root prompt, missing
parameters, Human responses, permission choices, REPL turns, and abort input.
Non-interactive mode never opens `/dev/tty`; it requires an explicit nonblank
message, all parameters, one-shot Roles, automatic permissions, and no Human in
the resolved tree. Use `--repl-timeout`, which defaults to `30m`, to bound each
operator wait.

See [ACP permission requests](acp-permissions.md) for option-selection rules and
[Workflow semantics](../reference/workflow-semantics.md#control-records-and-repl)
for the REPL control protocol.

## Read output and lifecycle data

The successful root artifact is written once to stdout after workflow execution
and cleanup succeed.
Lifecycle events, provider diagnostics, permission events, heartbeats, and
metrics go to stderr. Determine success from the process exit status.

If one provider turn remains active for at least 10 seconds, Callee emits an
`agent turn heartbeat` event with its current `turn_duration`. The final
`agent run finished` event contains run-wide metrics, while each completed Role
visit emits Role-scoped measurements. See
[Execution metrics](../reference/execution-metrics.md).

## Common failures

| Symptom | Check |
| --- | --- |
| `interactive terminal is required` | Attach a controlling TTY or select a compatible non-interactive configuration. |
| `non-interactive mode preflight failed` | Supply the reported message and parameters, use `allow` or `deny`, and remove Human or REPL requirements. |
| Required parameter error | Use the exact effective-node key reported by `agent view`. |
| Unauthorized escalation | Every edge from the nearest Loop to the Role occurrence must set `canEscalate: true`. |
| Provider executable was not found | Install the backend or correct `spec.provider.cmd`. |
| Cleanup error | The provider failed to close; Callee suppresses the otherwise successful artifact. |

Use `--debug` or `--trace` when the default diagnostic identifies too little
context. `--trace` overrides `--debug`.
