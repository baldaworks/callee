# Agent resource format

Use this reference when authoring or reviewing a Callee resource. The checked-in [Draft 2020-12 JSON Schema](../../internal/agent/schema.json) defines the structural contract; Callee also enforces semantic, template, state, and graph constraints in code. Use `callee agent schema <Role|Script|Human|TypeSafeJev|OpenRouterDecision|Sequential|Parallel|Loop|Router>` to print a standalone schema document for one kind.

## Discovery and IDs

Callee recursively discovers regular files under both of these roots:

- `$XDG_CONFIG_HOME/callee`, or `$HOME/.config/callee` when `XDG_CONFIG_HOME` is unset;
- `.callee` in the current working directory.

Pass `callee --agent-root <dir> ...` to switch into exclusive discovery mode.
When set, Callee ignores both default roots and discovers resources only under
`<dir>`. The resource ID remains relative to that directory.

Only lowercase `.md`, `.yaml`, and `.yml` extensions are supported. Symlinked files and unsupported extensions are skipped. Recursive discovery and remote import also skip Markdown files without YAML frontmatter and structurally valid Markdown, YAML, or YML documents whose `apiVersion` is absent or is not `callee.metalagman.dev/v1alpha1`; `README.md` is a common example. Malformed frontmatter/YAML and documents that declare the current API remain errors. Direct `callee agent validate <file>` remains strict even for a file discovery would skip. The resource ID is the slash-separated relative path with its final supported extension removed. For example, `.callee/workflows/review.yml` has ID `workflows/review`.

IDs must be unique across both roots and all supported formats. Project resources do not shadow user resources. A duplicate such as `roles/reviewer.md` and `roles/reviewer.yaml` makes registry loading fail.

Directories are namespaces, not kind selectors. A `Role` can technically reside outside `roles/`, but `roles/` and `workflows/` keep catalogs understandable.

## Versioned envelope

Every resource has exactly three top-level fields:

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec: {}
```

The only accepted API version is `callee.metalagman.dev/v1alpha1`. Supported kinds are `Role`, `Script`, `Human`, `TypeSafeJev`, `OpenRouterDecision`, `Sequential`, `Parallel`, `Loop`, and `Router`. Unknown fields are rejected at every schema-defined object boundary.

## Markdown and YAML representations

Markdown is the base authoring format. YAML frontmatter contains the envelope and the physical Markdown following the closing delimiter becomes `spec.body` byte-for-byte:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews one change.
  provider:
    type: codex
---
Review this task:

{{ .Input }}
```

Do not place `spec.body` in Markdown frontmatter. YAML represents the same complete object and must author `spec.body` inline:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/baldaworks/callee/main/internal/agent/schema.json
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews one change.
  provider:
    type: codex
  body: |
    Review this task:

    {{ .Input }}
```

A YAML file must contain exactly one UTF-8 document. Markdown frontmatter and its physical body must also be valid UTF-8.

## Common `spec` fields

All kinds require a nonblank `description`. Evaluation kinds require exactly one of `body` or structured `evidence`; every other kind requires a nonblank `body`. Every kind may declare `state`, whose values are described under [State modifiers](#state-modifiers).

The supported fields differ by kind:

| Field | Role | Script | Human | TypeSafeJev | OpenRouterDecision | Sequential | Parallel | Loop | Router |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `description` | Required | Required | Required | Required | Required | Required | Required | Required | Required |
| `body` | Required | Required | Required | One of body/evidence | One of body/evidence | Required | Required | Required | Required |
| `evidence` | Not allowed | Not allowed | Not allowed | One of body/evidence | One of body/evidence | Not allowed | Not allowed | Not allowed | Not allowed |
| `model` | Not allowed | Not allowed | Not allowed | Optional | Required | Not allowed | Not allowed | Not allowed | Not allowed |
| `timeout` | Not allowed | Optional positive Go duration | Not allowed | Optional positive Go duration | Optional positive Go duration | Not allowed | Not allowed | Not allowed | Not allowed |
| `questions` | Not allowed | Not allowed | Not allowed | Required, nonempty | Required, nonempty | Not allowed | Not allowed | Not allowed | Not allowed |
| `state` | Optional | Optional | Optional | Optional | Optional | Optional | Optional | Optional | Optional |
| `provider` | Required | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `permissions` | Optional | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `interactive` | Optional | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `params` | Optional | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `responseKey` | Not allowed | Not allowed | Required, nonblank | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `shell`, `cwd`, `env`, `onNonZero` | Not allowed | Kind-specific optional fields | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `children` | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Required | Required | Required | Required mappings |
| `route` | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Required |
| `output` | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Optional | Optional | Optional | Optional |
| `maxIterations`, `onExhausted` | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Loop fields | Not allowed |

## Role

A `Role` is a provider-backed leaf:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews a change with a requested focus.
  provider:
    type: codex
    timeout: 20m
  permissions:
    mode: ask
  params:
    focus: Area that needs the closest review
---
Task:

{{ .Input }}

Focus: {{ index .Params "focus" }}
```

The body must contain exactly one unconditional, bare `{{ .Prompt }}` or `{{ .Input }}` action. The insertion cannot be hidden in a conditional, pipeline, nested template, or helper call. Legacy `{{ prompt }}` and flat parameter actions are rejected.

`spec.params` maps parameter names to nonblank operator-facing descriptions. Names match `^[A-Za-z][A-Za-z0-9_-]*$`. At runtime, unbound values use `<effective-node-id>.<name>` keys. Access parameters with `.Params.name` when the name permits field syntax or with `index .Params "name"` for the general case.

`spec.interactive: true` allows multiple operator turns in one Role visit. Its execution contract is defined in [Control records and REPL](workflow-semantics.md#control-records-and-repl). Legacy `spec.repl` remains accepted as a compatibility alias.

See [ACP provider configuration](../guides/acp-providers.md) for the `provider` object.

`spec.permissions.mode` accepts exactly `ask`, `allow`, or `deny` and defaults to `ask` when `permissions` is omitted. It is a Role-only runtime policy and is independent of backend-specific `spec.provider.mode` and the Role's interactive protocol. The root-persistent `--permissions` flag can override the effective value for one invocation. See [ACP permission requests](../guides/acp-permissions.md) for option selection and failure semantics.

`callee agent view <agent-id> --json` reports `specDrivenInteractive` and the
effective whole-tree `interactive` value. Every resolved Role reports the
spec-driven `authoredInteractive` value, effective `interactive`,
`authoredPermissions`, and effective `permissions`. A `--permissions` override
changes only the effective permission and aggregate interactive projection.

## Script

A `Script` is a local deterministic validator leaf:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Script
spec:
  description: Runs the Go test suite.
  shell: sh
  onNonZero: continue
  env:
    PKG: ./...
---
go test "$PKG"
```

The body uses the restricted template surface: `.Prompt`, `.Input`, and `.State` are available, while `.Params` and `.Output` are not. `shell` defaults to `sh`; `bash` is the only other supported value. `cwd`, `env`, and `body` string values may render templates from the restricted surface.

`spec.timeout` is a positive Go duration and defaults to `15m`. `spec.onNonZero` defaults to `fail`; set it to `continue` when a later child or later Loop iteration should inspect the validator result and keep going. This field controls Script exit handling and is unrelated to Role [control records](workflow-semantics.md#control-records-and-repl); there is no `callee.control.v1.continue` record.

Every completed Script visit records a result object at `State.scripts[effectiveId]` with `status`, `exitCode`, `stdout`, `stderr`, and `timedOut`. A successful or continued Script visit also promotes a compact summary artifact to `State.outputs[effectiveId]`.

## Human

A `Human` is an operator-backed interactive leaf:

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: Human
spec:
  description: Requests a release approval.
  responseKey: approval
  body: |
    Release candidate:
    {{ .Input }}
```

The body uses the restricted template surface: `.Prompt`, `.Input`, and `.State` are available, while `.Params` and `.Output` are not.

`spec.responseKey` names the top-level shared-state entry that receives the collected operator response. The key must be nonblank and cannot be the reserved `outputs`, `scripts`, or `evaluations` keys.

At runtime, Callee renders `body`, displays the rendered text on the controlling terminal, prompts once for a nonblank response, stores that string at `State[responseKey]`, and also promotes it to `State.outputs[effectiveId]`.

## TypeSafeJev and OpenRouterDecision

Both kinds are typed remote judgment leaves. They batch every question into one
request and never starts ACP, a shell, or a tool:

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: TypeSafeJev
spec:
  description: Classifies a support request.
  model: jev-1.13
  timeout: 30s
  evidence:
    request: "{{ .Input }}"
    customerTier: "{{ .State.customerTier }}"
  questions:
    urgent:
      type: noul
      instructions: Is this request urgent?
    queue:
      type: choice
      instructions: Select the handling queue.
      criteria:
        support: Product support work.
        security: A possible security incident.
```

`TypeSafeJev` reads `TYPESAFE_API_KEY`. Its model resolves from `spec.model`,
then `TYPESAFE_DEFAULT_MODEL`, then `jev-latest`; its API root resolves from
`TYPESAFE_BASE_URL` or the official TypeSafe root. `OpenRouterDecision` reads
`OPENROUTER_API_KEY`, requires `spec.model`, and uses `/api/alpha/decisions`,
not chat completions. Callee does not restrict its model to TypeSafe or Jev.
The explicit evidence and questions are sent to the selected remote service.

Use exactly one evidence representation: Markdown body for a string, or
`spec.evidence` for a JSON-compatible string, object, or array. String leaves in
evidence, instructions, and criteria use the restricted template surface.
`noul` accepts optional true/false criteria, `choice` requires 2–255 named
choices, and `score` requires 2–10 ordered levels. OpenRouter requires both
true and false when noul criteria are supplied.

A validated result is stored structurally at
`State.evaluations[effectiveId]`; the same deterministic compact JSON is stored
at `State.outputs[effectiveId]` and returned as the artifact. Failed visits do
not publish partial data. Lifecycle logs expose only bounded API/model,
attempt, usage/cost, request/provider, and error-class fields. Evaluations contribute
no run metrics.

## Composite children

`Sequential`, `Parallel`, `Loop`, and `Router` require at least one child. Sequential, Parallel, and Loop children may be scalar references:

```yaml
children:
  - roles/explorer
```

Use a mapping to configure an occurrence:

```yaml
children:
  - ref: roles/reviewer
    alias: validator
    input: |
      Original goal:
      {{ .Prompt }}

      Candidate result:
      {{ .State.outputs.worker }}
    state:
      phase: validation
    params:
      focus: correctness
```

| Child field | Meaning |
| --- | --- |
| `ref` | Required nonblank resource ID. Any supported kind may be referenced. |
| `alias` | Optional effective ID matching `^[a-z][a-z0-9_]*$`. |
| `canEscalate` | Optional edge authorization for escalation toward the nearest enclosing Loop; defaults to `false`. |
| `input` | Optional template that replaces natural input for this occurrence. |
| `state` | Optional shallow state modifier applied when this child node is visited. |
| `params` | Optional Role parameter bindings; valid only when `ref` resolves directly to a Role. |
| `route` | Router-only nonblank, whitespace-canonical named route. Exactly one of `route` or `default: true` is required. |
| `default` | Router-only no-match edge. At most one child may set `default: true`. |

Router children must use mapping form. Named routes are unique and case-sensitive. The string `default` remains a normal legal named route; fallback is represented only by `default: true`.

Every effective ID must be unique across the complete resolved root tree. An alias changes runtime parameter qualification, state output lookup, and lifecycle identity for that occurrence; it does not change the source resource ID.

Bindings in child `params` must name parameters declared by the referenced Role. They use a restricted template surface without `.Params` or `.Output`, and their rendered values must be nonblank.

### Escalation authorization

`canEscalate` is attached to one parent-to-child occurrence, not to the referenced resource. A scalar child and a mapping that omits the field both mean `canEscalate: false`. Two aliases of the same Role can therefore have different escalation authority.

For a Role to escalate, every child edge from its nearest enclosing Loop to that Role occurrence must set `canEscalate: true`. Authorization is not inherited through an unmarked edge:

```yaml
kind: Loop
spec:
  children:
    - ref: workflows/review-phase
      alias: review_phase
      canEscalate: true
```

If `workflows/review-phase` is a Sequential, its edge to the Role must opt in too:

```yaml
kind: Sequential
spec:
  children:
    - ref: roles/reviewer
      alias: reviewer
      canEscalate: true
```

Either omitted `canEscalate` value makes the resolved `reviewer` occurrence unauthorized. Setting `canEscalate: true` on an edge that is not beneath a Loop is a static graph error.

A nested Loop establishes an independent boundary. Its child authorization starts from the inner Loop, regardless of the edge that connected the inner Loop to its parent. An escalation inside that subtree completes the inner Loop and becomes a normal successful result in the outer composition; it does not complete the outer Loop.

Use `callee agent view <agent-id>` to inspect the effective `canEscalate` value on every resolved node. Use `callee doctor --graph text`, `mermaid`, or `dot` to inspect the authored value on every registry edge. See [Escalation authorization](workflow-semantics.md#escalation-authorization) for prompt injection, rejection, and propagation behavior.

## Sequential

A `Sequential` body renders its local composite input before any child runs:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Explores a task and turns the findings into a plan.
  children:
    - ref: roles/explorer
      alias: explorer
    - ref: roles/architect
      alias: architect
      input: |
        Original task:
        {{ .Prompt }}

        Explorer findings:
        {{ .State.outputs.explorer }}
  output: |
    {{ .State.outputs.architect }}
---
{{ .Input }}
```

The detailed ordering and escalation rules are in [Sequential execution](workflow-semantics.md#sequential-execution). A runnable version is checked in as [`examples/workflows/investigate.md`](../../examples/workflows/investigate.md).

## Parallel

`Parallel` uses the same fields and child occurrence shape as `Sequential`, but every direct child receives input prepared before fan-out and runs concurrently:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Parallel
spec:
  description: Explores a task and classifies its urgency independently.
  children:
    - ref: roles/explorer
      alias: exploration
    - ref: evaluations/typesafe-jev
      alias: triage
  output: |
    Findings: {{ index .State.outputs "exploration" }}
    Triage: {{ index .State.outputs "triage" }}
---
{{ .Input }}
```

Any supported kind except `Human` may appear below Parallel. Descendant Roles always use the one-shot protocol; default or authored `permissions: ask` becomes `allow`, while `deny` remains `deny`. Missing Role parameters and incompatible explicit interactive overrides fail before fan-out.

All branches use the root run's live shared state. Each template sees a coherent snapshot, related state entries publish atomically, and a later read may observe a sibling's completed commit. Same-key writes follow actual commit order. Child commits are retained if another branch fails; the failed Parallel does not publish its own output. Natural aggregate JSON and failure diagnostics use authored child order. See [Parallel execution](workflow-semantics.md#parallel-execution) and the runnable [`parallel-review`](../../examples/workflows/parallel-review.md) and [`parallel-then-plan`](../../examples/workflows/parallel-then-plan.md) examples.

## Loop

A `Loop` adds a positive `maxIterations` bound and an exhaustion policy:

```yaml
kind: Loop
spec:
  maxIterations: 5
  onExhausted: fail
```

`onExhausted` defaults to `fail`; set it to `complete` only when the last natural child artifact is a valid successful result. A Loop consumes escalation from eligible descendants according to the rules in [Loop execution](workflow-semantics.md#loop-execution). The complete runnable example is [`examples/workflows/goalkeeper.md`](../../examples/workflows/goalkeeper.md).

## Router

A `Router` requires an independent route template and mapping-form children:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes one classified task.
  route: '{{ .Input }}'
  children:
    - ref: roles/implementer
      alias: routed_implementer
      route: implement
    - ref: roles/reviewer
      alias: routed_reviewer
      route: review
    - ref: roles/explorer
      alias: routed_generalist
      default: true
---
{{ .Prompt }}
```

`spec.route` renders the route key, while the Markdown body renders the selected child's natural payload. Surrounding route whitespace is trimmed before exact, case-sensitive matching. Default handles only blank or unknown keys. Without default, no-match fails before child activity. Route-template errors bypass default, and a selected child's failure is never retried through default. Router has no provider-owned routing, fan-out, multiple defaults, or arbitrary graph fields. See [Router execution](workflow-semantics.md#router-execution) and the runnable [`routed-task`](../../examples/workflows/routed-task.md) composition.

## Template surfaces

Templates use Go `text/template` with `missingkey=zero`. The common data root contains:

| Value | Available in | Meaning |
| --- | --- | --- |
| `.Prompt` | All authored templates | Immutable original root prompt. |
| `.Input` | All authored templates | Input for the current node or surface. |
| `.State` | All authored templates | Shared root-run state snapshot. |
| `.Params` | Role body, composite body, child input, composite output | Current Role parameters in a Role body; otherwise an empty map. |
| `.Output` | Composite `spec.output` only | Natural artifact produced by the composite's children. |

State string leaves and child parameter bindings use the restricted surface: `.Params` and `.Output` are unavailable. `.Output` is rejected outside composite `spec.output`.

Callee exposes a deterministic positive allowlist from Sprig v3.3.0 plus explicit-input UTC `dateParse` and `dateFormat` helpers. Environment, filesystem, network, current-clock, random, UUID, cryptographic generation, and mutating dictionary helpers are unavailable. The exact allowlist is maintained in [`internal/agent/template.go`](../../internal/agent/template.go).

## State modifiers

`spec.state` and child `state` accept JSON-compatible strings, booleans, finite numbers, arrays, and string-keyed objects. Null is not supported. The top-level `outputs` and `scripts` keys are reserved.

State application is shallow. Resource state is combined with edge state, with edge values replacing resource values at the same top-level key. String leaves are templates. Callee renders every value against the same immutable pre-node snapshot and commits the whole modifier only if every render succeeds.

## Validation layers

Use the narrowest check that answers the question:

```bash
# Decode and validate one file only.
callee agent validate .callee/roles/reviewer.md

# Load both discovery roots and resolve one complete tree.
callee agent view workflows/investigate

# Validate the complete registry and check every Role runtime.
callee doctor
```

`agent validate` does not resolve child references. `agent view` and `doctor` load the complete registry, so unrelated invalid or duplicate discovered resources also prevent them from succeeding.
