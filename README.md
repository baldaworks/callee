# Callee

[![Test](https://github.com/baldaworks/callee/actions/workflows/test.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/test.yml)
[![Lint](https://github.com/baldaworks/callee/actions/workflows/lint.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/lint.yml)
[![Security](https://github.com/baldaworks/callee/actions/workflows/security.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/security.yml)
[![Latest release](https://img.shields.io/github/v/release/baldaworks/callee)](https://github.com/baldaworks/callee/releases/latest)
[![npm version](https://img.shields.io/npm/v/%40baldaworks%2Fcallee)](https://www.npmjs.com/package/@baldaworks/callee)
[![License: MIT](https://img.shields.io/github/license/baldaworks/callee)](LICENSE)

## Put repeatable agent work in the repository

Useful agent workflows quickly outgrow a chat prompt. They need several roles,
local checks, approval points, routing, bounded iteration, and shared context.
When that logic lives in one conversation, it is difficult to review,
reproduce, or improve as a team.

Callee turns that logic into versioned Markdown or YAML resources. Compose AI
roles, shell scripts, human decisions, and typed model judgments with
sequential, parallel, loop, and router control flow. Callee validates the whole
graph before execution, carries one explicit state object through the run, and
returns one final artifact.

- **Review the workflow like code.** Prompts, models, permissions, inputs, and
  control flow live beside the project that uses them.
- **Use the right kind of step.** A workflow can call a coding model, run a
  deterministic check, ask a person, or request a typed decision from TypeSafe
  or OpenRouter.
- **Run it anywhere.** Invoke the same resource from a terminal or through
  Codex, Claude Code, Grok Build, Copilot CLI, OpenCode, or Cursor.

## Supported agent kinds

All ten public `callee.metalagman.dev/v1alpha1` kinds fit into three groups.
See [Choose an agent kind](docs/agent-kinds/index.md) for the complete chooser.

- **Leaves:** [Role](docs/agent-kinds/role.md) gives a coding agent a task with
  static provider configuration; [DynamicRole](docs/agent-kinds/dynamic-role.md)
  renders provider configuration from visit-time state;
  [Script](docs/agent-kinds/script.md) runs a deterministic local shell step;
  [Human](docs/agent-kinds/human.md) pauses for one operator response.
- **Typed evaluators:** [TypeSafeJev](docs/agent-kinds/typesafe-jev.md) is the
  native TypeSafe System One/Jev evaluator;
  [OpenRouterDecision](docs/agent-kinds/openrouter-decision.md) uses the
  OpenRouter Decisions API with a selected model and is not Jev-only.
- **Composite workflows:** [Sequential](docs/agent-kinds/sequential.md) orders
  children; [Parallel](docs/agent-kinds/parallel.md) fans out and joins
  unattended work; [Loop](docs/agent-kinds/loop.md) repeats children within a
  bound; [Router](docs/agent-kinds/router.md) deterministically selects one
  branch.

## A workflow is a small, inspectable graph

This workflow runs several reviewers concurrently, then gives their combined
output to an architect:

```yaml
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Collects independent reviews, then produces one plan.
  body: "{{ .Input }}"
  children:
    - ref: workflows/parallel-review
      alias: reviews
    - ref: roles/architect
      alias: architect
      input: |
        Task:
        {{ .Input }}

        Reviews:
        {{ .State.outputs.reviews }}
  output: "{{ .State.outputs.architect }}"
```

References resolve before the run starts. Each Role or DynamicRole visit gets a
fresh model session; Scripts run local commands; Human nodes collect an operator response;
typed evaluation nodes call their own HTTP APIs. Composite nodes only control
data flow and execution order, so a child can be any resource kind.

## Try it

Install six editable starter resources and the integration for your coding
agent:

```bash
npx --yes @baldaworks/callee@latest setup codex
```

Replace `codex` with `claude`, `grok`, `copilot`, `opencode`, or `cursor` as
needed. Then inspect and run a workflow directly:

```bash
npx --yes @baldaworks/callee@latest agent view workflows/investigate
npx --yes @baldaworks/callee@latest agent run workflows/investigate \
  --message "Explain this project's architecture and main entry points"
```

Or ask your coding agent to run it. In Codex:

```text
$callee Run workflows/investigate to explain this project's architecture and main entry points.
```

Provider executables and credentials are separate runtime prerequisites for
Roles and DynamicRoles. Scripts and Human nodes need no model provider; TypeSafeJev and
OpenRouterDecision use their respective HTTP APIs. See the
[Quickstart](docs/getting-started/quickstart.md) for the complete first run and
[Installation](docs/getting-started/installation.md) for CLI-only and other
coding-agent setup options.

## Explore

- [Run agents](docs/guides/running-agents.md)
- [Browse runnable examples](docs/examples/index.md)
- [Author agent resources](docs/reference/agent-resources.md)
- [Understand workflow semantics](docs/reference/workflow-semantics.md)
- [Set up coding-agent integrations](docs/guides/coding-agent-integrations.md)
- [Read the documentation hub](docs/index.md)

## License and notices

Callee is released under the [MIT License](LICENSE). See
[Third-party notices](THIRD_PARTY_NOTICES.md) for embedded and statically linked
dependencies.
