# ADR 0001: Native Jev evaluation node

- Status: Superseded
- Date: 2026-09-20
- Supersedes: None
- Superseded by: [ADR 0002](0002-native-jev-node-and-api-adapters.md)

## Context

Callee currently supports Role, Script, Human, Sequential, Loop, and Router
resources. We want a new native leaf node that evaluates workflow evidence with
TypeSafe's Jev model and exposes typed judgments to subsequent nodes. The user
has explicitly selected a native node; wrapping the API in an existing Script
node is outside this proposal.

Jev is accessed through TypeSafe's System One HTTP API. An evaluation accepts a
model, one evidence value named `state`, and a map of independently evaluated
questions. It returns typed answers, probabilities, the serving model, and usage.
It does not generate prose, execute tools, or accept an arbitrary output JSON
Schema as a generation instruction.

This proposal follows the installed TypeSafe skill and its linked documentation,
including the API and SDK references, migration guide, primitives, patterns, and
cookbooks. Those sources establish the request boundary but leave discrepancies
that must be resolved before a public Callee schema is finalized.

## Proposed decision

Introduce one native leaf kind whose execution represents one logical System One
evaluation. A visit sends all independent questions over the same evidence in
one request, with bounded transport retries. Dependent evaluations use later
visits whose evidence or questions incorporate earlier results.

The public kind name is undecided. `Judge` and `Jev` are working names, not
accepted schema values. This ADR establishes the integration boundary without
freezing resource field names or publishing executable YAML examples.

### HTTP API boundary

The native Go implementation will call:

```http
POST https://api.typesafe.ai/v1/systemone
Authorization: Bearer <API_KEY>
Content-Type: application/json
```

The request retains TypeSafe's v1 shape:

```json
{
  "model": "jev-1.13.0",
  "state": {"request": "Review the proposed changes."},
  "questions": {
    "activity": {
      "type": "choice",
      "instructions": "Which activity does `request` ask for?",
      "criteria": {
        "review": "Evaluate existing changes.",
        "implement": "Create or modify code.",
        "other": "Neither activity is requested."
      }
    }
  }
}
```

This is an illustrative HTTP payload, not a Callee resource. Versioned model IDs
and aliases are supported; workflows with evaluated thresholds should pin a
version. Preserve the actual model ID returned by the service. Do not require a
pinned ID to appear in `GET /v1/models`: the documentation says that listing
currently exposes aliases while accepting versioned IDs for evaluation.

Use a dedicated HTTP client boundary, separate from Role's ACP provider
configuration. Norma Runtime continues to own ACP process logic. This outbound
integration does not introduce a Callee server transport or provider session.

### Authentication and authorization

Resolve the API key at runtime, initially using the documented
`TYPESAFE_API_KEY` environment convention. A credential profile or alternative
secret reference remains an open authoring decision. Secrets must not become
resource literals, template values, shared-state entries, artifacts, or trace
fields. Coding-agent plugin installation and ACP provider login do not authenticate
TypeSafe calls.

Keep the service destination fixed in the initial integration. Any future
endpoint override belongs to trusted operator configuration, with an explicit
credential scope. Do not let rendered evidence or model answers select the
destination or authorization headers; reject redirects for evaluation calls.

The key authenticates an API request. It does not grant a model answer authority
to execute a tool, approve an action, or end a Loop. Existing workflow and
edge-authorization rules continue to govern execution. ACP permission choices
must not be silently reused as HTTP authorization semantics. Whether the new
node needs a separate operator policy for outbound evaluation is unresolved.

### Evidence and rendering

TypeSafe request `state` is evidence sent to a remote service. Callee `.State` is
the shared root-run object, and authored `spec.state` already means a workflow
state modifier. These meanings must remain distinct.

Authors explicitly select the evidence sent to Jev. Support strings, objects,
and arrays while preserving nested JSON types. Do not implicitly send the whole
root-run state. Render against a defined visit snapshot using Callee's existing
deterministic template rules, then encode JSON structurally. Inserted evidence
must not undergo a second template evaluation.

The relationship between the Markdown body and structured evidence remains
open. So does the mechanism for building dynamic question maps and candidate
sets. The extraction and verification cookbooks demonstrate that these dynamic
structures are useful; a string-only prompt contract would restrict them.

### Schemas and validation

Maintain three distinct contracts:

1. The versioned Callee resource schema, for authoring and static validation.
2. The rendered TypeSafe request schema, checked before sending a request.
3. The response schema plus request-dependent checks, applied before publishing
   answers into workflow state.

Question and answer schemas use a discriminated union on `type`:

| Type | Question criteria | Answer fields besides `type` |
| --- | --- | --- |
| `choice` | Map of option names to descriptions; descriptions may be structured or null. | `choice`, `probabilities`, `confidence` |
| `noul` | Optional descriptions for `true` and `false`. | `noul` |
| `score` | Ordered level descriptions; recommend 2–10 levels. | `score`, `legend`, `probabilities`, `confidence` |

Preserve structured instructions and descriptions. Do not reduce them to Go
strings. The documented Choice ceiling is 255 options; the documented Score
ceiling is 10 levels. Treat a two-level minimum as an explicit Callee policy if
adopted, since the HTTP reference phrases it as a recommendation.

Before freezing these schemas, reconcile the following source discrepancies:

- The HTTP reference describes Score `legend` values as strings. The Score
  examples and Python SDK JSON Schema also allow objects and arrays.
- The advanced guide permits null instructions and null Score descriptions.
  The HTTP reference is narrower, and the Python SDK schema permits null
  instructions but excludes null Score descriptions.
- SDK-generated schemas include client defaults and omit some service limits.
  They describe client types, not a complete server validation contract.

Pin the selected contract in the repository with source/version provenance;
runtime validation must not fetch mutable remote schemas. Confirm disputed
behavior through upstream clarification or focused integration tests before
claiming full compatibility.

Response checks must match answer IDs and types to the rendered questions,
verify Choice labels and probability keys against supplied options, and verify
Score level keys and ranges against its rubric. Check finite probability and
confidence values in `[0, 1]` and distribution sums with numerical tolerance.
Preserve Noul as a probability; do not invent a separate confidence field.

### Execution, failures, and observability

A node visit has a total deadline covering requests and retry delays, observes
root cancellation, and uses bounded retries with backoff and retry headers.
Authentication and request-validation failures terminate the visit. Rate limits
and transient service failures may retry within the budget. The exact defaults
remain open; TypeSafe's retry reference is the starting point.

Retries are additional HTTP attempts and may incur additional cost. Do not
promise exactly-once evaluation or infer an idempotency contract absent upstream
support. Record attempts separately from completed node visits.

Validate the complete response before atomically publishing its structured
result and artifact. An uncertain answer is a successful evaluation, distinct
from an HTTP failure, timeout, or malformed response. Failed visits must not
publish partial answers or pass an earlier visit's result off as current.

Retain typed answers and their distributions in an engine-owned result location,
and expose a JSON artifact through the existing output boundary. The exact state
key and artifact envelope are undecided. Repeated successful visits should use
the existing last-successful-write-wins convention.

Capture duration, attempt count, requested and actual model, and reported token
usage. Do not fabricate usage for failed attempts. Exclude authorization headers
and evidence bodies from ordinary lifecycle logs. Offline validation and graph
inspection must not make paid evaluation calls. Online authentication checks,
if added to doctor, must be explicitly distinguished from inference.

### Composition and scope

Router remains deterministic: it consumes answers through authored routing
logic. Keep thresholds, aggregation, and fallback choices explicit in workflows.
A low-confidence answer does not automatically select Router's default branch,
which currently handles only blank or unknown route keys.

The new leaf has no implicit Loop escalation capability. A later ADR must decide
whether to extend that authority beyond Roles. Independent questions inside one
remote evaluation do not introduce Parallel workflows or concurrent mutations
of Callee's shared state.

The initial integration adds no tool dispatcher, generic HTTP node, server,
thread store, or persistent evaluation cache. Function-calling and extraction
cookbooks describe compositions built around judgments; they do not change the
leaf's execution authority.

## Alternatives considered

- **An existing Script wrapper:** explicitly rejected for this feature; it does
  not deliver the requested native resource and runtime contract.
- **Jev as a Role ACP provider:** mismatches its HTTP evaluation API and adds
  session and tool semantics that this integration does not need.
- **One kind per primitive:** encourages separate calls for questions sharing
  evidence. One evaluation kind can batch all three question types.
- **A generic authenticated HTTP node:** broadens the product and leaves
  TypeSafe-specific schemas and result semantics to authors.
- **Arbitrary output JSON Schema:** does not match System One's documented API.
  Its question definitions constrain answers through fixed primitives.

## Consequences

The proposal makes typed semantic decisions available throughout Callee's
existing workflow model while keeping control flow explicit. It requires a
native resource schema, codecs, discovery and graph support, HTTP execution,
credential handling, structured results, metrics, and authoring documentation.

It also adds remote-service availability, request budgets, and credential
configuration to runtime operation. Typed answers establish structure, not
semantic correctness; representative evaluations are needed for each workflow's
questions and thresholds. Cookbook timings and thresholds are examples rather
than acceptance criteria for Callee.

## Supersession

This ADR records a provisional direction and does not authorize implementation
or establish shipped behavior. The replacement ADR should settle the kind name,
resource version and fields, body/evidence rendering, dynamic questions,
credential configuration, outbound-call policy, precise schemas, result
location, failure semantics, and retry defaults.

When the replacement is accepted, retain this file, change its status to
`Superseded`, link the replacement in `Superseded by`, and have the replacement
link back to ADR 0001. Preserve the original rationale rather than rewriting it
to appear to describe the final decision.

## References

- [TypeSafe skill](../../.agents/skills/typesafe-ai/SKILL.md)
- [Documentation index](https://docs.typesafe.ai/llms.txt)
- [HTTP API](https://docs.typesafe.ai/api)
- [Question schemas](https://docs.typesafe.ai/sdk/python/api/types/questions)
- [Answer schemas](https://docs.typesafe.ai/sdk/python/api/types/responses)
- [Advanced structure](https://docs.typesafe.ai/primitives/advanced)
- [Score](https://docs.typesafe.ai/primitives/score)
- [Migration to v1](https://docs.typesafe.ai/migrating-to-v1)
- [Models and request limits](https://docs.typesafe.ai/models)
- [Retry policy](https://docs.typesafe.ai/sdk/python/api/retries)
- [Function calling](https://docs.typesafe.ai/cookbooks/function_calling)
- [Structured extraction cascade](https://docs.typesafe.ai/cookbooks/sde_cascade)
- [Callee architecture](../concepts/architecture.md)
- [Current resource schema](../../internal/agent/schema.json)
