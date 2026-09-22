# Examples

Use the checked-in examples to inspect valid resources or run a complete
workflow without inventing a schema from scratch. Examples use the current
`callee.metalagman.dev/v1alpha1` API.

## Runnable packs

| Example | What it demonstrates |
| --- | --- |
| [Codex Sol + Luna](../../examples/codex/sol-luna/README.md) | A plan-then-implement workflow with explicit model and reasoning choices. |
| [Jev model selector](../../examples/codex/jev-model-selector/README.md) | A self-contained TypeSafe Jev → DynamicRole pack with visit-time Codex model selection. |
| [OpenCode rejudge](../../examples/opencode/rejudge/README.md) | A repository-authored multi-reviewer panel and diff-focused variant. |
| [Typed evaluations](../../examples/evaluations/README.md) | Direct TypeSafe and OpenRouter judgments plus a two-step TypeSafe Sequential workflow. |

## Individual resources

- [Reviewer Role](../../examples/roles/reviewer.md)
- [Dynamic reviewer](../../examples/roles/dynamic-reviewer.md)
- [Investigate Sequential](../../examples/workflows/investigate.md)
- [GoalKeeper Loop](../../examples/workflows/goalkeeper.md)
- [Parallel review](../../examples/workflows/parallel-review.md)
- [Task Router](../../examples/workflows/task-router.md)
- [Routed task composition](../../examples/workflows/routed-task.md)

Validate one physical file with `callee agent validate <path>`. To resolve a
complete tree from the shared example catalog, use the examples directory as an
exclusive root:

```bash
callee --agent-root examples agent view workflows/investigate
```

Examples may require the provider executable and credentials selected by their
Roles. Typed evaluation examples document their separate service credentials.
See [Agent resources](../reference/agent-resources.md) for field definitions and
[Workflow semantics](../reference/workflow-semantics.md) for runtime behavior.
