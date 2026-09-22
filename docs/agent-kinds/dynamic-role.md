# DynamicRole

Use a DynamicRole when a coding agent needs ordinary Role behavior but its ACP
provider configuration depends on state available only when the node is
visited. Keep using [Role](role.md) when the provider is known while the catalog
is loaded: static Roles can be checked and their provider processes can be
started before workflow execution.

## Minimal resource

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: DynamicRole
spec:
  description: Reviews with a provider selected from workflow state.
  provider:
    type: '{{ .State.provider }}'
    model: '{{ .State.model }}'
    reasoning: '{{ .State.reasoning }}'
    timeout: '{{ .State.timeout }}'
  state:
    provider: codex
    model: gpt-5.6-sol
    reasoning: high
    timeout: 20m
---
Review this change and return verified findings:

{{ .Input }}
```

Every provider field is a template: `type`, `cmd`, `model`, `reasoning`,
`mode`, `timeout`, and each `extraArgs` entry. Templates may read `.Prompt`,
the current `.Input`, one coherent post-node-state `.State` snapshot, and
resolved `.Params`. They cannot read `.Output` or use environment, filesystem,
network, clock, randomness, cryptography, or mutation helpers.

## Visit-time behavior

Callee resolves parameters and applies the node's state modifier before it
renders the complete provider configuration once. It validates the effective
provider before process lookup or startup. The effective configuration remains
fixed for the visit, including all REPL turns; a later Loop visit renders again
from the state visible then. Matching effective provider identities reuse a
process, but every visit still creates a fresh session.

Rendering or effective-provider validation errors fail the visit before a
provider starts. `cmd` remains one executable name and `extraArgs` remains an
ordered argument vector; Callee never parses either as a shell command.

## Inspection and security

`callee agent view` marks a DynamicRole with `provider=runtime`; JSON view keeps
the authored templates. Plain `callee doctor` validates the resource, template,
and graph but reports provider readiness as deferred because runtime state is
not available. The selected executable is checked only when the node is
reached.

Do not route untrusted model-produced state directly into `cmd` or
`extraArgs`. Constrain those values through authored template branches or use a
static Role when process selection does not genuinely need runtime state.

Run the checked-in
[dynamic reviewer example](../../examples/roles/dynamic-reviewer.md), or the
[self-contained Sequential Jev model-selector pack](../../examples/codex/jev-model-selector/README.md)
that feeds a validated Choice answer into its provider model. See
[DynamicRole fields](../reference/agent-resources.md#dynamicrole),
[DynamicRole execution](../reference/workflow-semantics.md#dynamicrole-execution),
and [ACP provider configuration](../guides/acp-providers.md).
