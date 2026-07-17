---
name: create-role
description: Create a project-defined versioned Callee Markdown Role from an embedded PromptKit template. Use when the user asks to generate, scaffold, or author a new Callee role.
---

# Create a Callee Role

Use `callee` when available. Otherwise use the pinned fallback `npx --yes @baldaworks/callee@0.10.0` for every command in the task.

Use the embedded PromptKit catalog and generate the Role with `callee promptkit role create`. Leave intended runtime values unbound so they become `spec.params`.

Generated resources use this envelope:

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: <capability description>
  provider:
    type: <codex|claude|opencode|copilot|grok|generic_acp>
  params:
    focus: What the role should focus on
---

Task:
{{ .Input }}

Focus:
{{ .Params.focus }}
```

Provider configuration stays under `spec.provider`. The Markdown body is canonical `spec.body` and must not also appear in frontmatter. A Role body contains exactly one unconditional bare `{{ .Prompt }}` or `{{ .Input }}` insertion. All template surfaces use Go `text/template`; runtime parameters are read from `.Params`.

Write project resources below `.callee/`; `roles/` is an optional ID namespace, not a kind discriminator. Callee also loads complete YAML objects from lowercase `.yaml` and `.yml`, but this generator keeps Markdown as the base format. The resource ID is its path relative to `.callee` without the final supported extension.

After creation, validate the single file, then resolve its graph and display required runtime parameters:

```bash
callee agent validate ".callee/roles/<resource-id>.md"
callee agent view "<resource-id>" --json
```

Do not add Gemini or legacy flat provider fields.
