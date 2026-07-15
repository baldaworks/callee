---
name: callee-create-role
description: Create a project-defined Callee Markdown role from an embedded PromptKit template. Use when the user asks to generate, scaffold, or author a new Callee role.
---

# Create a Callee role

## Invocation

Use `$callee-create-role <role request>`. This skill authors role files; use
`$callee-run-role <task>` to run existing roles.

## Gather the role contract

Identify a PromptKit template that fits the request. Search or inspect the
PromptKit catalog when the user did not name a template:

```bash
npx --yes @baldaworks/callee@0.8.0 promptkit search "<capability>" --type template
npx --yes @baldaworks/callee@0.8.0 promptkit show "<template>"
```

Collect the Callee role ID, a concise description, template parameters, and
the runtime type. If the runtime type is not explicitly supplied or cannot be
determined unambiguously, ask the user to choose it.
Never default or infer a type.
Keep Callee metadata flat. Do not add provider configuration or Gemini support.

Choose the PromptKit parameter that represents the future user message and pass
it through `--prompt-param`. Bind only values that should be fixed for every
use of the role with repeated `--bind key=value` or `--bind-file key=path`.
Leave varying values unbound; Callee copies their PromptKit descriptions into
the role's top-level `params` map and gives each one a runtime placeholder.
Empty compile-time values must still be bound explicitly.

If the PromptKit template has a configurable persona, select it at creation
time with `--persona`. Never expose persona as a runtime role parameter.

## Create the role

Run the generated command from the project directory:

```bash
npx --yes @baldaworks/callee@0.8.0 promptkit role create "<role-id>" \
  --template "<promptkit-template>" \
  --description "<role-description>" \
  --type "<codex|claude|opencode|copilot|grok|generic_acp>" \
  --prompt-param "<message-parameter>" \
  --bind "<key=value>"
```

When requested, adjust PromptKit composition with `--persona`, repeated
`--protocol`, repeated `--taxonomy`, and either `--format` or `--no-format`.
These change PromptKit components, not Callee's flat role metadata.

Add only user-supplied optional runtime values: `--model`, `--reasoning`,
`--mode`, `--cmd`, and repeated `--extra-arg`. A `generic_acp` role requires
`--cmd`.

The default destination is `.callee/roles/<role-id>.md`. Use `--dry-run` when
the user requests a preview, `--output` for a different destination, and
`--force` only when the user explicitly authorizes replacement. Report the
role path and its human-language capability; do not invoke the newly generated
role unless the user asks.
