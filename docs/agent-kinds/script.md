# Script

Use a Script for a deterministic local check or shell step. It starts `sh`
(the default) or `bash`; it does not start an ACP provider.

## Minimal resource

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: Script
spec:
  description: Runs the Go tests.
  body: go test ./...
```

## Inputs, artifacts, and state

The command body and optional `cwd` and `env` values can render `.Prompt`,
`.Input`, and `.State`. Callee captures stdout, stderr, exit code, and timeout
status. Every completed command writes `status`, `exitCode`, `stdout`,
`stderr`, and `timedOut` to `State.scripts[effectiveId]`. A successful or
continued command returns `status=... exitCode=...` and publishes it to
`State.outputs[effectiveId]`; command stdout is
available only through the structured script state.

## Composition and failures

A Script can be a child of any composite. `timeout` is a positive Go duration
and defaults to `15m`. `onNonZero` defaults to `fail`; `continue` records the
failed command and lets composition continue. A timeout always fails. This
policy is unrelated to Role control records—there is no
`callee.control.v1.continue` record. Command startup and template failures also
fail the visit.

## Inspect and run

```bash
callee agent schema Script
callee agent validate .callee/scripts/test.md
callee agent run scripts/test --message "Run the checks"
```

See the runnable [Human-loop smoke workflow](../../testdata/smoke/callee-human/workflows/human-loop.md),
which supplies the `State.clarification` value required by its state-check
Script child. Also see the
[Script fields](../reference/agent-resources.md#script), and
[Script execution](../reference/workflow-semantics.md#script-execution).
