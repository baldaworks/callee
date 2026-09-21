---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: >
    Maintains the root README.md as a concise product landing page through
    iterative writing and independent review.
  children:
    - ref: roles/technical-writer
      alias: readme_writer
      input: |
        Maintain README.md as the product landing page for Callee.

        Requested focus:
        {{ .Input }}

        Treat the following as the permanent README quality contract. Verify
        every claim against the current CLI help, schema, implementation,
        tests, checked-in examples, and canonical docs before changing it:

        - Address all verified README audit findings relevant to the request.
        - Lead with the problem Callee solves, the product value, and a compact
          example before installation or integration details.
        - Explain that Callee turns repeatable agent work into versioned,
          repository-defined Markdown or YAML resources that are validated as
          one graph and return one final artifact.
        - Explicitly name all nine public `callee.metalagman.dev/v1alpha1`
          kinds. Group and distinguish `Role`, `Script`, and `Human` leaves;
          `TypeSafeJev` and `OpenRouterDecision` typed evaluators; and
          `Sequential`, `Parallel`, `Loop`, and `Router` composites. Link to
          the dedicated pages below `docs/agent-kinds/` instead of duplicating
          their full reference contracts.
        - Keep one concise workflow example that demonstrates composition and
          makes the repository-authored graph concrete.
        - Keep setup and provider prerequisites secondary to the product
          explanation. Give one short executable path, then link to the
          installation, quickstart, integration, and reference docs for detail.
        - Treat npm/npx as the primary distribution path. Mention alternative
          installation only when it materially helps the requested focus.
        - Use `coding agent` for Codex, Claude Code, Grok Build, Copilot CLI,
          OpenCode, and Cursor. Use `ACP provider` only for a Role runtime.
          Never describe Callee as exclusively ACP-backed.
        - Keep README user-facing. Do not add contributor setup, exhaustive CLI
          or schema reference, full provider matrices, manual integration
          inventories, internal architecture detail, or release procedure.
        - Preserve useful badges, the License and notices section, and links to
          the canonical docs. Keep every command executable as written.
        - Do not add Gemini support or describe unsupported server, thread
          store, or handle-binding behavior.

        Prefer a short, scannable landing page over completeness. Put durable
        detail in docs/ and preserve useful existing structure unless changing
        it materially improves the product story or requested focus.

        Modify only README.md. Return your writing report normally; only the
        reviewer controls completion of this loop.

        {{ with index .State.outputs "readme_reviewer" }}
        Previous review feedback:
        {{ . }}

        Address every material finding before returning the updated README
        outcome.
        {{ end }}
    - ref: roles/technical-writer
      alias: readme_reviewer
      canEscalate: true
      input: |
        README maintenance goal:
        {{ .Input }}

        Writer report:
        {{ index .State.outputs "readme_writer" }}

        This is an independent read-only review. Do not modify files. Inspect
        README.md and the authoritative CLI help, schema, implementation,
        tests, examples, and canonical docs. Verify that the requested focus
        and permanent README contract are satisfied:
        the selling wedge leads; all nine public kinds are explicit and
        correctly grouped; the workflow example supports the product story;
        setup remains secondary and executable; durable detail links into
        docs/; coding-agent and ACP-provider terminology is accurate; the page
        contains no contributor or exhaustive reference material; and
        unsupported Gemini, server, thread-store, and handle-binding behavior
        is absent.

        If README.md is accurate, complete, well structured, and within scope,
        return concise approval with evidence and escalate to finish the loop.
        Otherwise return actionable findings normally. Reserve
        fail for unrecoverable review conditions. Do not escalate on an
        incomplete or uncertain result.
  maxIterations: 5
  onExhausted: fail
  output: |
    README workflow finished:
    {{ index .State.outputs "readme_reviewer" }}
---
{{ .Input }}
