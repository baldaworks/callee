# Router

Use Router when an authored template can deterministically dispatch exactly one
named branch, with an optional default for blank or unknown route keys. Router
does not ask a provider to choose a route.

## Minimal resource

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Dispatches a classified task.
  route: "{{ .Input }}"
  children:
    - ref: roles/implementer
      route: implement
    - ref: roles/explorer
      default: true
---
{{ .Prompt }}
```

## Selection, payload, and state

Router first renders `spec.route` from `.Prompt`, incoming `.Input`, and
current `.State`, trims surrounding whitespace, and compares it exactly and
case-sensitively with named routes. One match wins; otherwise the optional
`default: true` edge wins. The string `default` is an ordinary named route.

Only after selection does Router independently render its Markdown body as the
selected child's payload. Route text never becomes payload automatically. A
selected edge's `input` may override that payload. Only the selected subtree
runs or changes state. Its artifact is Router's natural artifact; optional
`spec.output` can transform it before publication at
`State.outputs[effectiveId]`.

## Composition and failures

Every child must use mapping form and specify exactly one unique `route` or
`default: true`; there can be at most one default. A selected child may be any
supported kind, subject to restrictions elsewhere in the resolved tree.
Without a default, blank or unknown routes fail before body rendering or child
activity. Route-template errors never select default. A selected child's
failure stops Router and never fails over to default. Router exposes no fan-out,
arbitrary graph edges, persistence, or multiple selected branches.

Authorized descendant escalation keeps its source and artifact and travels to
the nearest Loop under the ordinary all-edges authorization rule.

## Inspect and run

```bash
callee --agent-root examples agent view workflows/routed-task --json
callee --agent-root examples agent run workflows/routed-task \
  --message "Review the authentication change"
```

Use the runnable [task router](../../examples/workflows/task-router.md) inside
the [routed task](../../examples/workflows/routed-task.md). See
[Router fields](../reference/agent-resources.md#router) and
[Router execution](../reference/workflow-semantics.md#router-execution).
