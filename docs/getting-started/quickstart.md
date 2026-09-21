# Quickstart

Run the starter `investigate` workflow to verify discovery, graph resolution,
and provider execution in the current project.

## Before you begin

Complete one route in [Installation](installation.md). A coding-agent setup command must
have created the starter resources, and the ACP provider selected by those Roles
must be installed and authenticated.

## Inspect the starter catalog

These examples use the one-shot npm launcher; replace its prefix with `callee`
after a global or Go installation:

```bash
npx --yes @baldaworks/callee@latest agent list
npx --yes @baldaworks/callee@latest agent view workflows/investigate
```

The list should include `workflows/investigate`. The view reports the resolved
tree, effective policies, and required Role parameters without starting a
provider.

## Run the workflow

```bash
npx --yes @baldaworks/callee@latest agent run workflows/investigate --message "Explain this project's architecture and main entry points"
```

Callee writes lifecycle events to stderr. After the run finishes and providers
close, it writes one successful root artifact to stdout. Use the exit status,
not empty stderr, to determine success.

## If the first run fails

| Failure | What to check |
| --- | --- |
| `workflows/investigate` was not found | Run setup from the intended project root, or pass the same `--agent-root <dir>` used during setup. |
| Provider executable was not found | Install the Role's configured provider or set a valid `spec.provider.cmd`. |
| Provider startup or session preparation failed | Authenticate the provider and verify its model, mode, and reasoning settings. |
| Interactive terminal is required | Run under a controlling TTY, or choose a compatible one-shot tree with automatic permissions. |
| Registry or graph validation failed | Fix the first reported resource, reference, duplicate ID, or cycle before retrying. |

Continue with [Running agents](../guides/running-agents.md) for parameters,
interactive mode, permissions, terminal behavior, and output contracts.
