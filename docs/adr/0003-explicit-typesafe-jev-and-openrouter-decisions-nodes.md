# ADR 0003: Explicit TypeSafe Jev and OpenRouter Decisions nodes

- Status: Accepted
- Date: 2026-09-20
- Supersedes: [ADR 0002](0002-native-jev-node-and-api-adapters.md)

## Context

ADR 0002 exposed one `Jev` kind and selected TypeSafe or OpenRouter with
`spec.api.type`. That surface combined two different concepts. TypeSafe exposes
a concrete Jev/System One API with a Jev model namespace and TypeSafe
environment conventions. OpenRouter exposes a Decisions API whose model is an
argument and whose response includes OpenRouter request, provider, usage, and
cost metadata. Restricting that endpoint to TypeSafe Jev identifiers makes the
public resource less general than the service it represents.

## Decision

Replace `Jev` with two public leaf kinds:

- `TypeSafeJev` names the concrete TypeSafe Jev integration;
- `OpenRouterDecision` names OpenRouter's Decisions API, regardless of which
  Decisions-capable model the author selects.

The asymmetric names are intentional. They describe the stable contract at
each transport boundary rather than inventing a common vendor-neutral product.
The Workflows API is a clean break, so there is no `Jev` compatibility alias,
`spec.api.type` translation, or automatic migration.

Both kinds author `description`, exactly one of Markdown body or structured
`evidence`, typed `questions`, optional node-entry `state`, direct `model`, and
direct `timeout`. `OpenRouterDecision` requires `model`. `TypeSafeJev` resolves
model in this order: authored `spec.model`, nonblank `TYPESAFE_DEFAULT_MODEL`,
then `jev-latest`.

`TypeSafeJev` requires `TYPESAFE_API_KEY`. It uses `TYPESAFE_BASE_URL` when
nonblank, otherwise `https://api.typesafe.ai`, and appends the fixed
`/v1/systemone` path. The configured root must be an absolute HTTPS URL with a
host and without userinfo, query, or fragment. Redirects remain rejected.

`OpenRouterDecision` requires `OPENROUTER_API_KEY` and calls the fixed
`https://openrouter.ai/api/alpha/decisions` endpoint. Callee validates that the
model is one nonblank identifier but does not impose a vendor, Jev prefix, or
response-provider allowlist. OpenRouter remains authoritative about whether a
model supports Decisions.

Internally, both adapters share a provider-neutral evaluation request, typed
answers, request-dependent validation, retry/deadline core, and atomic workflow
publication. Each adapter retains its own response envelope and service rules.
The canonical result uses `service` with common requested/actual model,
answers, and usage fields, plus safe OpenRouter request/provider/cost metadata
when returned.

Lifecycle logs use `evaluation_*` operational fields. They exclude evidence,
questions, answers, credentials, headers, state, artifacts, and sensitive
endpoints. This decision adds no metrics. Doctor checks only the local
configuration required by each reachable kind and makes no inference request.

## Consequences

Authors can tell which service, credential namespace, model rules, and response
metadata apply by reading the kind. OpenRouter Decisions can adopt additional
models without a Callee schema release. TypeSafe retains its native model-family
checks and client environment conventions.

Existing `Jev` resources must be rewritten with a new kind and direct
`spec.model` / `spec.timeout` fields. Stored evaluation results change their
discriminator from `api` to `service`, and operational log fields change from
`jev_*` to `evaluation_*`. Evaluation state keys and composition semantics stay
the same.

OpenRouter Decisions is alpha. Its strict adapter boundary contains future
envelope changes. Unknown answer primitives remain rejected until Callee adds
an explicit typed schema for them.

## References

- [ADR 0002](0002-native-jev-node-and-api-adapters.md)
- [TypeSafe HTTP API](https://docs.typesafe.ai/api)
- [TypeSafe models](https://docs.typesafe.ai/models)
- [OpenRouter Jev 1.13](https://openrouter.ai/typesafe/jev-1.13)
