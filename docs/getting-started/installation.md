# Installation

Choose a coding-host setup when you want Callee exposed as create/run skills,
or install the executable when you want to call the CLI directly. Both routes
use the same resource format and runtime.

## Prerequisites

- For the npm launcher and one-command host setup, install Node.js with `npm`
  and `npx`.
- For installation from source, install the Go version declared in
  [`go.mod`](../../go.mod).
- Before running a provider-backed Role, install and authenticate its ACP
  provider. Host setup does not satisfy this runtime prerequisite; see
  [ACP provider configuration](../guides/acp-providers.md).

## Set up a coding host

Run one command from the project root:

| Host | Command |
| --- | --- |
| Codex | `npx --yes @baldaworks/callee@latest setup codex` |
| Claude Code | `npx --yes @baldaworks/callee@latest setup claude` |
| Grok Build | `npx --yes @baldaworks/callee@latest setup grok` |
| Copilot CLI | `npx --yes @baldaworks/callee@latest setup copilot` |
| OpenCode | `npx --yes @baldaworks/callee@latest setup opencode` |
| Cursor | `npx --yes @baldaworks/callee@latest setup cursor` |

Setup installs the host integration and six editable starter resources. Existing
managed files are preserved; `--force` replaces them. Use `--agent-root <dir>`
to install starter resources under a different exclusive catalog root.

For host invocation names, installed paths, and manual setup, see
[Coding-host integrations](../guides/coding-host-integrations.md).

## Install the CLI with npm

Install the launcher globally for repeated shell use:

```bash
npm install --global @baldaworks/callee@latest
callee --version
```

Or run any command without a global installation:

```bash
npx --yes @baldaworks/callee@latest --version
```

The npm launcher selects a CGO-disabled native executable for macOS AMD64 or
ARM64, Linux AMD64 or ARM64, and Windows AMD64.

## Install from Go source

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
callee --version
```

Ensure the Go installation directory, normally `$GOBIN` or `$GOPATH/bin`, is on
`PATH`.

## Next step

Continue with the [Quickstart](quickstart.md) to inspect and run the starter
workflow.
