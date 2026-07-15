# Callee

## Provider-aware subagent roles, described in Markdown

Callee lets developers and teams keep provider-aware specialist subagent roles
beside their code. Each project-local Markdown file defines role behavior and
declares its runtime provider through flat frontmatter; review roles in pull
requests and override them per repository.

Built for Codex, Claude Code, Grok Build, Copilot CLI, and OpenCode.

```text
.callee/roles/*.md  →  CLI  →  capability-based workflow
```

The core CLI keeps no durable conversation store, mappings, or bindings. The
host plugin can retain active work only within its current conversation, then
routes a follow-up naturally when it still has that context.

## Get started

From the repository you want to work in, install a host plugin and create
`.callee/roles/reviewer.md`:

```bash
npx --yes @baldaworks/callee@0.7.0 setup codex
# or: npx --yes @baldaworks/callee@0.7.0 setup claude
# or: npx --yes @baldaworks/callee@0.7.0 setup grok
# or: npx --yes @baldaworks/callee@0.7.0 setup copilot
# or: npx --yes @baldaworks/callee@0.7.0 setup opencode
```

For Codex, this creates the following project-local reviewer. The other setup
targets use their matching `type` with the same review contract.

```md
---
description: Reviews code changes for correctness and regressions.
type: codex
model: gpt-5-codex
reasoning: high
mode: review
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
```

Start a fresh host session and describe the outcome you want:

```text
$callee Review the current changes.
$callee Review the current changes and fix any verified findings.
$callee With the reviewer role, review the current changes.
$callee-promptkit Create a Go code-review role from a PromptKit template for Codex.
```

`$callee <task>` runs existing roles. It discovers the role catalog for a new
task, matches its descriptions to the requested work, and can combine
independent and dependent stages. `$callee-promptkit <role request>` authors a
new role from a PromptKit template. A user can naturally name a role as a
required first stage; the plugin resolves it from the catalog and asks for
clarification instead of substituting an ambiguous name. It keeps role IDs and
conversation handles internal. The editable reviewer created by setup is a
sample, not a special routing rule.

## Command line

Install the binary:

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

Or run the published command directly:

```bash
npx --yes @baldaworks/callee@0.7.0 --version
```

Prompt a role:

```bash
callee prompt --role reviewer --message "Review the current changes"
```

Roles may declare additional runtime inputs as a top-level description map:

```md
params:
  audience: Intended readers for the review
  context: Repository or change context
```

Supply every declared parameter when starting a thread. Empty values are
explicit, and file inputs preserve their contents exactly:

```bash
callee prompt --role reviewer \
  --message-file ./task.md \
  --param audience=maintainers \
  --param-file context=./context.md
```

Use `--roles-dir ./examples/roles` to load only a specific directory.

`prompt` has a 10 minute timeout covering runtime startup and the prompt. Override
it with `--timeout`, for example `--timeout 90s`.

### Continue a thread

Use `--json` when you need the opaque provider thread handle returned by a
prompt. It writes one result object to stdout and JSON Lines diagnostics to
stderr, including messages emitted by the provider:

```bash
callee prompt --role reviewer --message "Review the current changes" --json
# {"threadId":"<provider thread handle>","content":"...","resumed":false}

callee prompt --role reviewer --thread-id "<provider thread handle>" \
  --message "Now focus on tests" --json
```

On resume, the message is sent directly to the existing thread. Do not repeat
`--param` or `--param-file`; role parameters initialize only a new thread.

`threadId` is provider-owned, not Callee-generated. Callee keeps no local
thread store or role/workspace binding. If a provider cannot resume it and
starts a replacement conversation, the response contains that replacement
`threadId` and `"resumed": false`.

### Inspect roles

List configured roles with their descriptions and declared parameter names:

```bash
callee role list
callee role list --json
```

Inspect one role's metadata and parameter descriptions, or print its normalized
Markdown definition:

```bash
callee role view reviewer
callee role view reviewer --json
callee role view reviewer --markdown
```

### Doctor

Check that every loaded role can initialize its runtime without sending a model
prompt:

```bash
callee doctor
callee doctor --roles-dir ./examples/roles --timeout 90s
```

Provider processes are checked sequentially with a 60 second timeout per
provider. Roles with the same `type`, resolved command, and extra arguments
share one check; Callee still reports an outcome for every role. Successfully
initialized runtimes are closed before the next check.

## Host integrations

Callee is available as a Codex, Claude Code, Grok Build, and Copilot CLI
plugin. OpenCode uses its native skills and slash commands. Every integration
is a CLI wrapper: it runs Callee once for each role request and needs no server
configuration.

Install a plugin manually when you do not want to create the sample role:

```bash
# Codex
codex plugin marketplace add baldaworks/callee --sparse .agents/plugins
codex plugin add callee@callee

# Grok Build
grok plugin marketplace add baldaworks/callee
grok plugin install callee@callee --trust

# Copilot CLI
copilot plugin marketplace add baldaworks/callee
copilot plugin install callee@callee
```

For Claude Code:

```text
/plugin marketplace add baldaworks/callee
/plugin install callee@callee
/reload-plugins
```

For OpenCode, install the project-local skills and commands:

```bash
npx --yes @baldaworks/callee@0.7.0 setup opencode
```

OpenCode then supports natural-language skill use such as `$callee Review the
current changes.`, plus `/callee Review the current changes` and
`/callee-promptkit Create a reviewer role`. The installer creates no JavaScript
plugin or custom tools.

## Roles

Start with the [`reviewer`](examples/roles/reviewer.md). Other examples include
an [`explorer`](examples/roles/explorer.md),
[`architect`](examples/roles/architect.md),
[`implementer`](examples/roles/implementer.md), and
[`tester`](examples/roles/tester.md).

A role is Markdown with flat provider fields in YAML frontmatter. The Markdown
body must contain exactly one `{{ prompt }}`. Each name in the optional
top-level `params` description map must appear in the body at least once;
undeclared mustache fragments remain ordinary Markdown.

| Field | Required | Meaning |
|---|---:|---|
| `description` | yes | Role description shown by `callee role list` |
| `type` | yes | Built-in Callee runtime type |
| `cmd` | no | Executable override |
| `model` | no | Model identifier |
| `reasoning` | no | Norma Runtime `reasoning_effort` |
| `mode` | no | Runtime session mode |
| `extra_args` | no | Arguments appended by Norma Runtime |
| `params` | no | Runtime parameter names mapped to human-readable descriptions |

Supported types: `codex`, `claude`, `opencode`, `copilot`, `grok`, and
`generic_acp`. `generic_acp` requires `cmd`.

```md
---
description: Runs a custom ACP-compatible reviewer.
type: generic_acp
cmd: /usr/local/bin/company-review-agent
model: reviewer-v2
reasoning: high
mode: review
extra_args:
  - --stdio
---

Review the following task:

{{ prompt }}
```

### Generate a role with PromptKit

Callee embeds the complete pinned PromptKit catalog through
[`PromptKitty`](https://github.com/baldaworks/promptkitty), so catalog browsing
and assembly need neither Node.js nor network access. Browse templates and
their declared parameters before creating a role:

```bash
callee promptkit list
callee promptkit search "code review" --type template
callee promptkit show review-code
```

Use `callee promptkit list --all` to include personas, protocols, formats, and
taxonomies. The embedded PromptKitty CLI also provides `promptkit assemble`;
`list`, `search`, `show`, and `assemble` support `--json`.

```bash
callee promptkit role create go-reviewer \
  --template <promptkit-template> \
  --description "Reviews Go code for correctness and regressions." \
  --type codex \
  --prompt-param code \
  --bind language=Go \
  --bind context=repository \
  --dry-run
```

`--prompt-param` names the PromptKit parameter supplied by Callee's future user
message. Fix role-wide constants with repeated `--bind key=value` or
`--bind-file key=path`; unbound PromptKit parameters become described runtime
role parameters. The PromptKit template is fully rendered during authoring,
then Callee adds one Runtime Input section with exactly one placeholder for the
message and each unbound parameter. Configurable personas must be selected at
creation time with `--persona`.

Composition can be adjusted with `--persona`, repeated `--protocol`, repeated
`--taxonomy`, and either `--format` or `--no-format`. Omit `--dry-run` to write
the generated role to `.callee/roles/go-reviewer.md`, use `--output` to choose
another file, and use `--force` to replace an existing file. Required Callee
metadata is always explicit: `--description` and `--type`; `generic_acp` also
requires `--cmd`.

## Role discovery

Callee loads user roles first from `$XDG_CONFIG_HOME/callee/roles` (or
`~/.config/callee/roles`), then project roles from `.callee/roles`. Project
roles override user roles with the same path-relative ID. Nested files use
slash-separated IDs, such as `code/explorer`.

## Current limitations

Each CLI prompt creates and closes its runtime. Direct CLI continuation is
available only when the caller explicitly supplies an opaque thread handle with
`--thread-id`. Plugin workflow context is limited to the current host
conversation.
