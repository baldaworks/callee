# Codex Sol + Luna Max pack

This pack provides two separate Codex Roles for a deliberate plan-then-implement
workflow:

- `codex/sol-luna/roles/sol-planner` uses `gpt-5.6-sol` with high reasoning and
  denied permissions for evidence-backed, read-only planning and review.
- `codex/sol-luna/roles/luna-max-implementer` uses `gpt-5.6-luna` with max
  reasoning and operator-confirmed permissions for bounded implementation.

The Roles are independent. You can run either one directly, or pass the Sol
artifact to Luna Max as the input for a later run. No Callee workflow state is
shared between them.

## Validate and inspect the pack

From the repository root, validate the two resources and inspect their
namespaced IDs:

```bash
go run ./cmd/callee agent validate examples/codex/sol-luna/roles/sol-planner.md
go run ./cmd/callee agent validate examples/codex/sol-luna/roles/luna-max-implementer.md
go run ./cmd/callee --agent-root examples agent list --kind Role
go run ./cmd/callee --agent-root examples agent view codex/sol-luna/roles/sol-planner
```

## Import the pack

Import the pack from GitHub into the current project's `.callee` catalog:

```bash
callee agent import baldaworks/callee \
  --path examples/codex/sol-luna \
  --prefix codex/sol-luna
```

`--path` selects this remote pack subtree. `--prefix` keeps the imported
resources in the `codex/sol-luna` namespace, so the Sol Role is available as
`codex/sol-luna/roles/sol-planner` and the Luna Max Role as
`codex/sol-luna/roles/luna-max-implementer`. The pack's `README.md` is
documentation and is skipped during catalog discovery and import.

After importing, inspect the catalog or run the read-only planner:

```bash
callee agent list
callee agent view codex/sol-luna/roles/sol-planner
callee agent run codex/sol-luna/roles/sol-planner \
  --message "Plan the requested change with verified facts, risks, and validation."
```

Running a Role requires an authenticated Codex installation and a controlling
terminal. The model and reasoning selections are backend-specific; structural
validation does not guarantee that every Codex installation exposes
`gpt-5.6-luna` or `max` reasoning.
