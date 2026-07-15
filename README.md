# Callee

[![Test](https://github.com/baldaworks/callee/actions/workflows/test.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/test.yml)
[![Lint](https://github.com/baldaworks/callee/actions/workflows/lint.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/lint.yml)
[![Security](https://github.com/baldaworks/callee/actions/workflows/security.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/security.yml)
[![Latest release](https://img.shields.io/github/v/release/baldaworks/callee)](https://github.com/baldaworks/callee/releases/latest)
[![npm version](https://img.shields.io/npm/v/%40baldaworks%2Fcallee)](https://www.npmjs.com/package/@baldaworks/callee)
[![License: MIT](https://img.shields.io/github/license/baldaworks/callee)](LICENSE)

## Provider-aware subagent roles, described in Markdown

Callee gives users of agent harnesses a project-local way to define specialist
roles. Each Markdown file contains the role instructions and declares its
runtime provider through strict YAML frontmatter, so roles can be reviewed in
pull requests, shared with the repository, and overridden per project.

Use the same CLI-backed workflow from **Codex, Claude Code, Grok Build, Copilot
CLI, and OpenCode**. Callee ships as a native Go executable; the host plugins
and skills remain thin wrappers around that CLI.

Set up the integration for your host:

```bash
npx --yes @baldaworks/callee@latest setup codex
```

Or install the native command directly:

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

**Contents:** [Quick start](#quick-start) ·
[The wedge](#the-wedge-roles-that-ship-with-the-code) ·
[How Callee works](#how-callee-works) · [Distribution](#distribution) ·
[Tradeoffs and limitations](#tradeoffs-and-limitations) ·
[Command line](#command-line) · [Host integrations](#host-integrations) ·
[Roles](#roles) · [Role discovery](#role-discovery) · [License](#license)

## Quick start

From the repository where you want to use Callee, install the integration for
your agent harness and create `.callee/roles/reviewer.md`:

```bash
npx --yes @baldaworks/callee@0.9.0 setup codex
# Replace codex with claude, grok, copilot, or opencode.
```

This creates a project-local reviewer role for the selected host:

```md
---
api: callee.metalagman.dev
kind: role
description: Reviews code changes for correctness and regressions.
provider:
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

Start a fresh host session and describe the outcome you want. Hosts that add a
plugin namespace use the short skill IDs; hosts that expose skill IDs directly
use the `callee-` prefix:

- **Codex**
  - Run: `$callee:run-role Review the current changes.`
  - Create: `$callee:create-role Create a Go code-review role.`
- **Claude Code**
  - Run: `/callee:run-role Review the current changes.`
  - Create: `/callee:create-role Create a Go code-review role.`
- **Grok Build**
  - Run: `/callee-run-role Review the current changes.`
  - Create: `/callee-create-role Create a Go code-review role.`
- **Copilot CLI**
  - Run: `/callee-run-role Review the current changes.`
  - Create: `/callee-create-role Create a Go code-review role.`
- **OpenCode**
  - Run: `$callee-run-role Review the current changes.`
  - Create: `$callee-create-role Create a Go code-review role.`

The run-role skill discovers the role catalog for a new task, matches its
descriptions to the requested work, and can combine independent and dependent
stages. The create-role skill authors a new role from a PromptKit template. A
user can naturally name a role as a required first stage; the plugin resolves
it from the catalog and asks for clarification instead of substituting an
ambiguous name. It keeps role IDs and conversation handles internal. The
editable reviewer created by setup is a sample, not a special routing rule.

## The wedge: roles that ship with the code

Agent harnesses are good at running agents, but specialist instructions often
live in personal configuration, copied prompts, or host-specific formats.
Callee's entry point is narrower: put a reviewable Markdown role in the
repository and invoke it through the harness your team already uses.

That gives a project one versioned role contract without turning Callee into a
hosted platform. Callee does not operate a server, a durable orchestrator, or a
conversation database. It resolves the selected role, starts the corresponding
provider runtime through [Norma Runtime](https://github.com/normahq/runtime),
returns the result, and exits.

## How Callee works

```text
.callee/roles/*.md
        │
        ▼
   callee prompt  ──▶  Norma Runtime  ──▶  provider CLI
        ▲                                      │
        └──────── result on stdout ◀───────────┘
```

The role file supplies instructions, nested provider metadata, and optional
runtime parameters. Every `callee prompt` invocation creates and closes its
runtime; Callee has no background process or local thread store. Diagnostics
go to stderr, leaving role output on stdout for scripts and host integrations.

Providers own their opaque thread handles. To continue a conversation from the
CLI, the caller must retain the returned handle and pass it back with
`--thread-id`.

## Distribution

Callee is written in Go and released as one native **Callee executable per
target platform**, with CGO disabled. The configured release targets are:

| Operating system | Architectures |
| --- | --- |
| macOS | AMD64, ARM64 |
| Linux | AMD64, ARM64 |
| Windows | AMD64 |

There are two installation paths:

- `go install github.com/baldaworks/callee/cmd/callee@latest` compiles and
  installs the command with the Go toolchain.
- `npx --yes @baldaworks/callee@latest ...` obtains and runs the published npm
  distribution. Node.js is required for this `npx` path, not for an already
  installed native Callee executable.

“Single binary” describes the Callee CLI, not the complete agent stack. The
binary does **not** bundle provider executables, provider credentials, or
hosted model access. Install and authenticate the provider CLI required by each
role separately.

## Tradeoffs and limitations

### Pros

- Roles are project-local Markdown that can be reviewed and versioned with the
  code they support.
- One CLI contract works across five supported agent harness integrations.
- The directly installed Callee executable itself needs no Callee server or
  Node.js runtime.
- Flat frontmatter keeps provider selection explicit and easy to inspect.
- The complete pinned PromptKit catalog is embedded for offline role authoring.

### Cons

- Each provider CLI must be installed, authenticated, and maintained outside
  Callee.
- Every prompt starts and closes a provider runtime instead of reusing a
  persistent Callee process.
- Installation and invocation syntax differs slightly between host plugin
  systems.
- Provider-aware roles intentionally contain runtime-specific metadata rather
  than pretending every provider has identical capabilities.

### Limitations

- Callee has no server transport, durable thread store, role/thread mapping, or
  handle binding.
- Direct CLI continuation works only when the caller supplies the opaque
  provider handle with `--thread-id`.
- Plugin workflow context lasts only for the current host conversation.
- A provider that cannot resume a supplied thread may return a replacement
  thread with `"resumed": false`.
- Compatibility is limited to the explicitly listed runtime types and host
  integrations; Callee does not claim universal harness support.

## Command line

Install the binary:

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
```

Or run the published command directly through npm:

```bash
npx --yes @baldaworks/callee@latest --version
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

`prompt` has a 15 minute default timeout. A role may override it with
`provider.timeout`; an explicit `--timeout` takes precedence over both. For a
one-shot role the budget covers runtime startup and the prompt together.

Set `provider.repl: true` to keep one provider process and one local session
open for line-oriented follow-up prompts. The startup and each active turn get
their own timeout budget; time spent waiting for the next input line is not
timed. Blank lines are ignored, and `exit`, `quit`, or EOF closes the runtime.
REPL roles use human-readable output and do not support `--json`.

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
codex plugin marketplace add baldaworks/callee \
  --sparse .agents/plugins \
  --sparse plugins/callee
codex plugin add callee@callee

# Grok Build
grok plugin marketplace add baldaworks/callee
grok plugin install callee@callee --trust

# Copilot CLI
copilot plugin marketplace add baldaworks/callee
copilot plugin install callee@callee
```

Rerun `setup codex` to migrate an existing pre-0.8.0 marketplace snapshot. It
replaces the old sparse registration before installing the plugin again.

For Claude Code:

```text
/plugin marketplace add baldaworks/callee
/plugin install callee@callee
/reload-plugins
```

For OpenCode, install the project-local skills and commands:

```bash
npx --yes @baldaworks/callee@latest setup opencode
```

OpenCode then supports natural-language skill use such as `$callee-run-role
Review the current changes.` and `$callee-create-role Create a reviewer role`,
plus the compatible `/callee` and `/callee-promptkit` commands. The installer
creates no JavaScript plugin or custom tools.

## Roles

Start with the [`reviewer`](examples/roles/reviewer.md). Other examples include
an [`explorer`](examples/roles/explorer.md),
[`architect`](examples/roles/architect.md),
[`implementer`](examples/roles/implementer.md), and
[`tester`](examples/roles/tester.md).

A role is Markdown with provider fields nested under `provider` in YAML
frontmatter. `api` and `kind` may be omitted because the roles-directory loader
supplies `callee.metalagman.dev` and `role`. The Markdown
body must contain exactly one `{{ prompt }}`. Each name in the optional
top-level `params` description map must appear in the body at least once;
undeclared mustache fragments remain ordinary Markdown.

| Field | Required | Meaning |
| --- | ---: | --- |
| `api` | no | API identity; defaults from the loader context |
| `kind` | no | Resource kind; `roles` directories default it to `role` |
| `description` | yes | Role description shown by `callee role list` |
| `provider.type` | yes | Built-in Callee runtime type |
| `provider.cmd` | no | Executable override |
| `provider.model` | no | Model identifier |
| `provider.reasoning` | no | Norma Runtime `reasoning_effort` |
| `provider.mode` | no | Runtime session mode |
| `provider.extra_args` | no | Arguments appended by Norma Runtime |
| `provider.repl` | no | Enables the interactive line-oriented REPL; defaults to `false` |
| `provider.timeout` | no | Positive Go duration used unless `--timeout` is explicit |
| `params` | no | Runtime parameter descriptions |

Supported types: `codex`, `claude`, `opencode`, `copilot`, `grok`, and
`generic_acp`. `generic_acp` requires `cmd`.

```md
---
api: callee.metalagman.dev
kind: role
description: Runs a custom ACP-compatible reviewer.
provider:
  type: generic_acp
  cmd: /usr/local/bin/company-review-agent
  model: reviewer-v2
  reasoning: high
  mode: review
  extra_args:
    - --stdio
  repl: true
  timeout: 20m
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

## License

Callee is released under the [MIT License](LICENSE). You may use, copy, modify,
distribute, sublicense, and sell the software subject to the license notice and
terms. The software is provided without warranty.
