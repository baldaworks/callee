# Loop

Use Loop when ordered children should repeat within a fixed bound until an
authorized Role decides the goal is satisfied and escalates.

## Minimal resource

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Repeats review until accepted.
  children:
    - ref: roles/reviewer
      alias: reviewer
      canEscalate: true
  maxIterations: 3
  onExhausted: fail
---
{{ .Input }}
```

## Iterations, artifacts, and state

The body renders local input once. The first iteration starts with that input;
later iterations start with the preceding iteration's last artifact. Within an
iteration, artifacts pipe between ordered children as in Sequential. Explicit
child `input` templates always see the Loop's local input, so use shared
`State.outputs` to retrieve prior results.

`maxIterations` is required and positive. `onExhausted` defaults to `fail`;
`complete` returns the last natural artifact. An optional `spec.output`
transforms either an escalated result or an exhaustion-complete result, which
is then published at `State.outputs[effectiveId]`.

## Escalation and composition

Children may be any supported kind, subject to their restrictions. Only a Role
can emit `callee.control.v1.escalate`. For that occurrence to be authorized,
every edge from its nearest enclosing Loop must set `canEscalate: true`. The
nearest Loop consumes the signal immediately; nested Loops form independent
authorization boundaries and do not complete an outer Loop.

Escalation is sticky while passing through a nested Sequential: remaining
Sequential children run before the signal reaches the Loop. A nested Loop
consumes its own signal. Unauthorized escalation, child failure, blank output,
or `onExhausted: fail` fails the workflow. A normal Role return means the Loop
continues; `callee.control.v1.fail` never requests another iteration.

## Inspect and run

```bash
callee --agent-root examples agent view workflows/goalkeeper --json
callee --agent-root examples agent run workflows/goalkeeper \
  --message "Implement and validate the requested change"
```

Use the runnable [goalkeeper workflow](../../examples/workflows/goalkeeper.md).
See [edge authorization](../reference/agent-resources.md#escalation-authorization),
[Loop fields](../reference/agent-resources.md#loop), and
[Loop execution](../reference/workflow-semantics.md#loop-execution).
