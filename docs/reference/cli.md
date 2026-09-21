# CLI reference

Use `callee <command> --help` as the current flag authority. This page maps the
public command surface and its read-only inspection operations; task procedures
live in the linked guides.

## Global flags

| Flag | Effect |
| --- | --- |
| `--agent-root <dir>` | Use one exclusive agent catalog and write root instead of the user and project defaults. |
| `--permissions <ask|allow|deny>` | Override every Role's effective ACP policy for `agent run` or `agent view`. |
| `--debug` | Enable debug diagnostics. |
| `--trace` | Enable trace diagnostics and override `--debug`. |
| `--version` | Print the executable version. |

## Command map

```text
callee
├── agent
│   ├── import
│   ├── list
│   ├── run
│   ├── schema
│   ├── validate
│   └── view
├── bridge codex
├── doctor
├── promptkit
│   ├── list
│   ├── search
│   ├── show
│   └── role create
└── setup
```

## Agent catalog commands

`agent list` loads the complete registry, sorts IDs lexicographically, and can
filter by exact `--kind`. `--json` preserves structured stdout and enables
structured command errors on stderr.

```bash
callee agent list
callee agent list --kind Role
callee agent list --json
```

`agent view <agent-id>` prints one canonical resource, its resolved tree,
effective policies, and unbound Role parameters. It does not start providers.
Use `--json` for the recursive structured representation.

`agent validate <path>` decodes and validates one physical resource without
resolving its children. Success prints `<path>: ok`.

`agent schema <kind>` prints a standalone JSON Schema for one supported kind,
derived from the same embedded source as the checked-in
[`schema.json`](../../internal/agent/schema.json).

`agent run <agent-id>` executes one resolved tree; see
[Running agents](../guides/running-agents.md). `agent import <repo>` copies a
remote catalog; see [Importing agents](../guides/importing-agents.md).

Commands that load the registry fail on an invalid current-version resource,
duplicate ID, unresolved reference, graph cycle, or resolved effective-ID
collision.

## Doctor and graph inspection

```bash
callee doctor
callee doctor --timeout 90s
callee doctor --graph text
callee doctor --graph mermaid
callee doctor --graph dot
```

Plain doctor performs static validation, then initializes configured Role
providers and disposable sessions without sending a model prompt. It also
validates reachable TypeSafe and OpenRouter evaluation configuration without
making inference calls. `--timeout` applies to each provider group.

Graph modes are static-only and never start providers. Their edges show the
authored `canEscalate` value; `agent view` shows the effective capability for a
specific resolved occurrence.

## Setup and generation

`setup <codex|claude|grok|copilot|opencode|cursor>` installs one host integration
and starter catalog. See [Installation](../getting-started/installation.md) and
[Coding-host integrations](../guides/coding-host-integrations.md).

The `promptkit` catalog commands and `promptkit role create` generate Roles;
see [PromptKit](../guides/promptkit.md).

## Embedded Codex bridge

`bridge codex` exposes Callee's ACP bridge over stdin/stdout and does not require
a controlling TTY:

```bash
callee bridge codex --help
callee bridge codex version
```

Place Callee's global diagnostics flag before `bridge codex`. The bridge also
defines its own `--debug` after that command path.
