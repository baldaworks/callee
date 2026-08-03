# Author Callee workflows

Read this reference before creating any `Sequential`, `Loop`, `Router`, or nested workflow.

## Contents

- [Place and represent files](#place-and-represent-files)
- [Compose the resolved tree](#compose-the-resolved-tree)
- [Author a Sequential workflow](#author-a-sequential-workflow)
- [Author a Router workflow](#author-a-router-workflow)
- [Author a Loop workflow](#author-a-loop-workflow)
- [Finish the workflow](#finish-the-workflow)

## Place and represent files

Write one agent resource per file below `.callee/`. Use `.callee/roles/` and
`.callee/workflows/` as optional organizational namespaces; directories do not
select the resource kind. The agent ID is the path relative to `.callee/`
without the final `.md`, `.yaml`, or `.yml` extension.

Prefer Markdown. Put `apiVersion`, `kind`, and `spec` in YAML frontmatter,
then put the workflow input template in the Markdown body. The loader binds that
physical body to `spec.body`, so do not also write `spec.body` in Markdown
frontmatter.

Use YAML only when the user explicitly asks for it. A YAML or YML file is the
complete resource object and must include `spec.body` explicitly.

## Compose the resolved tree

Use only `Role`, `Script`, `Human`, `Sequential`, `Loop`, and `Router`. A workflow child may reference
any supported kind, so workflows may nest other workflows. Do not author
`Parallel`, fan-out, Join, or arbitrary graph edges.

Each child accepts `ref` and optional `alias`, `canEscalate`, `input`,
`state`, and `params` fields.

- Resolve every `ref` before writing the parent.
- Add an `alias` when a resource is reused or when templates need a stable,
  readable output key. An alias replaces the generated effective ID, must match
  `^[a-z][a-z0-9_]*$`, and must be unique across the entire resolved tree.
- Use `input` to render the value passed into that child. Without it, the first
  child receives the workflow input and each later child receives the previous
  child artifact.
- Use `state` for shallow top-level state replacements applied before the node
  runs. String leaves are Go templates. Never author the reserved `outputs`
  key or the reserved `scripts` key.
- Use child `params` only when that child resolves directly to a `Role` and
  only for parameters declared by that Role. Bindings are Go templates over
  `.Prompt`, `.Input`, and `.State`; they must render nonblank. Leave an
  unbound Role parameter for the operator to supply at runtime.
- Permission policy belongs to each referenced Role's `spec.permissions`, not
  to the child edge or composite. Inspect the resolved authored and effective
  policy before running; omission defaults to `ask`.
- A Router child must use mapping form and add exactly one unique named
  `route` or the sole `default: true` edge. Do not use `default` as a
  reserved route string; `route: default` remains an ordinary named route.

Every successful nonblank node artifact is stored at
`.State.outputs[effectiveID]`. Repeated visits overwrite that key with the
last successful artifact. Completed `Script` visits also record structured
validator results at `.State.scripts[effectiveID]`. A `Human` additionally
stores its response at the top-level key selected by `spec.responseKey`. Use
`index` for robust lookup:

```gotemplate
{{ index .State.outputs "validator" }}
```

The template root exposes `.Prompt` for the immutable root user prompt,
`.Input` for the current node input, and `.State` for shared root-run state.
`.Params` is meaningful only while rendering a Role body. `.Output` is
available only in a composite `spec.output` template. Template surfaces use Go
`text/template` plus Callee's deterministic safe Sprig allowlist.

## Author a Sequential workflow

A `Sequential` runs every child once in order. Prefer implicit artifact piping
when each stage consumes only the previous result. Add explicit child inputs
when a stage also needs the original workflow goal or a named earlier artifact.

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Plans, implements, and validates a change.
  children:
    - ref: roles/planner
      alias: planner
    - ref: roles/implementer
      alias: implementer
      input: |
        Goal:
        {{ .Input }}

        Plan:
        {{ index .State.outputs "planner" }}
    - ref: roles/validator
      alias: validator
      input: |
        Goal:
        {{ .Input }}

        Implementation:
        {{ index .State.outputs "implementer" }}
  output: |
    {{ index .State.outputs "validator" }}
---
{{ .Input }}
```

Keep a child `params` entry only if the referenced Role declares that exact
parameter. For example, a child Role that declares `spec.params.language` may
be bound with:

```yaml
    - ref: roles/language-worker
      params:
        language: Go
```

A nested `Sequential` is itself an ordinary child and receives its
parent-rendered input in the same way.

## Author a Router workflow

A `Router` deterministically selects exactly one child. Render the route key
with `spec.route` and render the selected child's payload independently with
the Markdown body. Use an optional `default: true` child only for blank or
unknown keys.

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Router
spec:
  description: Routes one classified task to exactly one handler.
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

Trimmed route keys match named routes case-sensitively. Without default, a
blank or unknown key fails before child activity. A route-template error never
selects default. Once a child is selected, its failure does not retry or fail
over to default. Without child `input`, the selected child receives the
rendered Router body; an explicit child `input` overrides that payload.
Unselected children do not run, mutate state, prompt, or promote output.

Router output and `canEscalate` use the same composite and nearest-Loop rules
as other workflows. Do not use a provider to choose an undeclared child, add
multiple defaults, select multiple branches, or expose ADK graph primitives.

## Author a Loop workflow

A `Loop` runs its ordered children repeatedly. Set `maxIterations` to an
integer of at least one. `onExhausted` defaults to `fail`; set it to
`complete` only when the latest artifact is a valid successful result.

Preserve the original goal explicitly in worker and validator inputs. Feed the
last validator artifact back to the worker so later iterations can act on
feedback. Tell the validator to escalate only when the goal is satisfied and to
return actionable feedback normally otherwise. Reserve `fail` for an
unrecoverable condition because it aborts the entire workflow.

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Repeats a worker and validator until the validator escalates.
  children:
    - ref: roles/implementer
      alias: worker
      input: |
        Goal:
        {{ .Input }}

        {{ with index .State.outputs "validator" }}
        Previous validation:
        {{ . }}
        {{ end }}
    - ref: roles/reviewer
      alias: validator
      canEscalate: true
      input: |
        Goal:
        {{ .Input }}

        Worker result:
        {{ index .State.outputs "worker" }}

        Validate the result. If it satisfies the goal, return your validation
        and escalate to finish the loop. Otherwise return actionable feedback
        normally so the next iteration can improve it.
  maxIterations: 5
  onExhausted: fail
  output: |
    GoalKeeper finished with result:
    {{ index .State.outputs "validator" }}
---
{{ .Input }}
```

Callee injects the concrete control protocol only into Roles whose resolved
occurrence is authorized to escalate. Set `canEscalate: true` on every edge
from the nearest `Loop` to that Role; the default is `false`. Describe the
semantic choice in the authored prompt; do not invent a custom stop token.
Escalation from an authorized direct Loop child completes that Loop immediately
and skips its later children. A nested `Sequential` still runs its remaining
children under sticky escalation before propagating it. The nearest Loop
consumes the escalation.

A nested `Loop` is an ordinary child: its Markdown body renders that node's
input, its children run with the same shared state object, and its final artifact
becomes the parent node's input or named output.

## Finish the workflow

Create and validate any new child resources first. Then return to the main skill
and run `callee agent validate` on every written file followed by
`callee agent view` on the root ID. The resolved view must have no missing
children, duplicate IDs, duplicate aliases, or unbound parameters that were
intended to be fixed by the workflow.
