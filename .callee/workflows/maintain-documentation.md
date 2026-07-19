---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: >
    Maintains structured project documentation below docs/ through iterative
    writing and independent review, ready for later publication as a
    Markdown-based GitHub Pages site.
  children:
    - ref: roles/technical-writer
      alias: writer
      input: |
        Documentation goal:
        {{ .Input }}

        This is the writing phase. Inspect the relevant repository sources and
        maintain the project's canonical long-form documentation exclusively
        below docs/. Do not modify README.md, product code, release files, or
        site-generator configuration. Create docs/ when it does not exist.

        Build and preserve a coherent information architecture:

        - use docs/index.md as the documentation landing page and navigation
          hub;
        - organize durable topics into clearly named Markdown pages and
          subdirectories, using lowercase kebab-case paths;
        - make every maintained page discoverable from docs/index.md or a
          linked section index, without orphan pages;
        - use relative repository-safe links and keep documentation assets
          below docs/assets/;
        - write portable CommonMark/GitHub Flavored Markdown with one clear H1,
          stable headings, language-tagged code fences, and no unnecessary raw
          HTML;
        - keep the content ready for later GitHub Pages conversion without
          committing to a theme or generator. Do not add layouts, Liquid,
          plugins, _config.yml, or deployment workflows unless the goal
          explicitly requests that migration;
        - update navigation and validate internal links whenever pages move or
          new pages are added.

        Preserve verified project behavior, avoid duplicating README onboarding,
        and treat docs/ as the canonical home for detailed project guides and
        reference material.

        Return your writing report normally. Do not escalate from this phase;
        only the reviewer controls completion of the documentation loop.

        {{ with index .State.outputs "reviewer" }}
        Previous review feedback:
        {{ . }}

        Address every material finding before returning the updated writing
        outcome.
        {{ end }}
    - ref: roles/technical-writer
      alias: reviewer
      input: |
        Documentation goal:
        {{ .Input }}

        Writer report:
        {{ index .State.outputs "writer" }}

        This is an independent read-only review phase. Do not modify files.
        Inspect the actual documentation and authoritative repository sources,
        not only the writer report. Verify that all documentation mutations are
        confined to docs/, the goal is fully addressed, factual claims and
        examples are supported, commands are current, and docs/index.md exposes
        a coherent navigation path to every maintained page. Check relative
        links, assets, filename consistency, heading structure, portable
        Markdown, and readiness for a later Markdown-based GitHub Pages site
        without premature generator-specific configuration. Reject orphaned,
        duplicated, misplaced, or structurally ambiguous content.

        If the documentation satisfies the goal, return concise approval with
        the evidence and checks used, then escalate to finish the loop. If it
        does not, return actionable findings normally so the next writing phase
        can address them. Do not escalate on an incomplete or uncertain result.
  maxIterations: 5
  onExhausted: fail
  output: |
    Project documentation workflow finished:
    {{ index .State.outputs "reviewer" }}
---
{{ .Input }}
