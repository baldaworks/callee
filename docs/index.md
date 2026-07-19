# Callee engineering documentation

Callee is a CLI runtime for provider-backed agents and deterministic workflows defined as versioned Markdown or YAML resources. This documentation is the canonical long-form reference for engineers who author agents, operate the CLI, integrate coding hosts, or maintain the project. For the shortest installation and first-run path, start with the [repository README](../README.md).

## Understand the system

- [Concepts and architecture](concepts/architecture.md) explains the resource, registry, runtime, state, and ACP process model.
- [Agent resource format](reference/agent-resources.md) defines discovery, the versioned envelope, kind-specific fields, templates, parameters, and validation rules.
- [Workflow semantics](reference/workflow-semantics.md) defines node input and output, state updates, composition, edge-authorized Loop escalation, failure, REPL control, and lifecycle behavior.

## Install and operate Callee

- [CLI installation and usage](guides/cli.md) covers installation choices, catalog inspection, validation, execution, graph inspection, diagnostics, and PromptKit role generation.
- [ACP provider configuration](guides/acp-providers.md) covers supported providers, command resolution, session settings, timeouts, permissions, and troubleshooting.
- [ACP permission requests](guides/acp-permissions.md) defines Role permission modes, automatic ACP option selection, controlling-TTY interaction, failures, and timeout behavior.
- [Coding-host integrations](guides/coding-host-integrations.md) explains the installed skills, setup targets, generated project files, and the boundary between a coding host and a runtime provider.

## Maintain the project

- [Development and validation](contributing/development.md) covers the repository layout, toolchain, local checks, focused validation, and documentation maintenance.
- [Release process](contributing/release.md) records the tag-triggered release sequence, versioned surfaces, remote quality gate, artifact publication, and post-release verification.

## Scope and compatibility

The current resource API is `callee.metalagman.dev/v1alpha1`. It supports `Role`, `Sequential`, and `Loop`. Callee is CLI-only and intentionally has no `Parallel` kind, Gemini provider, server transport, durable thread store, or handle binding. Treat the Workflows API as a clean break: removed legacy command and resource forms are not compatibility surfaces.

The checked-in [JSON Schema](../internal/agent/schema.json) is the machine-readable resource contract. Source, tests, CLI help, manifests, and repository instructions remain authoritative when implementation behavior changes.
