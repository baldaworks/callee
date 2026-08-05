# Codex Sol + Luna Max pack

This pack provides an opinionated Codex workflow for deliberate,
evidence-backed changes:

- `codex/sol-luna/roles/sol-planner` uses `gpt-5.6-sol` with high reasoning and
  denied permissions for evidence-backed, read-only planning and review.
- `codex/sol-luna/roles/luna-max-implementer` uses `gpt-5.6-luna` with max
  reasoning and operator-confirmed permissions for bounded implementation.
- `codex/sol-luna/workflows/plan-then-implement` runs Sol first, then passes
  both the original task and Sol's plan to Luna Max. Luna retains its `ask`
  permission mode, so mutating actions remain operator-confirmed.

Use `plan-then-implement` as the normal entry point. The individual Roles are
also available when you only need planning or implementation.

## Validate and inspect the pack

From the repository root, validate the workflow and inspect its resolved tree:

```bash
callee agent validate examples/codex/sol-luna/workflows/plan-then-implement.md
callee --agent-root examples agent list --kind Sequential
callee --agent-root examples agent view codex/sol-luna/workflows/plan-then-implement
```

## Import the pack

Import the pack from GitHub into the current project's `.callee` catalog:

```bash
callee agent import baldaworks/callee \
  --path examples/codex/sol-luna \
  --prefix codex/sol-luna
```

`--path` selects this remote pack subtree. `--prefix` keeps its resources in
the `codex/sol-luna` namespace. The pack's `README.md` is documentation without
Callee frontmatter, so catalog discovery and import skip it. They likewise skip
structurally valid documents with no current `apiVersion`, while malformed
documents and current-version resources remain strict errors.

After importing, inspect the workflow and run it:

```bash
callee agent list
callee agent view codex/sol-luna/workflows/plan-then-implement
callee agent run codex/sol-luna/workflows/plan-then-implement \
  --message "Add the requested feature with appropriate tests and validation."
```

Running a Role requires an authenticated Codex installation and a controlling
terminal. The model and reasoning selections are backend-specific; structural
validation does not guarantee that every Codex installation exposes
`gpt-5.6-luna` or `max` reasoning.
