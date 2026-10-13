# GoalKeeper pack

GoalKeeper repeats implementation and independent review until the reviewer
accepts the result. This pack bundles its complete workflow and both Roles:

- [GoalKeeper Loop](workflows/goalkeeper.md) runs the worker and validator.
- [Implementer Role](roles/implementer.md) makes a bounded change and runs
  relevant checks.
- [Reviewer Role](roles/reviewer.md) inspects the actual changes without editing
  files and returns findings.

The worker receives the original goal and, after the first iteration, the
previous validator feedback. The validator receives the goal and worker result.
It escalates when the goal is satisfied, ending the Loop successfully; otherwise
its feedback drives the next worker visit. Unrecoverable validation conditions
fail the run. The Loop fails if it exhausts five iterations without acceptance.
Each Role visit starts a fresh provider session; feedback travels through the
root run's shared state.

## Validate, inspect, and run

From the repository root:

```bash
callee agent validate examples/goalkeeper/workflows/goalkeeper.md
callee agent validate examples/goalkeeper/roles/implementer.md
callee agent validate examples/goalkeeper/roles/reviewer.md
callee --agent-root examples agent view goalkeeper/workflows/goalkeeper
callee --agent-root examples agent run goalkeeper/workflows/goalkeeper \
  --message "Implement the requested change and verify it meets the goal."
```

Resource references use the `goalkeeper` namespace, so use `examples` as the
catalog root when inspecting or running the checked-in pack.

## Import the pack

Import only this pack into the current project's `.callee` catalog:

```bash
callee agent import baldaworks/callee \
  --path examples/goalkeeper \
  --prefix goalkeeper
callee agent view goalkeeper/workflows/goalkeeper
callee agent run goalkeeper/workflows/goalkeeper \
  --message "Implement the requested change and verify it meets the goal."
```

Keep the `goalkeeper` prefix to match the workflow's bundled Role references.
The README is documentation and is skipped during resource discovery and import.
The pack requires no shared example Roles.

Both Roles use Codex. Running them requires an authenticated Codex installation
and a controlling terminal. The reviewer retains `ask` permission mode.
