# Role

Use a Role when a coding agent must reason, use tools, or change a repository.
A Role renders instructions and sends them through an ACP provider. Every Role
visit creates and prepares a fresh provider session, including repeated visits
inside a Loop; compatible Roles may reuse the underlying provider process.
Use [DynamicRole](dynamic-role.md) instead only when provider fields must render
from visit-time state. A Role's provider configuration is always concrete and
is never interpreted as a template.

## Minimal resource

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews one requested change.
  provider:
    type: codex
---
Review this change and return verified findings:

{{ .Input }}
```

The body must contain exactly one unconditional bare `{{ .Input }}` or
`{{ .Prompt }}` action. It may also read `.State` and declared `.Params`.

## Inputs, artifacts, and state

`.Prompt` is the immutable root request; `.Input` is this occurrence's input.
Qualified `--param node.name=value` and `--param-file node.name=path` values,
or direct child bindings, populate `.Params`. A normal nonblank final response
is the artifact and is published at `State.outputs[effectiveId]`.

One-shot Roles may return plain text or an exact final-line control record.
With `interactive: true`, every response must end in `await`, `return`,
authorized `escalate`, or `fail`; `await` asks the operator for another turn in
the same visit and session. This interactive protocol is separate from ACP
permissions. `permissions.mode` controls provider tool approval and defaults to
`ask`; the invocation-wide `--permissions` override accepts `ask`, `allow`, or
`deny`.

## Composition and failures

A Role can be a child of any composite. Child aliases determine parameter and
state keys. An `escalate` is legal only when every edge from the nearest Loop
to this occurrence has `canEscalate: true`; otherwise it fails the workflow.
Provider startup, session preparation, turns, blank artifacts, malformed
control records, and `fail` can also fail the run. Under Parallel, Roles are
forced to one-shot execution and effective `ask` becomes automatic `allow`.

## Inspect and run

```bash
callee agent validate .callee/roles/reviewer.md
callee agent view roles/reviewer --json
callee agent run roles/reviewer --message "Review the current diff"
```

Run the checked-in [reviewer example](../../examples/roles/reviewer.md). See
[Role fields](../reference/agent-resources.md#role),
[Role execution](../reference/workflow-semantics.md#role-execution),
[control records](../reference/workflow-semantics.md#control-records-and-repl),
and [ACP permissions](../guides/acp-permissions.md).
