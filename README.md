# Callee

[![Test](https://github.com/baldaworks/callee/actions/workflows/test.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/test.yml)
[![Lint](https://github.com/baldaworks/callee/actions/workflows/lint.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/lint.yml)
[![Security](https://github.com/baldaworks/callee/actions/workflows/security.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/security.yml)
[![Latest release](https://img.shields.io/github/v/release/baldaworks/callee)](https://github.com/baldaworks/callee/releases/latest)
[![npm version](https://img.shields.io/npm/v/%40baldaworks%2Fcallee)](https://www.npmjs.com/package/@baldaworks/callee)
[![License: MIT](https://img.shields.io/github/license/baldaworks/callee)](LICENSE)

## Markdown-defined agents and deterministic workflows

Callee lets a repository define provider-backed `Role`, `Sequential`, and `Loop` agents as versioned Markdown or YAML. Markdown is the base authoring and generation format; YAML represents the same complete schema object with `spec.body` inline. Every kind has the same node boundary: it receives input, may update one root-run state object, and returns one artifact or a structured orchestration outcome.

Callee remains CLI-only. It uses [Norma Runtime](https://github.com/normahq/runtime) for ACP provider processes and Go ADK-aligned escalation semantics. It has no server, durable thread store, or handle binding.

## Install and set up

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
# or
npx --yes @baldaworks/callee@latest setup codex
```

| Host | Setup |
| --- | --- |
| Codex | `callee setup codex` |
| Claude Code | `callee setup claude` |
| Grok Build | `callee setup grok` |
| Copilot CLI | `callee setup copilot` |
| OpenCode | `callee setup opencode` |

Setup installs a thin host integration and six editable starter agents:

| Workflow | Graph | Purpose |
| --- | --- | --- |
| `workflows/investigate` | `roles/explorer -> roles/architect` | Read-only codebase investigation and an implementation-ready plan |
| `workflows/goalkeeper` | `roles/implementer -> roles/reviewer -> repeat` | Iterative implementation until the reviewer accepts the result |

Generated Roles set only the selected host's `provider.type`; provider defaults choose the model and mode. Existing starter files are left unchanged while missing files are added. Use `--force` to replace the complete starter pack. Provider CLIs and credentials remain external.

## Quick start

Inspect the installed agents and the safe, read-only workflow:

```bash
callee agent list
callee agent view workflows/investigate
callee agent run workflows/investigate --message "Explain this project's architecture and main entry points"
```

Validate one agent file without loading its referenced children:

```bash
callee agent validate .callee/roles/reviewer.md
callee agent validate .callee/roles/reviewer.yaml
```

When you are ready to make changes, run GoalKeeper through the same entrypoint:

```bash
callee agent run roles/reviewer --message "Review the current changes"
callee agent run workflows/goalkeeper --message "Implement the requested feature"
```

`agent run` always requires a real controlling TTY. Initial input, missing parameters, permissions, and REPL turns use the TTY. Info lifecycle events for every `Role`, `Sequential`, and `Loop` visit use stderr, so nonempty stderr alone does not indicate failure; use the command exit status. Lifecycle durations use standard Go strings with units, such as `43.453998585s`. The successful root artifact is written once to stdout only after all provider cleanup succeeds.

Qualified runtime parameters use the resolved effective node ID:

```bash
callee agent run workflows/pipeline \
  --message "Build it" \
  --param worker.language=Go \
  --param-file planner.context=./context.md
```

## Agent format

Agents are discovered recursively below the user Callee config root and project `.callee`. Lowercase `.md`, `.yaml`, and `.yml` extensions are supported. Directories such as `roles/` and `workflows/` are optional ID namespaces; `kind` alone determines behavior. The final extension is removed from the ID, so `.callee/roles/reviewer.md` and `.callee/roles/reviewer.yaml` both have ID `roles/reviewer` and conflict if both exist. IDs must be unique across roots and formats.

All agents use the same Kubernetes-style `apiVersion`/`kind`/`spec` envelope.

## Agent kinds

### Role

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews changes.
  provider:
    type: codex
  params:
    focus: Review focus
---
You are a reviewer.

Task:
{{ .Input }}

Focus:
{{ .Params.focus }}
```

A Role body must contain exactly one unconditional bare `{{ .Prompt }}` or `{{ .Input }}` insertion. `.Prompt` is the immutable original root prompt; `.Input` is this node occurrence's rendered input.

Supported provider types are `codex`, `claude`, `opencode`, `copilot`, `grok`, and `generic_acp`. `generic_acp` requires `spec.provider.cmd`. Gemini is not supported.

See the runnable [`reviewer`](examples/roles/reviewer.md) example.

### Sequential

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Plans, implements, and validates.
  children:
    - ref: roles/planner
      alias: planner
    - ref: roles/implementer
      alias: implementer
      input: |
        Plan:
        {{ .State.outputs.planner }}
    - ref: roles/reviewer
      alias: validator
  output: |
    {{ .State.outputs.validator }}
---
{{ .Input }}
```

`Sequential` runs children in source order. Without an explicit child `input`, the first child receives the composite input and later children receive their predecessor's output. Escalation is sticky across the remaining sequential children and propagates upward after they finish. See the runnable [`investigate`](examples/workflows/investigate.md) example.

### Loop

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
      input: |
        Goal:
        {{ .Input }}

        Worker result:
        {{ .State.outputs.worker }}

        Validate the result. If it satisfies the goal, return your validation
        and escalate to finish the loop. Otherwise return actionable feedback
        normally so the next iteration can improve it.
  maxIterations: 5
  onExhausted: fail
  output: |
    GoalKeeper finished with result:
    {{ .State.outputs.validator }}
---
{{ .Input }}
```

A `Loop` repeats its ordered children up to `maxIterations`. It consumes escalation from a direct child and completes. `onExhausted` is `fail` by default or may be `complete`. `Parallel` is not part of v1alpha1. See the runnable [`goalkeeper`](examples/workflows/goalkeeper.md) example.

### Children and composition

Children may reference any supported kind, including another `Loop`. A child mapping supports `ref`, optional globally unique `alias`, `input`, shallow `state`, and Role-only `params`. Aliases match `^[a-z][a-z0-9_]*$` and replace the occurrence's effective ID.

## YAML representation and JSON Schema

Markdown is the canonical authoring format: its physical body becomes `spec.body` and `spec.body` must not also appear in frontmatter. A `.yaml` or `.yml` file represents the same complete resource object and must author `spec.body` inline.

Callee validates both representations against the checked-in [Draft 2020-12 JSON Schema](internal/agent/schema.json), whose exact bytes are embedded in the binary. For editor integration, use the raw schema from the repository:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/baldaworks/callee/main/internal/agent/schema.json
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews changes.
  provider:
    type: codex
  params:
    focus: Review focus
  body: |
    You are a reviewer.

    Task:
    {{ .Input }}

    Focus:
    {{ .Params.focus }}
```

This YAML object is canonically identical to the Markdown Role above. The same representation rule applies to `Sequential` and `Loop`.

`agent validate` performs schema, semantic, state, and template validation for exactly one file. It intentionally does not resolve workflow child references; use `agent view <id>` or `doctor` to validate the discovered graph.

## Templates and state

All workflow-aware surfaces use Go `text/template` with `missingkey=zero`, a pinned deterministic Sprig v3.3.0 positive allowlist, and strict explicit-input UTC `dateParse`/`dateFormat` helpers. Environment, filesystem, network, clock, random, UUID, crypto-generation, and mutating dictionary helpers are unavailable.

The common template root exposes:

- `.Prompt`: immutable original user prompt.
- `.Input`: current node input.
- `.State`: one JSON-compatible root-run state object.
- `.Params`: current Role parameter map.
- `.Output`: natural child-derived output, only while rendering composite `spec.output`.

Every successful nonblank node artifact is promoted to `.State.outputs[effectiveId]`. The engine owns `outputs`; authored state cannot replace it. Repeated loop visits use last-successful-write-wins.

State modifiers are shallow top-level replacements. String leaves are templates evaluated against one pre-node snapshot, and the complete modifier commits atomically.

## Control and REPL

Callee injects a versioned control protocol into workflow-run Roles. REPL responses end with exactly one final record:

```text
callee.control.v1.await
callee.control.v1.return
callee.control.v1.escalate
callee.control.v1.fail
```

Artifact text, when present, is separated from the record by exactly one empty line. `await` retains the same visit session for another operator turn. A prepared REPL visit emits one `entering repl` / `exiting repl` lifecycle pair; any `await` turns remain inside it. `return` completes normally. `escalate` returns control to the nearest Loop. `fail` aborts the root. Hosts must not send `/done`, `quit`, or `exit` to choose completion.

## Doctor and graphs

```bash
callee doctor
callee doctor --timeout 90s
callee doctor --graph text
callee doctor --graph mermaid
callee doctor --graph dot
```

Plain doctor completes static schema/template/graph validation before provider startup, groups Roles by provider process identity, checks ACP initialization and disposable session creation, and sends no model prompt. Graph modes are static-only and never start providers.

## PromptKit

Callee embeds the pinned PromptKit catalog through [PromptKitty](https://github.com/baldaworks/promptkitty):

```bash
callee promptkit list
callee promptkit search "code review" --type template
callee promptkit show review-code
callee promptkit role create go-reviewer \
  --template review-code \
  --description "Reviews Go code" \
  --type codex \
  --prompt-param code \
  --bind language=Go
```

Generated Roles use the v1alpha1 envelope and Go templates. Unbound PromptKit parameters become `spec.params`; literal template examples in assembled PromptKit text are escaped safely.

## Distribution and limits

Callee publishes CGO-disabled native executables for macOS and Linux on AMD64/ARM64 and Windows AMD64, plus an npm launcher distribution. Each root run is ephemeral: no Callee thread store, persisted workflow state, server transport, or cross-process continuation is created.

## License

Callee is released under the [MIT License](LICENSE).
