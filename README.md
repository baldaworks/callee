# Callee

[![Test](https://github.com/baldaworks/callee/actions/workflows/test.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/test.yml)
[![Lint](https://github.com/baldaworks/callee/actions/workflows/lint.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/lint.yml)
[![Security](https://github.com/baldaworks/callee/actions/workflows/security.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/security.yml)
[![Latest release](https://img.shields.io/github/v/release/baldaworks/callee)](https://github.com/baldaworks/callee/releases/latest)
[![npm version](https://img.shields.io/npm/v/%40baldaworks%2Fcallee)](https://www.npmjs.com/package/@baldaworks/callee)
[![License: MIT](https://img.shields.io/github/license/baldaworks/callee)](LICENSE)

## Markdown-defined agents and deterministic workflows

Callee lets a repository define reusable AI agents and workflows as versioned
Markdown or YAML. Compose provider-backed Roles with local scripts, human
approval, typed evaluations, sequencing, parallel work, bounded loops, and
deterministic routing. Callee validates the complete tree before execution and
returns one final artifact through a CLI that works directly or through a
supported coding host.

## Set up your coding host

Run the command for your host from the project root. Setup installs Callee's
create/run skills and six editable starter agents; provider CLIs and credentials
remain separate prerequisites.

| Host | Setup | Run Agent | Create Agent |
| --- | --- | --- | --- |
| Codex | `npx --yes @baldaworks/callee@latest setup codex` | `$callee:run-agent` | `$callee:create-agent` |
| Claude Code | `npx --yes @baldaworks/callee@latest setup claude` | `/callee:run-agent` | `/callee:create-agent` |
| Grok Build | `npx --yes @baldaworks/callee@latest setup grok` | `/callee-run-agent` | `/callee-create-agent` |
| Copilot CLI | `npx --yes @baldaworks/callee@latest setup copilot` | `/callee-run-agent` | `/callee-create-agent` |
| OpenCode | `npx --yes @baldaworks/callee@latest setup opencode` | `callee-run-agent` skill (`/callee` wrapper) | `callee-create-agent` skill (`/callee-create-agent` wrapper) |
| Cursor | `npx --yes @baldaworks/callee@latest setup cursor` | `callee-run-agent` skill | `callee-create-agent` skill |

Then ask the host to run a project workflow. For example, in Codex:

```text
$callee Run workflows/investigate to explain this project's architecture and main entry points.
```

See [Coding-host integrations](docs/guides/coding-host-integrations.md) for
installed files, manual setup, invocation names, and the boundary between a
host integration and a runtime provider.

## Minimal CLI quickstart

Node.js with npm provides the shortest direct CLI path. After running one setup
command from the table above:

```bash
npx --yes @baldaworks/callee@latest agent list
npx --yes @baldaworks/callee@latest agent view workflows/investigate
npx --yes @baldaworks/callee@latest agent run workflows/investigate --message "Explain this project's architecture and main entry points"
```

The last command validates and resolves the selected tree, starts its configured
providers, writes lifecycle diagnostics to stderr, and writes the successful
root artifact to stdout. Provider executables and authentication must already
be available. See the [Quickstart](docs/getting-started/quickstart.md) for
expected output and common first-run failures, or
[Installation](docs/getting-started/installation.md) for global npm and Go
installation alternatives.

## How it works

Callee discovers resources below the project `.callee/` directory and the user
configuration root, validates their schema and references, and resolves one
execution tree. A root run owns one ephemeral shared state object. Every Role
visit receives a fresh provider session, while compatible Roles may reuse an
ACP provider process. Composite resources control ordering, routing, fan-out,
and bounded iteration. The CLI remains the execution boundary: Callee does not
run a server or persist workflow threads or state.

## Documentation

- [Documentation hub](docs/index.md)
- [Run agents](docs/guides/running-agents.md)
- [Import agents](docs/guides/importing-agents.md)
- [Agent resource reference](docs/reference/agent-resources.md)
- [Workflow semantics](docs/reference/workflow-semantics.md)
- [CLI reference](docs/reference/cli.md)
- [Examples](docs/examples/index.md)

## License and notices

Callee is released under the [MIT License](LICENSE). See
[Third-party notices](THIRD_PARTY_NOTICES.md) for embedded and statically linked
dependencies.
