# Callee

## Versioned Markdown roles for AI coding agents

Callee lets developers and teams keep reusable specialist roles beside their
code and route natural-language work through them as CLI subagents. Roles are
project-local Markdown files: review them in pull requests and override them
per repository.

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
npx --yes @baldaworks/callee@0.6.0 setup codex
# or: npx --yes @baldaworks/callee@0.6.0 setup claude
# or: npx --yes @baldaworks/callee@0.6.0 setup grok
# or: npx --yes @baldaworks/callee@0.6.0 setup copilot
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
```

`$callee <task>` is the only plugin invocation. The plugin discovers the role
catalog for a new task, matches its descriptions to the requested work, and
can combine independent and dependent stages. A user can naturally name a role
as a required first stage; the plugin resolves it from the catalog and asks for
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
npx --yes @baldaworks/callee@0.6.0 --version
```

Prompt a role:

```bash
callee prompt --role reviewer --message "Review the current changes"
```

Use `--roles-dir ./examples/roles` to load only a specific directory.

`prompt` has a 10 minute timeout covering ACP startup and the prompt. Override
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

`threadId` is provider-owned, not Callee-generated. Callee keeps no local
thread store or role/workspace binding. If a provider cannot resume it and
starts a replacement conversation, the response contains that replacement
`threadId` and `"resumed": false`.

### Inspect roles

List configured role IDs and descriptions:

```bash
callee list
callee list --json
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

## Host plugins

Callee is available as a Codex, Claude Code, Grok Build, and Copilot CLI
plugin. Each plugin is a CLI wrapper: it runs Callee once for each role request
and needs no server configuration.

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

## Roles

Start with the [`reviewer`](examples/roles/reviewer.md). Other examples include
an [`explorer`](examples/roles/explorer.md),
[`architect`](examples/roles/architect.md),
[`implementer`](examples/roles/implementer.md), and
[`tester`](examples/roles/tester.md).

A role is Markdown with flat YAML frontmatter. The Markdown body must contain
exactly one `{{ prompt }}`; no other template expressions are supported.

| Field | Required | Meaning |
|---|---:|---|
| `description` | yes | Role description shown by `callee list` |
| `type` | yes | Built-in Callee runtime type |
| `cmd` | no | Executable override |
| `model` | no | Model identifier |
| `reasoning` | no | Norma Runtime `reasoning_effort` |
| `mode` | no | ACP session mode |
| `extra_args` | no | Arguments appended by Norma Runtime |

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
