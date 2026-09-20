# ADR 0002: Native Jev node and API adapters

- Status: Superseded by [ADR 0003](0003-explicit-typesafe-jev-and-openrouter-decisions-nodes.md)
- Date: 2026-09-20
- Supersedes: [ADR 0001](0001-native-jev-evaluation-node.md)

## Context

Callee needs typed semantic judgments as a first-class workflow leaf. TypeSafe
serves Jev through its System One API, and OpenRouter serves the same model
family through its Decisions API. Jev returns `noul`, `choice`, and `score`
answers rather than prose. It is therefore neither an ACP Role nor a Script and
must not gain tool or control-flow authority.

## Decision

Add the public `callee.metalagman.dev/v1alpha1` kind `Jev`. It is a native leaf
with no children and runs through Callee's private native-leaf executor boundary.
One visit evaluates one explicitly authored evidence value against one non-empty
map of independent typed questions.

### Resource contract

`spec` contains `description`, `api`, `questions`, optional node-entry `state`,
and exactly one of Markdown `body` or structured `evidence`. Body is string
evidence. Evidence is a JSON-compatible string, object, or array. String leaves
in evidence, instructions, and criteria use the restricted visit-template
surface over `.Prompt`, `.Input`, and `.State`; non-string values retain type.
Callee never sends the complete workflow state implicitly.

`api.type` is `typesafe` or `openrouter`; optional `api.timeout` is the total
visit budget and defaults to 30 seconds. Optional `api.model` defaults to
`jev-latest` for TypeSafe and `~typesafe/jev-latest` for OpenRouter. OpenRouter
accepts only TypeSafe Jev model IDs and aliases.

Questions are keyed by author-selected IDs and use exactly one discriminator:

- `noul` has instructions and optional `true`/`false` criteria;
- `choice` has instructions and 2–255 named criteria;
- `score` has instructions and 2–10 ordered criteria.

Instructions are string, object, or array values. Criteria descriptions may be
structured or null. The checked-in resource schema, rendered request checks,
and request-dependent response checks are separate fail-closed boundaries.

### API and authentication contract

The TypeSafe adapter calls fixed `POST https://api.typesafe.ai/v1/systemone`
with `TYPESAFE_API_KEY`. The OpenRouter adapter calls fixed
`POST https://openrouter.ai/api/alpha/decisions` with `OPENROUTER_API_KEY`.
Both send `{model,state,questions}`. OpenRouter Decisions is not a chat
completion endpoint; arbitrary chat models and OpenAI-compatible chat clients
are outside this decision.

Destinations and authorization headers are not authorable or templated.
Redirects fail. Credentials never enter resources, templates, state, artifacts,
errors, or logs. Selecting a Jev resource authorizes its declared outbound
evaluation; no interactive permission prompt or ACP permission reuse is added.

### Result and execution contract

After the entire response validates, Callee stores the provider-neutral result
under `.State.evaluations[effectiveID]`, stores the same deterministic compact
JSON under `.State.outputs[effectiveID]`, and returns it as the visit artifact.
The result preserves API mode, requested and actual model, typed answers, token
usage, and safe OpenRouter metadata such as provider, request ID, and reported
cost when present. Repeated successes use last-successful-write-wins. A failed
visit publishes no partial or current artifact.

Choice/Score keys and legends must match the rendered request. Probabilities,
confidence, and scores must be finite and in range. Distribution and weighted
score checks allow only the numerical envelope implied by OpenRouter's
documented two-decimal rounding. Callee does not repair or renormalize answers.

One total deadline covers at most one initial attempt plus two retries and all
backoff. Retry connection failures, remaining-budget timeouts, HTTP 408, 429,
and 5xx responses. Honor bounded retry headers; stop immediately for
cancellation, authentication, other client errors, redirects, or malformed
successful responses. Response bodies are bounded.

### Composition and observability

Jev returns an ordinary leaf outcome. Routers, thresholds, escalation, Loop
completion, side effects, and authorization remain explicit workflow logic.
The model cannot request tools or mutate arbitrary workflow state.

Lifecycle completion logs may contain only bounded typed operational fields:
API mode, requested and actual model, provider and request ID when returned,
attempt count, reported token usage and cost, error class, and the existing
status and duration. Evidence, questions, answers, bodies, headers, credentials,
state, and artifacts are excluded. This decision adds no metrics.

## Consequences

Callee gains one auditable typed-decision primitive with two API transports and
one stable workflow result. Operators must configure the credential for the
selected transport and accept remote disclosure of the explicit evidence.
Moving model aliases trade reproducibility for automatic upgrades, so policy
workflows should pin a model and always retain the returned model ID.

The OpenRouter Decisions endpoint is alpha. Its adapter is isolated behind the
same evaluator boundary as TypeSafe so envelope changes do not alter the public
resource or stored result. Offline schema, graph, list, view, and doctor paths
make no paid inference call.

This decision does not add a dedicated Jev CLI, generic HTTP node, chat-model
emulation, provider routing, tools, Parallel workflows, server transport,
thread storage, handle binding, persistent evaluation cache, or new metrics.

## References

- [ADR 0001](0001-native-jev-evaluation-node.md)
- [TypeSafe HTTP API](https://docs.typesafe.ai/api)
- [TypeSafe models](https://docs.typesafe.ai/models)
- [TypeSafe primitives](https://docs.typesafe.ai/primitives)
- [OpenRouter Jev 1.13](https://openrouter.ai/typesafe/jev-1.13)
- [OpenRouter Go SDK](https://github.com/OpenRouterTeam/go-sdk)
