---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: >
    Maintains the root README.md through iterative writing and independent
    review, with explicit coverage of Callee host plugins and onboarding
    behavior.
  children:
    - ref: roles/technical-writer
      alias: readme_writer
      input: |
        Maintain README.md as release-quality onboarding and reference
        documentation for Callee.

        Requested focus:
        {{ .Input }}

        Treat the following as a permanent README quality contract. Verify
        every claim against the current CLI help, implementation, tests,
        manifests, and installed assets before changing the README:

        - Address all verified README audit findings relevant to the request.
        - Clearly distinguish runtime ACP providers from host plugins and
          integrations; do not present them as the same concept.
        - Follow a user-centered host-plugin onboarding structure similar to
          PromptKitty: first introduce the installed Run Agent and Create Agent
          skills and the outcome of each; then show one-shot setup; then give a
          compact table mapping each host to its setup command and the exact
          invocation or skill name installed for both capabilities.
        - Cover Codex, Claude Code, Grok Build, Copilot CLI, OpenCode, and
          Cursor in that table. Verify every invocation from current plugin
          manifests and embedded assets.
        - For OpenCode, do not conflate skills with command wrappers. The
          installed skills are `callee-run-agent` and `callee-create-agent`;
          `/callee` and `/callee-create-agent` are convenience commands that
          load those skills.
        - Follow the table with only a concise explanation of marketplace-based
          installation versus file-based OpenCode and Cursor assets, preservation
          of existing files, `--force`, and external host credentials.
        - Put optional manual integration instructions in a later `Manual host
          setup` section. State that automated `npx ... setup` is recommended,
          distinguish integration-only manual steps from starter-agent
          installation, show exact marketplace commands and source-to-target
          mappings, and omit internal rollback or partial-failure narration.
        - Show concrete host usage immediately after setup. For Codex, prefer
          the plugin-level `$callee <request>` entrypoint and explain that it
          routes to the appropriate installed skill without requiring a
          `:run-agent` or `:create-agent` suffix; keep the explicit selectors in
          the host table. Include examples for running an existing workflow,
          creating a Role with explicit provider, model, and reasoning, and
          creating a Loop from two named existing agents with a clear iteration
          limit and completion condition. Present direct CLI installation and
          CLI quick start as a separate path after host onboarding instead of
          mixing both paths together.
        - Treat the npm distribution as the primary installation and usage
          path. Use `npm install --global @baldaworks/callee@latest` for repeated
          CLI use and `npx --yes @baldaworks/callee@latest ...` for one-shot
          setup and commands. Show complete `npx` setup commands in the host
          table so each row is executable as written. Keep `go install` only as
          a clearly secondary alternative, not the default onboarding path.
        - Ensure installation and Quick Start commands are executable exactly
          as written and do not assume files or shell commands that setup does
          not create.
        - Keep provider prerequisites, supported provider types, nested
          configuration fields, and provider examples complete and current.
        - Document relevant REPL activation and lifecycle constraints,
          PromptKit behavior, runtime parameter examples, and agent discovery
          roots when those topics appear in the README.
        - Do not add Gemini support or describe unsupported server, thread
          store, or handle-binding behavior.
        - Keep the OpenAI Build Week section as the final README section.

        Prefer precise corrections over speculative expansion. Preserve useful
        existing structure unless changing it materially improves onboarding.

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
        README.md and the authoritative CLI help, implementation, tests,
        manifests, and installed assets. Verify that the requested focus and
        permanent README contract are satisfied: npm/npx is the primary path;
        runtime ACP providers remain distinct from host plugins; Codex, Claude
        Code, Grok Build, Copilot CLI, OpenCode, and Cursor setup and invocation
        are accurate; examples are executable; unsupported Gemini, server,
        thread-store, and handle-binding behavior is absent; and OpenAI Build
        Week remains the final section.

        If README.md is accurate, complete, well structured, and within scope,
        return concise approval with evidence and escalate to finish the loop.
        Otherwise return actionable findings normally. Do not escalate on an
        incomplete or uncertain result.
  maxIterations: 5
  onExhausted: fail
  output: |
    README workflow finished:
    {{ index .State.outputs "readme_reviewer" }}
---
{{ .Input }}
