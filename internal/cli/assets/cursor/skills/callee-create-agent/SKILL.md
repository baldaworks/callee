---
name: callee-create-agent
description: Create project-defined Callee Role, Script, Human, Sequential, Loop, and Router agents in Markdown or YAML. Use when the user asks to generate, scaffold, compose, or author a Callee agent or deterministic workflow.
---

# Create a Callee agent

Use `callee` when available. Otherwise use the pinned fallback `npx --yes @baldaworks/callee@0.20.1` for every command in the task.

## Choose the kind and ID

Inspect the existing catalog before writing:

```bash
callee agent list --json
```

Choose exactly one supported kind:

- `Role`: one provider-backed leaf agent.
- `Script`: one deterministic local validator leaf.
- `Human`: one operator-backed interactive leaf.
- `Sequential`: ordered child agents that each run once.
- `Loop`: ordered child agents repeated until an authorized Role escalates or the iteration limit is exhausted.
- `Router`: exactly one named or default child selected by a deterministic route template.

Do not create `Parallel`. Write below `.callee/`. Markdown is the default; use a complete `.yaml` or `.yml` object only when the user explicitly requests YAML. Directories such as `roles/` and `workflows/` are optional ID namespaces, not kind selectors. The agent ID is the relative path without its final supported extension.

## Choose the authored interaction profile

Choose the profile before authoring any Role or Human. Keep whole-run mode,
Role protocol, and ACP permissions distinct:

| Intent | Role fields | Default execution path |
| --- | --- | --- |
| Supervised one-shot (default) | Omit `spec.interactive`; use `permissions.mode: ask` | Interactive whole run for permission prompts; one-shot Role |
| Conversational | Set `spec.interactive: true`; choose permissions independently | Interactive whole run and REPL Role |
| Unattended restricted | Omit `spec.interactive`; use `permissions.mode: deny` | Non-interactive when the tree has no Human and all inputs are supplied |
| Unattended approved | Omit `spec.interactive`; use `permissions.mode: allow` only on explicit request | Non-interactive when the tree has no Human and all inputs are supplied |

Preserve the supervised one-shot profile when the user does not specify a
mode. Do not turn `ask` into `allow` or `deny` merely to avoid a controlling
TTY. Use `deny` for unattended execution without approval to grant
permissions, and use `allow` only when the user explicitly authorizes automatic
approval. Do not add a Human anywhere in an unattended tree, including beneath
an unselected Router branch.

## Author a Role

When an embedded PromptKit template clearly fits the requested capability, inspect it and use the generator:

```bash
callee promptkit search "<intent>" --type template
callee promptkit show "<template>" --json
callee promptkit role create "<agent-id>" \
  --template "<template>" \
  --description "<capability description>" \
  --provider "<provider>" \
  --prompt-param "<runtime-input-parameter>"
```

Inspect `metadata.mode` in the selected template. When it is `interactive`, let the generator set `spec.interactive: true`; keep its questions and confirmation gates for `callee agent run` and do not execute those phases while authoring the reusable Role. Use `--interactive` only to force REPL behavior for a template that is not marked interactive.

Bind only values that are fixed when the agent is authored. Leave intended runtime values unbound so they become `spec.params`. Use `--output` when the requested ID must not live below the generator's default `.callee/roles/` namespace.

If PromptKit does not fit, author the Role directly:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: <capability description>
  provider:
    type: <codex|claude|opencode|copilot|grok|cursor|generic_acp>
  permissions:
    mode: ask
  params:
    focus: What the agent should focus on
---
Task:
{{ .Input }}

Focus:
{{ .Params.focus }}
```

Keep provider configuration under `spec.provider`. Configure ACP permission handling separately with Role-only `spec.permissions.mode`: `ask` makes the whole agent interactive and uses the controlling TTY, `allow` automatically selects a compatible allow option, and `deny` automatically selects a compatible reject option. Omission defaults to `ask`. Permission policy does not select the Role protocol: set `spec.interactive: true` only when the Role must continue in the same provider session across operator turns. Keep exactly one unconditional bare `{{ .Input }}` insertion in a generated Role body. Use Go `text/template` syntax on every template surface.

## Author a Script

Author a `Script` directly when the requested node is a deterministic validator such as tests, lint, formatting checks, or other local gates:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Script
spec:
  description: Runs a validator.
  shell: sh
  onNonZero: continue
---
go test ./...
```

Use `shell: sh` unless the request clearly needs Bash-specific syntax. Set `onNonZero: fail` for hard gates and `onNonZero: continue` only when a later node or later Loop iteration must inspect the validator result in `.State.scripts`. Use `cwd`, `env`, and `timeout` only when the request explicitly needs them.

## Author a Human

Author a `Human` directly when the workflow must collect one explicit operator response:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Human
spec:
  description: Requests operator approval.
  responseKey: approval
---
Review and approve this request:
{{ .Input }}
```

Use a nonblank `responseKey` other than the reserved `outputs` and `scripts` keys. A Human has no provider, permissions, parameters, or REPL setting. A Human anywhere in the resolved tree makes spec-driven mode interactive and causes explicit non-interactive execution to fail during preflight, even beneath an unselected Router branch. At runtime it displays the rendered body on the controlling TTY, waits for one nonblank response, stores that response at the selected top-level state key and `.State.outputs[effectiveId]`, and returns it as the node artifact.

## Author a workflow

For every `Sequential`, `Loop`, `Router`, or nested-composite request, read [references/workflows.md](references/workflows.md) completely before writing. Follow its placement, representation, child-wiring, state, parameter, routing, loop-control, and output rules.

Resolve every referenced child with `callee agent view "<child-id>" --json`. If the request also needs new child Roles or workflows, create and validate those first. Then author the root workflow from the reference and validate the complete resolved tree.

## Validate the result

Validate each written file, then resolve the complete tree and required runtime parameters:

```bash
callee agent validate "<written-agent-path>"
callee agent view "<agent-id>" --json
```

Use the actual generated `.md`, `.yaml`, or `.yml` path for validation. Confirm the top-level `specDrivenInteractive` and effective `interactive` values in `agent view --json`. Treat the former as the authored baseline and the latter as the agent's default execution path. For every Role, confirm `authoredInteractive`, effective `interactive`, `authoredPermissions`, and effective `permissions`; these are independent axes. Confirm that supervised and conversational profiles are effectively interactive. Confirm that unattended profiles are effectively non-interactive, contain no Human, and expose every required parameter for explicit runtime input. Confirm that every Human has the intended `responseKey` without Role-only fields. Fix every schema, template, missing-child, duplicate-ID, and duplicate-alias error before reporting success. Do not add Gemini, legacy flat provider fields, a server transport, or thread persistence.
