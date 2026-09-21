# Human

Use a Human when execution must stop and collect one nonblank response from
the operator on the controlling terminal. It is a terminal interaction leaf,
not an ACP Role or a multi-turn chat.

## Minimal resource

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: Human
spec:
  description: Requests release approval.
  responseKey: approval
  body: |-
    Approve this release?
    {{ .Input }}
```

## Inputs, artifacts, and state

The body reads `.Prompt`, `.Input`, and `.State`. Callee displays the rendered
body and prompts until the operator enters a nonblank response. It returns that
response as the artifact, writes it to the top-level `State[responseKey]`, and
also publishes it at
`State.outputs[effectiveId]`. `responseKey` cannot be blank or use the reserved
`outputs`, `scripts`, or `evaluations` keys.

## Composition and failures

A Human may appear under Sequential, Loop, or Router. Its presence anywhere in
the resolved tree makes the whole run interactive unless mode was explicitly
overridden; `--interactive=false` rejects the tree during preflight, even when
the Human is in an unselected Router branch. Human is prohibited anywhere
below Parallel. Blank input re-prompts; it does not fail the visit. A missing
terminal/interactor, display or prompt error, timeout, or terminal closure fails
the visit; `/abort` aborts the workflow.

## Inspect and run

```bash
callee agent validate .callee/humans/approval.yaml
callee agent view humans/approval
callee agent run humans/approval --message "Version 1.2.3"
```

See the runnable [Human smoke resource](../../testdata/smoke/callee-human/humans/questions.md),
[Human fields](../reference/agent-resources.md#human),
[Human execution](../reference/workflow-semantics.md#human-execution), and the
[whole-tree TTY rules](../reference/workflow-semantics.md#tty-permissions-and-timeouts).
