# Parallel

Use Parallel for independent, unattended branches that should fan out and join.
Callee compiles direct children into an ADK fan-out and Join; there is no
author-facing concurrency knob.

## Minimal resource

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Parallel
spec:
  description: Runs two independent reviews.
  children:
    - ref: roles/reviewer
      alias: correctness
    - ref: roles/tester
      alias: tests
---
{{ .Input }}
```

## Fan-out, artifacts, and state

Parallel renders its body and pre-renders every direct-child input from one
coherent state snapshot before starting any branch. All branches then use the
root run's one live shared state: a later read can observe a sibling commit,
same-key writes follow actual commit order, and there is no merge or rollback.

Join waits for every started branch. Success naturally produces compact JSON
mapping direct-child effective IDs to artifact strings in authored order.
Optional `spec.output` renders after Join; the final artifact is published at
`State.outputs[effectiveId]`. Failures and escalation precedence are also
reported in authored order, independent of completion order. A failed branch
does not undo sibling publications, but the Parallel itself publishes no output.

## Composition and constraints

Nested Role, Script, TypeSafeJev, OpenRouterDecision, Sequential, Parallel,
Loop, and Router nodes retain their usual behavior. `Human` is prohibited
anywhere in the subtree. Descendant Roles are always one-shot; authored or
default `permissions: ask` automatically becomes `allow`, while `deny` remains
`deny`. Explicit `--interactive=true`, explicit `--permissions=ask`, and
missing Role parameters fail preflight before fan-out. Branch failures fail the
Parallel after Join.

## Inspect and run

```bash
callee --agent-root examples agent view workflows/parallel-review --json
callee --agent-root examples agent run workflows/parallel-review \
  --message "Investigate the reported regression"
```

Use [parallel-review](../../examples/workflows/parallel-review.md) and
[parallel-then-plan](../../examples/workflows/parallel-then-plan.md). See
[Parallel fields](../reference/agent-resources.md#parallel) and
[Parallel execution](../reference/workflow-semantics.md#parallel-execution).
