# Callee agent guidance

Use CLI help for the command surface, the published schema for resource syntax,
and the README for public behavior. This file records only project decisions
that are not safely recoverable from those sources.

## Product guardrails

- Treat the Workflows API as a clean break. Do not add compatibility aliases for
  `callee exec`, `callee role`, selector-based `callee agent --role`, thread
  flags, or unversioned role resources.
- Do not add `Parallel` workflows, Gemini support, a server transport, a Callee
  thread store, or handle binding without an explicit product decision.
- Each root run owns one shared state object. Each Role visit uses a fresh
  provider session.
- Use Norma Runtime for ACP process logic rather than duplicating it.

## Quality gate

- Review the complete changed Go diff once, run `gofmt` on every changed Go
  file, then run `go test ./...`, `go test -race ./...`, and the configured
  linter once. Include relevant plugin validation when plugin assets change.

## Release cycle

Releases are tag-triggered. Run the local quality gate, commit and push to
`main` without a tag, and wait for every required GitHub Actions run on that
exact commit. Only then create and push the annotated `vX.Y.Z` tag. Watch the
release workflow and verify that the tag points to the checked commit. Never
publish manually, tag before the remote gate passes, or retag an unchecked
commit.

## Project tracking

Use Beads for durable task tracking and follow
`.agents/skills/beads/SKILL.md`; keep Beads CLI guidance in that skill.
