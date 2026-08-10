# OpenCode rejudge-style review panel

This pack approximates the public
[Rejudge](https://github.com/syabro/rejudge) workflow shape with Callee
resources and repo-authored prompts. Three OpenCode-backed reviewer Roles
inspect the repository independently, then a final OpenCode judge Role
synthesizes their write-ups.

Callee differs from Rejudge in two important ways:

- Callee does not define `Parallel`, so the reviewers run sequentially even
  though each visit gets a fresh OpenCode session.
- Callee does not provide a judge-side follow-up tool such as `ask_panel`, so
  the judge sees only the saved reviewer outputs.

The checked-in model IDs match the public Rejudge README examples. If your
OpenCode installation exposes different models, update the
`spec.provider.model` values before running the pack.

Because Callee does not currently expose a dedicated read-only reviewer policy,
the reviewer Roles stay in `permissions.mode: ask`. That lets the operator
allow inspection while still denying any mutating request. The judge Role is
checked in with `permissions.mode: deny` so it synthesizes only the supplied
panel text.

## Validate and inspect the pack

From the repository root, validate the workflows and inspect their resolved
trees:

```bash
callee agent validate examples/opencode/rejudge/workflows/rejudge.md
callee agent validate examples/opencode/rejudge/workflows/rejudge-diff.md
callee --agent-root examples agent view opencode/rejudge/workflows/rejudge
callee --agent-root examples agent view opencode/rejudge/workflows/rejudge-diff
```

## Import the pack

Import the pack from GitHub into the current project's `.callee` catalog:

```bash
callee agent import baldaworks/callee \
  --path examples/opencode/rejudge \
  --prefix opencode/rejudge
```

After importing, inspect the workflows and run them:

```bash
callee agent view opencode/rejudge/workflows/rejudge
callee agent run opencode/rejudge/workflows/rejudge \
  --message "Review this project and identify the two most important risks."

callee agent view opencode/rejudge/workflows/rejudge-diff
callee agent run opencode/rejudge/workflows/rejudge-diff \
  --message "Review the current changes and identify the two most important risks."
```

Running this pack requires an installed, authenticated OpenCode CLI plus model
IDs that exist in the local OpenCode configuration.
