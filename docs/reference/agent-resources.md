# Agent resource format

Use this reference when authoring or reviewing a Callee resource. The checked-in [Draft 2020-12 JSON Schema](../../internal/agent/schema.json) defines the structural contract; Callee also enforces semantic, template, state, and graph constraints in code. Use `callee agent schema <Role|Script|Human|Sequential|Loop|Router>` to print a standalone schema document for one kind.

## Discovery and IDs

Callee recursively discovers regular files under both of these roots:

- `$XDG_CONFIG_HOME/callee`, or `$HOME/.config/callee` when `XDG_CONFIG_HOME` is unset;
- `.callee` in the current working directory.

Pass `callee --agent-root <dir> ...` to switch into exclusive discovery mode.
When set, Callee ignores both default roots and discovers resources only under
`<dir>`. The resource ID remains relative to that directory.

Only lowercase `.md`, `.yaml`, and `.yml` extensions are supported. Symlinked files and unsupported extensions are skipped. The resource ID is the slash-separated relative path with its final supported extension removed. For example, `.callee/workflows/review.yml` has ID `workflows/review`.

IDs must be unique across both roots and all supported formats. Project resources do not shadow user resources. A duplicate such as `roles/reviewer.md` and `roles/reviewer.yaml` makes registry loading fail.

Directories are namespaces, not kind selectors. A `Role` can technically reside outside `roles/`, but `roles/` and `workflows/` keep catalogs understandable.

## Versioned envelope

Every resource has exactly three top-level fields:

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec: {}
```

The only accepted API version is `callee.metalagman.dev/v1alpha1`. Supported kinds are `Role`, `Script`, `Human`, `Sequential`, `Loop`, and `Router`. Unknown fields are rejected at every schema-defined object boundary.

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

All kinds require a nonblank `description` and nonblank `body`. Every kind may declare `state`, whose values are described under [State modifiers](#state-modifiers).

The supported fields differ by kind:

| Field | Role | Script | Human | Sequential | Loop | Router |
| --- | --- | --- | --- | --- | --- | --- |
| `description` | Required | Required | Required | Required | Required | Required |
| `body` | Required | Required | Required | Required | Required | Required |
| `state` | Optional | Optional | Optional | Optional | Optional | Optional |
| `provider` | Required | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `permissions` | Optional | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `interactive` | Optional | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `params` | Optional | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed |
| `responseKey` | Not allowed | Not allowed | Required, nonblank | Not allowed | Not allowed | Not allowed |
| `shell` | Not allowed | Optional: `sh` or `bash` | Not allowed | Not allowed | Not allowed | Not allowed |
| `cwd` | Not allowed | Optional | Not allowed | Not allowed | Not allowed | Not allowed |
| `env` | Not allowed | Optional string map | Not allowed | Not allowed | Not allowed | Not allowed |
| `timeout` | Not allowed | Optional positive Go duration | Not allowed | Not allowed | Not allowed | Not allowed |
| `onNonZero` | Not allowed | Optional: `fail` or `continue` | Not allowed | Not allowed | Not allowed | Not allowed |
| `children` | Not allowed | Not allowed | Not allowed | Required, nonempty | Required, nonempty | Required, nonempty mappings |
| `route` | Not allowed | Not allowed | Not allowed | Not allowed | Not allowed | Required, nonblank |
| `output` | Not allowed | Not allowed | Not allowed | Optional | Optional | Optional |
| `maxIterations` | Not allowed | Not allowed | Not allowed | Not allowed | Required, integer at least 1 | Not allowed |
| `onExhausted` | Not allowed | Not allowed | Not allowed | Not allowed | Optional: `fail` or `complete` | Not allowed |

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

`spec.permissions.mode` accepts exactly `ask`, `allow`, or `deny` and defaults to `ask` when `permissions` is omitted. It is a Role-only runtime policy and is independent of backend-specific `spec.provider.mode`. See [ACP permission requests](../guides/acp-permissions.md) for option selection and failure semantics.

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

`spec.responseKey` names the top-level shared-state entry that receives the collected operator response. The key must be nonblank and cannot be the reserved `outputs` or `scripts` keys.

At runtime, Callee renders `body`, displays the rendered text on the controlling terminal, prompts once for a nonblank response, stores that string at `State[responseKey]`, and also promotes it to `State.outputs[effectiveId]`.

## Composite children

`Sequential`, `Loop`, and `Router` require at least one child. Sequential and Loop children may be scalar references:

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
