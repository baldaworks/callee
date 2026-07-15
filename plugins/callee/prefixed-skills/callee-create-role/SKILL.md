---
name: callee-create-role
description: Create a project-defined Callee Markdown role from an embedded PromptKit template. Use when the user asks to generate, scaffold, or author a new Callee role.
---

# Create a Callee role

## Resolve the CLI

Use the same Callee command launcher for the whole workflow:

1. Try `callee --version`. If it succeeds, use `callee` for every command.
2. If `callee` is not available, use
   `npx --yes @baldaworks/callee@0.10.0` as the command prefix.
3. If the fallback fails because the host blocks network or npm cache access,
   including `EAI_AGAIN` or `EROFS`, request the required approval and retry
   the exact command. Do not interpret a failed command as an empty catalog.
4. If neither launcher can run, report the launcher failure and the one-time
   CLI installation requirement instead of guessing about templates.

The examples below use `callee`; substitute the pinned `npx` prefix when the
local CLI is unavailable.

## Discover templates semantically

Do not search only for the user's exact wording. Translate the request into
two to four short capability queries covering the intended action, artifact,
domain, and interaction mode. Search each query and merge results by template
name:

```bash
callee promptkit search "<capability>" --type template --json
```

If searches return no plausible match, inspect `callee promptkit list --json`
and rank templates by their descriptions, parameter contracts, output
contracts, and modes. Do not dump the full catalog into the conversation.
Inspect the best candidates before recommending them:

```bash
callee promptkit show "<template>" --json
```

Present at most three candidates. For each, explain the intended output and
the input it expects, mark one as recommended, and wait for the user to confirm
the template before collecting the role contract or writing a file. Ask a
concise clarification when the candidates represent materially different
outcomes.

For example, interpret "spec writer" as requirements, design, and architecture
authoring. Recommend `author-requirements-doc` when the user wants structured
requirements from a natural-language feature description. Offer
`author-design-doc` when requirements already exist and implementation design
is wanted, and `author-architecture-spec` when system structure and
cross-cutting concerns are the target.

## Gather the role contract

After the template is confirmed, propose a kebab-case Callee role ID and a
concise capability description. Collect the runtime type explicitly from
`codex`, `claude`, `opencode`, `copilot`, `grok`, or `generic_acp`. Never
default or infer a type. A `generic_acp` role also requires `--cmd`.

Present the template parameters and propose how they map into the reusable
role. Choose the parameter representing the future user task as
`--prompt-param`. Bind only values that should be fixed for every use of the
role with repeated `--bind key=value` or `--bind-file key=path`. Leave varying
values unbound; Callee copies their PromptKit descriptions into the role's
top-level `params` map and gives each one a runtime placeholder. Empty
compile-time values must still be bound explicitly.

For `author-requirements-doc`, propose `description` as the prompt parameter
and leave `project_name`, `audience`, and `context` as runtime parameters unless
the user explicitly fixes them.

If the template has a configurable persona, select it at creation time with
`--persona`. Never expose persona as a runtime role parameter. Generated roles
use `api: callee.metalagman.dev`, `kind: role`, and a nested `provider` section;
the top-level `params` map remains separate. Do not add Gemini support.

## Decide the interaction mode

After the template and role contract are known, inspect the template's
instructions and output contract. Enable REPL when the resulting role is
expected to ask model-led follow-up questions after its declared runtime
parameters have been supplied. Treat instructions to clarify insufficient,
ambiguous, or conflicting information as a positive signal. Requirements and
specification roles that interview the user before producing the artifact are
REPL roles.

Do not enable REPL merely because the role has unbound runtime parameters.
Callee can collect missing declared parameters separately; REPL describes
clarification led by the role model. If the template can produce its complete
artifact from the user message and declared parameters in one model turn, keep
REPL disabled.

Pass `--repl` only for a positive decision. Omit it otherwise so generated
roles do not contain `repl: false`. PromptKit does not perform this semantic
analysis; this skill makes the decision before invoking the deterministic CLI.

## Create the role

Run the confirmed command from the project directory:

```bash
callee promptkit role create "<role-id>" \
  --template "<promptkit-template>" \
  --description "<role-description>" \
  --type "<codex|claude|opencode|copilot|grok|generic_acp>" \
  --prompt-param "<message-parameter>" \
  --bind "<key=value>"
```

Add `--repl` to this command only when the interaction-mode decision is
positive. Omit it when the decision is negative.

When requested, adjust PromptKit composition with `--persona`, repeated
`--protocol`, repeated `--taxonomy`, and either `--format` or `--no-format`.
These change PromptKit components, not Callee's provider metadata.

Add only user-supplied optional runtime values: `--model`, `--reasoning`,
`--mode`, `--cmd`, and repeated `--extra-arg`.

The default destination is `.callee/roles/<role-id>.md`. Use `--dry-run` when
the user requests a preview, `--output` for a different destination, and
`--force` only when the user explicitly authorizes replacement. Report the
role path and its human-language capability; do not invoke the newly generated
role unless the user asks.
