# Callee documentation

Callee turns repeatable agent work into repository-defined, statically
validated workflows authored as versioned Markdown or YAML. A workflow can mix
model-backed Roles, local Scripts, Human decisions, typed evaluations, and
deterministic control flow. Start with a first run, then use the task-oriented
guides and precise references as needed.

## Get started

- [Installation](getting-started/installation.md) covers prerequisites, coding-agent setup, and direct CLI installation.
- [Quickstart](getting-started/quickstart.md) takes the installed starter workflow from discovery to a completed run.

## Guides

- [Coding-agent integrations](guides/coding-agent-integrations.md) explains the six supported coding agents, installed assets, and manual setup.
- [Running agents](guides/running-agents.md) covers run modes, parameters, permissions, terminal behavior, output, and failures.
- [Importing agents](guides/importing-agents.md) copies and validates a catalog subtree from a remote git repository.
- [PromptKit](guides/promptkit.md) discovers templates and generates validated Roles.
- [ACP provider configuration](guides/acp-providers.md) configures Role backends, sessions, timeouts, and readiness checks.
- [ACP permission requests](guides/acp-permissions.md) defines interactive and automatic permission handling.

## Agent kinds

Start with [Choose an agent kind](agent-kinds/index.md) for a compact comparison,
then use the page for the resource you are authoring:

- Leaf kinds: [Role](agent-kinds/role.md), [Script](agent-kinds/script.md), and [Human](agent-kinds/human.md).
- Typed evaluators: [TypeSafeJev](agent-kinds/typesafe-jev.md) and [OpenRouterDecision](agent-kinds/openrouter-decision.md).
- Composite workflow kinds: [Sequential](agent-kinds/sequential.md), [Parallel](agent-kinds/parallel.md), [Loop](agent-kinds/loop.md), and [Router](agent-kinds/router.md).

## Concepts and reference

- [Architecture](concepts/architecture.md) explains discovery, graph compilation, shared state, kind-specific execution, and Role process/session ownership.
- [CLI reference](reference/cli.md) maps the public commands and inspection surfaces.
- [Agent resources](reference/agent-resources.md) defines discovery, the versioned envelope, every kind, templates, and validation.
- [Workflow semantics](reference/workflow-semantics.md) defines node data flow, composition, escalation, control records, and cleanup.
- [Execution metrics](reference/execution-metrics.md) defines lifecycle measurement fields and aggregation boundaries.
- [Examples](examples/index.md) indexes runnable packs and individual resources.

## Project and contributor material

The following pages describe how Callee is built and maintained rather than how
to use it:

- [Development and validation](contributing/development.md)
- [Release process](contributing/release.md)
- [OpenAI Build Week](project/build-week.md)
- [Architecture decision records](adr/)

Older repository links remain available through the [moved CLI guide](guides/cli.md)
and [renamed coding-agent integration page](guides/coding-host-integrations.md).

The checked-in [JSON Schema](../internal/agent/schema.json), CLI help,
implementation, and tests are authoritative for current behavior. The public
resource API is `callee.metalagman.dev/v1alpha1`; removed legacy commands and
unversioned resources are not compatibility surfaces.
