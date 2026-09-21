# PromptKit Role generation

Callee embeds a pinned PromptKit catalog and can turn one selected template into
a versioned Callee Role.

## Find a template

```bash
callee promptkit list --type template
callee promptkit search "review Go code" --type template
callee promptkit show review-code --json
```

Use `show` to inspect the template's declared parameters and metadata before
generation.

## Create a Role

```bash
callee promptkit role create go-reviewer \
  --template review-code \
  --description "Reviews Go code" \
  --provider codex \
  --prompt-param code \
  --bind language=Go
```

The generated Role uses the current `v1alpha1` envelope and Go templates. The
parameter selected by `--prompt-param` receives runtime input. `--bind` and
`--bind-file` freeze author-time values; remaining declared template parameters
become runtime `spec.params`. Supply a configurable persona with `--persona`.

Use `--cmd`, `--model`, `--reasoning`, `--mode`, and repeatable `--extra-arg`
for provider session configuration. `--protocol`, `--taxonomy`, `--format`, or
`--no-format` adjust template assembly.

Templates marked with `metadata.mode: interactive` automatically generate
`spec.interactive: true`. `--interactive` forces the same author-time behavior
for another template. This is distinct from the runtime `agent run
--interactive=true|false` override.

## Choose the output

Without `--output`, the command writes `.callee/roles/go-reviewer.md`, or
`<agent-root>/roles/go-reviewer.md` when global `--agent-root` is set. It creates
parent directories and refuses to replace an existing file unless `--force` is
set. Use `--dry-run` to print the resource without writing it.

Validate both the physical resource and its resolved tree:

```bash
callee agent validate .callee/roles/go-reviewer.md
callee agent view roles/go-reviewer
```

See [Agent resources](../reference/agent-resources.md#role) for Role fields and
[Running agents](running-agents.md) for runtime parameters and interactive
execution.
