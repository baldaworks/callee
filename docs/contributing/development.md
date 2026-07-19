# Development and validation

Use this guide when changing Callee itself. Repository instructions in [`AGENTS.md`](../../AGENTS.md) are authoritative for product guardrails, quality gates, and release sequencing.

## Prerequisites

Install the Go version declared by [`go.mod`](../../go.mod), currently Go 1.26.5. The module pins the linter and vulnerability scanner as Go tools, so separate global installations are not required for the standard checks.

Some tests and integrations also exercise platform or external boundaries:

- PTY integration tests run on Linux and macOS;
- host setup commands require the corresponding external host CLI only when invoked for real;
- provider readiness checks require provider executables and credentials, but unit tests use controlled fakes.

## Repository layout

| Path | Purpose |
| --- | --- |
| [`cmd/callee`](../../cmd/callee) | Process entry point and signal handling. |
| [`internal/agent`](../../internal/agent) | Resource schema, codec, semantic checks, and template engine. |
| [`internal/registry`](../../internal/registry) | Discovery and static graph resolution. |
| [`internal/workflow`](../../internal/workflow) | Runtime execution and control protocol. |
| [`internal/runtime`](../../internal/runtime) | Norma Runtime and ACP adaptation. |
| [`internal/cli`](../../internal/cli) | CLI commands, setup assets, TTY interaction, and PromptKit generation. |
| [`internal/doctor`](../../internal/doctor) | Provider checks and graph renderers. |
| [`examples`](../../examples) | Runnable resources; starter assets are expected to match the applicable examples. |
| [`plugins/callee`](../../plugins/callee) | Multi-host plugin manifests and skill content. |
| [`docs`](..) | Canonical long-form engineering documentation. |

## Product guardrails

The Workflows API is a clean break. Do not restore legacy `exec` or `role` commands, selector-based agent roles, thread flags, or unversioned Role resources. Do not add `Parallel`, Gemini, a server transport, a Callee thread store, handle binding, or duplicated ACP process logic without an explicit product decision.

One root run owns one state object. Every Role visit owns a fresh provider session. Norma Runtime remains the ACP process layer.

## Focused development loop

Start with the package nearest the change:

```bash
go test ./internal/agent
go test ./internal/registry
go test ./internal/workflow
go test ./internal/runtime
go test ./internal/cli
```

Useful behavior-specific checks include:

```bash
# Inspect the public command surface built from current source.
go run ./cmd/callee --help
go run ./cmd/callee agent run --help

# Validate checked-in examples through the CLI.
go run ./cmd/callee agent validate examples/roles/reviewer.md
go run ./cmd/callee agent validate examples/workflows/goalkeeper.md

# Run plugin manifest, skill, version, and example consistency tests.
go test ./plugins/callee
```

`agent validate` checks one resource and intentionally does not resolve example child references. The test suite builds a complete registry from checked-in examples and verifies starter assets against them.

## Required quality gate

Before handing off a code change:

1. Review the complete changed Go diff.
2. Run `gofmt` on every changed Go file.
3. Run the full tests, race detector, and configured linter once.
4. Include relevant plugin validation when plugin assets changed.

```bash
gofmt -w <changed-go-files>
go test ./...
go test -race ./...
go tool golangci-lint run ./...
```

The configured CI also runs:

```bash
go tool govulncheck ./...
```

The test, lint, and security workflows run for pull requests and pushes to `main`. Tag-triggered release workflows repeat the full test, race, lint, and vulnerability gate.

## Resource and schema changes

When resource behavior changes, update the schema, semantic validation, codecs, tests, examples, skills, and documentation as applicable. [`internal/agent/schema_test.go`](../../internal/agent/schema_test.go) verifies that the checked-in JSON Schema exactly matches the bytes embedded in the executable.

Keep examples executable and use the current `v1alpha1` envelope. Unknown fields must remain rejected. Add negative tests for removed or unsupported syntax when a regression could silently reopen a compatibility path.

## Plugin and setup changes

Plugin tests verify:

- supported hosts expose only the intended create/run skills;
- duplicated skill variants and workflow references remain synchronized;
- plugin and marketplace manifests carry the release version and public metadata;
- OpenCode command wrappers load the matching skill;
- setup preserves existing files unless forced;
- starter resources match checked-in examples and form a valid registry.

Run at least `go test ./plugins/callee ./internal/cli` when these assets change, followed by the complete quality gate.

## Documentation changes

Maintain durable long-form material exclusively below `docs/`. Keep [`docs/index.md`](../index.md) as the navigation hub, use lowercase kebab-case paths, and link to repository sources with portable relative links. Do not add site-generator frontmatter, layouts, Liquid, themes, plugins, `_config.yml`, or deployment workflows unless a separate migration explicitly requires them.

For documentation-only changes:

- compare commands against `go run ./cmd/callee ... --help`;
- compare resource fields against the checked-in schema and semantic validators;
- validate any complete resource examples;
- check every local Markdown link and anchor;
- review nearby pages for duplicated or contradictory guidance.

No Go formatting is necessary when no Go file changed, but the relevant repository tests should still be run in proportion to the claims affected.
