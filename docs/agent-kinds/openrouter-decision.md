# OpenRouterDecision

Use `OpenRouterDecision` when OpenRouter's fixed Decisions endpoint should
answer typed questions using an explicitly chosen Decisions-capable model. It
is a native HTTP evaluation leaf, not an ACP Role, and it does not use chat
completions.

## Minimal resource

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: OpenRouterDecision
spec:
  description: Scores release risk.
  model: typesafe/jev-1.13
  body: "{{ .Input }}"
  questions:
    risk:
      type: score
      instructions: Score release risk from low to high.
      criteria:
        - Low risk
        - High risk
```

Set `OPENROUTER_API_KEY`. `spec.model` is required and is sent to
`https://openrouter.ai/api/alpha/decisions`. Callee does not impose a vendor,
Jev prefix, or other model restriction; OpenRouter decides whether the selected
model supports Decisions.

## Evidence, questions, and results

Use exactly one of a Markdown body or structured `spec.evidence`. Evidence
string leaves, instructions, and criteria render `.Prompt`, `.Input`, and
`.State`. All questions are sent in one request. `noul` may omit criteria, but
when criteria are present OpenRouterDecision requires both `true` and `false`.
`choice` requires 2–255 named choices, and `score` requires 2–10 ordered levels.

A validated response is stored structurally at
`State.evaluations[effectiveId]`. Its deterministic compact JSON is both the
artifact and `State.outputs[effectiveId]`; it can include safe OpenRouter
request ID, provider, token, and cost metadata. Publication is atomic, so a
failed visit does not expose partial data or replace an earlier success.

## Runtime, composition, and failures

The positive `timeout` defaults to 30 seconds and covers the complete request
and retry sequence. Retries are bounded to three attempts for eligible
transport/timeouts and HTTP 408, 429, or 5xx responses. Configuration,
authentication, redirects, invalid responses, and exhausted deadlines fail.
The leaf composes like other non-Human leaves.

Selecting this resource authorizes sending rendered evidence and questions to
OpenRouter. Safe evaluation lifecycle logs omit evidence, questions, answers,
credentials, headers, state, artifacts, and endpoint details. `callee doctor`
checks the required credential and model locally without inference.

## Inspect and run

```bash
export OPENROUTER_API_KEY="<your-key>"
callee --agent-root examples agent view evaluations/openrouter-decision
callee --agent-root examples agent run evaluations/openrouter-decision \
  --message "Authentication changed without a rollback test"
callee --agent-root examples doctor
```

Use the runnable [OpenRouter Decisions example](../../examples/evaluations/openrouter-decision.md).
See [evaluation resource fields](../reference/agent-resources.md#typesafejev-and-openrouterdecision),
[artifact publication](../reference/workflow-semantics.md#artifact-promotion),
and accepted [ADR 0003](../adr/0003-explicit-typesafe-jev-and-openrouter-decisions-nodes.md).
