# TypeSafeJev

Use `TypeSafeJev` when TypeSafe System One should answer one or more typed
questions from the same evidence. It is a native HTTP evaluation leaf, not an
ACP Role: it starts no coding-agent session, shell, or tool.

## Minimal resource

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: TypeSafeJev
spec:
  description: Classifies request urgency.
  evidence:
    request: "{{ .Input }}"
  questions:
    urgent:
      type: noul
      instructions: Is this request urgent?
```

Set `TYPESAFE_API_KEY`. The model resolves from `spec.model`, then
`TYPESAFE_DEFAULT_MODEL`, then `jev-latest`, and must identify a Jev model.
`TYPESAFE_BASE_URL` may replace the default `https://api.typesafe.ai` HTTPS
root; Callee appends `/v1/systemone` and rejects redirects.

## Evidence, questions, and results

Author exactly one evidence form: a Markdown body for one string, or
`spec.evidence` for a JSON-compatible string, object, or array. String leaves,
question instructions, and criteria render `.Prompt`, `.Input`, and `.State`.
Questions are batched into one request:

- `noul` produces a typed binary judgment and may define `true`/`false` criteria;
- `choice` requires 2–255 named choices;
- `score` requires 2–10 ordered levels.

After response validation, Callee atomically stores the structured result at
`State.evaluations[effectiveId]` and its deterministic compact JSON at
`State.outputs[effectiveId]`; that compact JSON is also the artifact. A failed
visit publishes no partial result and preserves an earlier successful visit.

## Runtime, composition, and failures

The positive `timeout` is one total request-and-retry budget and defaults to
30 seconds. Transport failures, request timeouts before the total deadline,
HTTP 408, 429, and 5xx responses are retried, with at most three attempts.
Authentication, configuration, redirect, response-shape, and deadline errors
fail without an unbounded retry path. The leaf can be used directly or under
any composite except where the composite itself imposes a restriction.

Selecting this resource authorizes sending its rendered evidence and questions
to TypeSafe. Lifecycle logs include only bounded service/model, attempt,
usage/cost, request/provider, and error-class fields—not evidence, questions,
answers, credentials, headers, state, artifacts, or sensitive endpoints.
`callee doctor` checks local credential, model, and endpoint configuration
without making an inference request.

## Inspect and run

```bash
export TYPESAFE_API_KEY="<your-key>"
callee --agent-root examples agent view evaluations/typesafe-jev
callee --agent-root examples agent run evaluations/typesafe-jev \
  --message "Checkout is failing for every customer"
callee doctor
```

Use the runnable [TypeSafe example](../../examples/evaluations/typesafe-jev.md).
See [evaluation resource fields](../reference/agent-resources.md#typesafejev-and-openrouterdecision),
[artifact publication](../reference/workflow-semantics.md#artifact-promotion),
and accepted [ADR 0003](../adr/0003-explicit-typesafe-jev-and-openrouter-decisions-nodes.md).
