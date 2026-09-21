# Sequential

Use Sequential when each child must run once in authored order and the result
of one step should naturally become the next step's input.

## Minimal resource

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Explores a task, then plans it.
  children:
    - ref: roles/explorer
      alias: explorer
    - ref: roles/architect
      alias: architect
---
{{ .Input }}
```

## Input, ordering, and state

The body renders a nonblank local input. The first child receives it; every
later child receives the preceding child's artifact. An edge `input` template
overrides that natural input and can read the composite's local `.Input`, the
root `.Prompt`, and current `.State`. Children share the root run's one live
state object and publish under their effective IDs.

On normal completion, the last child artifact is Sequential's natural artifact.
Optional `spec.output` can replace it using `.Output` plus the final state; the
final artifact is published at `State.outputs[effectiveId]`.

## Composition and failures

Children may reference any supported kind, subject to the referenced kind's
restrictions. A child failure stops the sequence. Authorized escalation is
sticky: Sequential records it, still runs remaining children, then propagates
it to the nearest Loop with the final child's artifact; a later failure wins.
Child references, cycles, duplicate effective IDs, invalid parameter bindings,
blank body/output, and an escalation that reaches the root are failures.

## Inspect and run

```bash
callee --agent-root examples agent view workflows/investigate
callee --agent-root examples agent run workflows/investigate \
  --message "Plan support for a new flag"
```

Use the runnable [investigate workflow](../../examples/workflows/investigate.md).
See [composite child syntax](../reference/agent-resources.md#composite-children),
[Sequential fields](../reference/agent-resources.md#sequential), and
[Sequential execution](../reference/workflow-semantics.md#sequential-execution).
