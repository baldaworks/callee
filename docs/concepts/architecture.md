# Concepts and architecture

Callee turns repository-owned agent resources into a statically validated execution tree, then runs that tree through Agent Client Protocol (ACP) providers. The design separates host-facing authoring skills from the runtime provider that supplies each model session.

## Core concepts

| Concept | Meaning |
| --- | --- |
| Resource | One versioned Markdown or YAML definition with a `Role`, `Script`, `Human`, `Sequential`, `Loop`, or `Router` kind. |
| Resource ID | The path below a discovery root with the final supported extension removed, such as `roles/reviewer`. |
| Resolved node | One occurrence of a resource in a selected root tree. An edge alias, when present, becomes its effective ID. |
| Role | A provider-backed leaf that renders a prompt and performs one or more turns in one fresh ACP session. |
| Script | A deterministic local validator leaf that runs a shell step and records structured results in workflow state. |
| Human | An operator-backed leaf that displays a rendered prompt and records one nonblank response in workflow state. |
| Sequential | A composite that visits children once in source order. |
| Loop | A bounded composite that repeatedly visits ordered children until an authorized descendant escalates or the Loop exhausts. |
| Router | A deterministic composite that dispatches exactly one named or default child from an authored route template. |
| Root-run state | One ephemeral JSON-compatible object shared by every node visit in a run. |
| Provider process | A reusable ACP transport process identified by provider type and resolved command. |
| Role visit | One execution of a resolved Role node, with a fresh provider session even when its provider process is reused. |

## Execution pipeline

```text
Markdown/YAML files
        |
        v
decode + schema/semantic/template validation
        |
        v
registry + static graph resolution
        |
        v
resolved root tree + required parameter keys
        |
        v
workflow runner + one shared ephemeral state object
        |
        v
Norma Runtime -> ACP provider processes -> fresh Role visit sessions
        |
        v
one root artifact on stdout after provider cleanup
```

Discovery loads the user and project roots together. Registry construction rejects invalid resources, unresolved references, cycles, duplicate resource IDs, and duplicate effective IDs in a resolved tree before execution begins. See [Agent resource format](../reference/agent-resources.md) for discovery and validation rules.

Escalation authority belongs to child edges, not resource definitions. A Role may escalate to its nearest enclosing Loop only when every edge from that Loop to the Role occurrence sets `canEscalate: true`; omitted values are `false`. Entering a nested Loop starts a new authorization boundary, so its descendants do not inherit authority from the outer Loop. The resolved effective capability is visible in `agent view`, while doctor graphs show the authored value on every edge. See [Escalation authorization](../reference/workflow-semantics.md#escalation-authorization) for the runtime consequences.

At runtime, the runner creates state with engine-owned `outputs` and `scripts` maps. Each node may render a state modifier against a pre-node snapshot. A Role renders its body and calls its provider session. A Script renders and executes a local validator step, then records its structured result under `State.scripts`. A Human displays its rendered body on the controlling terminal and records the response under its configured state key. Sequential and Loop composites activate children serially. Router uses the internal ADK 2 graph scheduler to activate exactly one `StringRoute` or `Default` edge; its route key and child payload are rendered separately. Every composite may render `spec.output` to transform the natural child result. See [Workflow semantics](../reference/workflow-semantics.md) for the precise data flow.

## Process and session ownership

A root run reuses a provider process when Roles normalize to the same public provider type and command. Session configuration does not change that process identity: `model`, `mode`, and `reasoning` are applied when creating the Role visit session.

Every Role visit receives a fresh provider session. Repeated visits to the same Role in a Loop therefore do not continue the previous provider conversation. A REPL Role is the exception only within that single visit: `await` retains its session for the next operator turn. Provider processes remain live until the root finishes and are closed in reverse start order. A cleanup error suppresses the otherwise successful artifact.

Provider process startup, session creation and preparation, and every model turn each receive the Role's effective provider timeout. Operator waits use the CLI's separate REPL timeout, and active provider-turn timeout accounting pauses while an ACP permission request waits for the operator.

## State and artifact model

The runner owns one state object for the entire root run:

```yaml
outputs: {}
scripts: {}
```

Authored `spec.state` and child-edge `state` values add or replace top-level keys; they cannot author `outputs` or `scripts`. Edge state wins over resource state for the same key. All string leaves are templates rendered against one immutable pre-node snapshot, and the complete modifier commits atomically.

Each successful, nonblank node artifact is promoted to `State.outputs[effectiveId]`. Completed Script visits also record `status`, `exitCode`, `stdout`, `stderr`, and `timedOut` at `State.scripts[effectiveId]`. Human visits additionally store their response at the top-level key selected by `spec.responseKey`. Repeated visits use last-successful-write-wins. State is neither persisted after the command exits nor shared across root runs.

## Host integration versus runtime provider

A coding-host integration installs instructions that let Codex, Claude Code, Grok Build, Copilot CLI, OpenCode, or Cursor discover, create, and run project-defined Callee resources. It does not provide the ACP runtime used by a `Role`.

The Role's `spec.provider.type` independently selects an ACP backend. For example, a project may invoke Callee through the Codex plugin while a Role uses the `claude` provider. Each provider CLI, adapter, authentication method, and required credential remains external to host setup. See [Coding-host integrations](../guides/coding-host-integrations.md) and [ACP provider configuration](../guides/acp-providers.md).

## Package responsibilities

| Path | Responsibility |
| --- | --- |
| [`internal/agent`](../../internal/agent) | Resource types, Markdown/YAML codecs, JSON Schema validation, state constraints, and template functions. |
| [`internal/registry`](../../internal/registry) | Discovery, duplicate detection, graph resolution, aliases, cycles, and required parameters. |
| [`internal/workflow`](../../internal/workflow) | Node execution, state, artifact promotion, composition, control records, REPL behavior, timeouts, and cleanup. |
| [`internal/runtime`](../../internal/runtime) | Callee-to-Norma provider normalization, ACP process reuse, and Role visit sessions. |
| [`internal/doctor`](../../internal/doctor) | Static graph rendering and provider/session readiness checks. |
| [`internal/cli`](../../internal/cli) | Public command surface, TTY interaction, permissions, setup, and PromptKit integration. |
| [`plugins/callee`](../../plugins/callee) | Marketplace plugin manifests and the create/run skills distributed to coding hosts. |

## Deliberate limits

Callee does not provide a server, a thread or state store, cross-process continuation, provider handle binding, `Parallel` or fan-out workflows, arbitrary graph edges, or a Gemini provider. Router selection is deterministic and single-branch. ACP process logic is delegated to Norma Runtime rather than reimplemented in the project. These limits are current product decisions, not undocumented extension points.
