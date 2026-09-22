# Choose an agent kind

Choose the smallest kind that owns the behavior you need. Leaves do work;
typed evaluators make structured judgments; composites arrange other resources.
All ten names below are public `callee.metalagman.dev/v1alpha1` kinds.

| Group | Kind | Choose it when | Natural artifact |
| --- | --- | --- | --- |
| Leaf | [Role](role.md) | A coding agent must reason, use tools, or edit files through ACP. | The Role's final text. |
| Leaf | [DynamicRole](dynamic-role.md) | Role behavior is needed, but provider configuration depends on visit-time state. | The DynamicRole's final text. |
| Leaf | [Script](script.md) | A local shell command should perform a deterministic check or step. | A compact exit summary. |
| Leaf | [Human](human.md) | The workflow must stop for one terminal response. | The response text. |
| Typed evaluator | [TypeSafeJev](typesafe-jev.md) | TypeSafe System One should answer typed questions with a Jev model. | Compact validated JSON. |
| Typed evaluator | [OpenRouterDecision](openrouter-decision.md) | OpenRouter's Decisions endpoint should answer typed questions with an explicitly selected model. | Compact validated JSON. |
| Composite | [Sequential](sequential.md) | Children must run once in order and naturally pass artifacts forward. | The last child artifact. |
| Composite | [Parallel](parallel.md) | Independent, unattended branches should fan out and then join. | Authored-order JSON of child artifacts. |
| Composite | [Loop](loop.md) | Ordered work must repeat within a fixed bound until an authorized Role escalates. | The escalated or last child artifact. |
| Composite | [Router](router.md) | An authored template can deterministically select exactly one named or default branch. | The selected child artifact. |

Prefer Role when provider configuration is static and use DynamicRole only when
runtime state must select it. Do not use either merely to obtain a typed judgment: `TypeSafeJev` and
`OpenRouterDecision` are native HTTP leaves, not ACP Roles. Do not use Router
for model-selected routing: route selection is deterministic. Composites may
reference any supported kind, subject to kind-specific restrictions such as
the prohibition on `Human` anywhere below `Parallel`.

For fields shared across kinds, child occurrence syntax, templates, and state,
use [Agent resource format](../reference/agent-resources.md). For exact data
flow, failures, and control records, use
[Workflow semantics](../reference/workflow-semantics.md).

## Inspect before running

```bash
callee agent list --kind Sequential
callee agent schema Sequential
callee agent validate .callee/workflows/review.md
callee agent view workflows/review --json
callee agent run workflows/review --message "Review the current change"
```

`validate` checks one file; `view` resolves its complete tree. `callee doctor`
checks static Role executable readiness and reports DynamicRole providers as
deferred until runtime.
