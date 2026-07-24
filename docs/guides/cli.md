# CLI installation and usage

Use the CLI directly when you want reproducible catalog inspection, validation, graph checks, or workflow execution outside a coding-host skill. The [README quick start](../../README.md#npm-cli-installation-and-quick-start) remains the shortest onboarding path; this guide explains the command surface and operational details.

## Install the executable

### npm launcher

Node.js with npm is required for the npm paths. Install the launcher globally for repeated shell use:

```bash
npm install --global @baldaworks/callee@latest
callee --version
```

For a one-shot command without a global Callee installation:

```bash
npx --yes @baldaworks/callee@latest --version
```

The npm launcher selects a CGO-disabled native executable for macOS AMD64/ARM64, Linux AMD64/ARM64, or Windows AMD64.

### Go installation

Install from source with the Go version declared in [`go.mod`](../../go.mod):

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
callee --version
```

Ensure the Go installation directory, normally `$GOBIN` or `$GOPATH/bin`, is on `PATH`.

## Command map

```text
callee
├── agent
│   ├── import
│   ├── list
│   ├── schema
│   ├── view
│   ├── validate
│   └── run
├── doctor
├── promptkit
│   ├── list
│   ├── search
│   ├── show
│   └── role create
├── setup
└── bridge codex
```

Use `callee <command> --help` as the current command-and-flag authority. Global `--debug` and `--trace` enable progressively more diagnostics; `--trace` overrides `--debug`.

## Inspect the catalog

List all discovered resources or filter by exact kind:

```bash
callee agent list
callee agent list --kind Role
callee agent list --json
```

`--json` preserves structured stdout and emits structured command errors on stderr. IDs are sorted lexicographically.

View one canonical resource, its resolved tree, effective policies, and unbound Role parameters:

```bash
callee agent view workflows/investigate
callee agent view workflows/investigate --json
```

Text output includes `canEscalate=true|false` on every resolved node. The JSON form exposes the same effective value as `resolvedTree.canEscalate` recursively. This is the computed occurrence capability: a Role is `true` only when every edge from its nearest enclosing Loop opts in. It can therefore differ between aliases of the same resource. Nested Loops start independent authorization boundaries.

Because list and view load the complete registry, any invalid resource, duplicate ID, unresolved reference, cycle, or resolved effective-ID collision prevents the command from succeeding.

## Validate resources and graphs

Validate one physical resource without resolving its children:

```bash
callee agent validate .callee/workflows/investigate.md
```

Print the standalone JSON Schema for one kind:

```bash
callee agent schema Role
callee agent schema Sequential
callee agent schema Loop
```

Success prints `<path>: ok`. Use `agent view` for a selected resolved tree or `doctor --graph` for the complete static registry graph:

```bash
callee doctor --graph text
callee doctor --graph mermaid
callee doctor --graph dot
```

Graph modes do not start provider processes. Plain doctor performs static validation first, then initializes configured Role providers and disposable sessions without sending a model prompt:

```bash
callee doctor
callee doctor --timeout 90s
```

The timeout is applied to each provider group. Doctor requires at least one discovered Role.

Doctor graphs annotate every registry edge with its authored `canEscalate=true|false` value in text, Mermaid, and DOT output. Unlike `agent view`, this is the edge setting rather than a resolved path capability. Compare the graph with a selected resolved view when diagnosing a missing opt-in through a nested composite.

## Import agents from a remote git repository

Use `agent import` when you want to copy a Callee catalog subtree from another repository into the current write root:

```bash
callee agent import https://github.com/acme/platform-agents.git
callee agent import https://github.com/acme/platform-agents.git --path catalog/frontend
callee agent import https://github.com/acme/platform-agents.git --prefix vendor --force
```

The command shells out to the local `git` executable, clones the repository into a temporary checkout, discovers supported `.md`, `.yaml`, and `.yml` resources recursively under `--path`, and imports them into the local write root. `--path` defaults to `.callee`.

`--prefix` rewrites imported resource IDs as `<prefix>/<original-id>` and rewrites `spec.children[].ref` only when the referenced target is also part of the same import set. References to local, non-imported resources are preserved unchanged. Existing destination files are left unchanged by default; `--force` overwrites only the files selected by the current import.

Before writing anything locally, Callee stages the resulting destination tree and validates the complete discovered registry. If validation fails, nothing is written. Successful runs report created, overwritten, and unchanged destination paths on stdout.

## Run an agent tree

```bash
callee agent run workflows/investigate \
  --message "Explain the architecture and main entry points"
```

Omit `--message` to enter the root prompt on the controlling terminal. Supplying an explicitly blank `--message` is an error.

Execution metrics are structured fields on stderr lifecycle events and never change artifact-only stdout. Every Role visit reports `role_*` provider-selection and token fields and, after its first provider turn starts, duration and wait fields. The final `agent run finished` event reports `agent_*` metrics for the complete command. See [Execution metrics](../reference/execution-metrics.md) for the complete field list, duration boundaries, operator-wait semantics, provider-selection fallback, token aggregation, and unsupported tool metrics.

Provide required Role parameters by effective node ID:

```bash
callee agent view workflows/review
callee agent run workflows/review \
  --message "Review the current changes" \
  --param validator.focus=security \
  --param-file worker.context=./request.md
```

The two parameter flags are repeatable. Missing values are prompted on the terminal. `--repl-timeout 45m` changes the maximum wait for every operator prompt in the run.

Execution always requires a real controlling TTY. The terminal carries the root prompt, missing parameters, REPL turns, abort input, and ACP permission selection. Lifecycle and provider diagnostics go to stderr. The sole successful root artifact is written to stdout only after provider cleanup succeeds, so automation should determine success from the exit status rather than an empty stderr assumption.

See [ACP permission requests](acp-permissions.md) for the permission-selection contract, [Workflow semantics](../reference/workflow-semantics.md) for exact input, output, Loop, REPL, and failure behavior, and [Execution metrics](../reference/execution-metrics.md) for emitted run and Role measurements.

## Generate Roles with PromptKit

Callee embeds a pinned PromptKit catalog. Browse it before generating a Role:

```bash
callee promptkit list --type template
callee promptkit search "review Go code" --type template
callee promptkit show review-code --json
```

Create a resource from a selected template:

```bash
callee promptkit role create go-reviewer \
  --template review-code \
  --description "Reviews Go code" \
  --provider codex \
  --prompt-param code \
  --bind language=Go
```

Unless `--output` is supplied, this writes `.callee/roles/go-reviewer.md`. With
`--agent-root <dir>`, the default becomes `<dir>/roles/go-reviewer.md`. The
command creates parent directories and refuses to overwrite an existing file
unless `--force` is set. Use `--dry-run` to print the generated resource
without writing it.

The parameter selected by `--prompt-param` comes from the Role's runtime input. `--bind` and `--bind-file` freeze author-time values. Other declared template parameters become runtime `spec.params`. A configurable persona must use `--persona`. Use `--protocol`, `--taxonomy`, `--format`, or `--no-format` to adjust assembly. Provider session fields are available through `--cmd`, `--model`, `--reasoning`, `--mode`, and repeatable `--extra-arg`.

Templates marked with PromptKit `metadata.mode: interactive` automatically generate `spec.repl: true`; `--repl` forces the same behavior for other templates.

After generation, validate the file and its resolved tree:

```bash
callee agent validate .callee/roles/go-reviewer.md
callee agent view roles/go-reviewer
```

## Install coding-host assets

`callee setup <target>` installs a host integration and six editable starter
resources. Valid targets are `codex`, `claude`, `grok`, `copilot`, `opencode`,
and `cursor`:

```bash
callee setup codex
```

Existing setup-managed files are preserved unless `--force` is supplied. Setup does not install or authenticate the ACP provider selected by a Role. See [Coding-host integrations](coding-host-integrations.md) for target-specific behavior.

Pass `--agent-root <dir>` to make the starter Roles and workflows land under
that root instead of `.callee/`. In that mode, the same directory also becomes
the only discovery root for `agent list`, `agent view`, `agent run`, and
`doctor`.

## Embedded Codex bridge

`codex` Roles use Callee's embedded bridge by default. Inspect its independent flags with:

```bash
callee bridge codex --help
callee bridge codex version
```

The bridge uses ACP over stdin/stdout and does not require a controlling TTY. Place Callee's global diagnostics flag before `bridge codex`; the bridge also defines its own `--debug` flag after that command path:

```bash
callee --debug bridge codex
callee bridge codex --debug
```

## Common failures

| Symptom | Check |
| --- | --- |
| `interactive terminal is required` | Run under a real controlling TTY; redirected stdin is not sufficient. |
| Duplicate resource ID | Remove or rename one matching ID across user/project roots and formats. |
| Child was not found or graph cycle | Run `agent view` and inspect every referenced ID. |
| Required parameter error | Use the exact key reported by `agent view`, including the effective alias. |
| Unauthorized escalation | Inspect `agent view`; every edge from the nearest Loop to that Role occurrence must opt in with `canEscalate: true`. |
| Provider executable was not found | Install the provider runtime or correct `spec.provider.cmd`. |
| Doctor fails before starting providers | Fix schema, semantic, template, or graph errors in the discovered registry first. |
