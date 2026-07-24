# Callee

[![Test](https://github.com/baldaworks/callee/actions/workflows/test.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/test.yml)
[![Lint](https://github.com/baldaworks/callee/actions/workflows/lint.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/lint.yml)
[![Security](https://github.com/baldaworks/callee/actions/workflows/security.yml/badge.svg)](https://github.com/baldaworks/callee/actions/workflows/security.yml)
[![Latest release](https://img.shields.io/github/v/release/baldaworks/callee)](https://github.com/baldaworks/callee/releases/latest)
[![npm version](https://img.shields.io/npm/v/%40baldaworks%2Fcallee)](https://www.npmjs.com/package/@baldaworks/callee)
[![License: MIT](https://img.shields.io/github/license/baldaworks/callee)](LICENSE)

## Markdown-defined agents and deterministic workflows

Callee lets a repository define provider-backed `Role`, `Sequential`, and `Loop` agents as versioned Markdown or YAML. Markdown is the base authoring and generation format; YAML represents the same complete schema object with `spec.body` inline. Every kind has the same node boundary: it receives input, may update one root-run state object, and returns one artifact or a structured orchestration outcome.

Callee remains CLI-only. It uses [Norma Runtime](https://github.com/normahq/runtime) for ACP provider processes and Go ADK-aligned escalation semantics. It has no server, durable thread store, or handle binding.

## Documentation

See the [Callee engineering documentation](docs/index.md) for detailed guides,
architecture, reference, and contributor material. This README stays focused
on installation and first use.

## Agent skills

Callee installs two complementary skills in your coding host:

| Skill | What it does | Result |
| --- | --- | --- |
| **Run Agent** | Discovers project-defined agents, resolves the selected tree and required parameters, and runs a `Role`, `Sequential`, or `Loop` agent through its controlling terminal. | The completed root artifact and a concise capability trace, followed by the emitted run-wide and per-Role execution metrics. |
| **Create Agent** | Authors a reusable `Role`, `Sequential`, or `Loop` in Markdown or YAML, using an embedded PromptKit template when one fits, then validates the file and resolved tree. | A validated agent or deterministic workflow below `.callee/`. |

These skills are host integrations: they teach Codex, Claude Code, Grok Build,
Copilot CLI, OpenCode, or Cursor how to create and run Callee agents. Runtime
ACP providers are a separate layer selected by `spec.provider.type`; they
supply the model session for each `Role`. Host setup does not install or
authenticate an ACP provider, even when the host and provider share a name.

## Set up your coding host

Run setup from the project root with the complete one-shot `npx` command for
your host below. No global Callee installation is required; each cell is
directly executable as written. Setup installs both skills and six editable
starter agents.

| Host | Setup | Run Agent | Create Agent |
| --- | --- | --- | --- |
| Codex | `npx --yes @baldaworks/callee@latest setup codex` | `$callee:run-agent` | `$callee:create-agent` |
| Claude Code | `npx --yes @baldaworks/callee@latest setup claude` | `/callee:run-agent` | `/callee:create-agent` |
| Grok Build | `npx --yes @baldaworks/callee@latest setup grok` | `/callee-run-agent` | `/callee-create-agent` |
| Copilot CLI | `npx --yes @baldaworks/callee@latest setup copilot` | `/callee-run-agent` | `/callee-create-agent` |
| OpenCode | `npx --yes @baldaworks/callee@latest setup opencode` | `callee-run-agent` skill (`/callee` wrapper) | `callee-create-agent` skill (`/callee-create-agent` wrapper) |
| Cursor | `npx --yes @baldaworks/callee@latest setup cursor` | `callee-run-agent` skill | `callee-create-agent` skill |

Codex, Claude Code, Grok Build, and Copilot CLI setup use the repository's
plugin marketplace. OpenCode setup writes skills and convenience commands
below `.opencode/`; Cursor setup writes skills below `.cursor/skills/`.
Existing local files are preserved; use `--force` only when setup-managed
assets should be replaced. Host CLIs and credentials remain external.

## Use Callee from your host

In Codex, invoke the Callee plugin directly and describe the outcome. Callee
routes the request to the appropriate installed skill, so the `:run-agent` or
`:create-agent` suffix is optional:

```text
$callee Run workflows/investigate to explain this project's architecture and main entry points.
```

Create a reusable Role while selecting its provider, model, and reasoning:

```text
$callee Create a Go code-review Role named roles/go-reviewer using the codex provider, model gpt-5.6-sol, and high reasoning.
```

Or compose existing agents into a deterministic workflow:

```text
$callee Create a Loop workflow named workflows/implementation-goalkeeper with roles/implementer as worker and roles/reviewer as validator. Run at most five iterations and finish only when the reviewer approves.
```

The explicit `$callee:run-agent` and `$callee:create-agent` selectors remain
available when you want to choose a skill directly. For other hosts, use the
invocation shown in the setup table. Run Agent inspects the catalog and selected
tree before execution. After completion, it returns the root artifact and a
concise capability trace, then reports the run-wide and per-Role metrics emitted
by Callee. Repeated and nested Role visits remain separate, and fields that were
not emitted remain omitted. See [Execution metrics](docs/reference/execution-metrics.md)
for field definitions, presence rules, and aggregation boundaries. Create Agent
validates every file it writes and the fully resolved tree.

## npm CLI installation and quick start

These npm paths require Node.js with npm, which provides `npx`. For repeated
direct shell use, install the npm launcher globally:

```bash
npm install --global @baldaworks/callee@latest
callee --version
```

For one-shot shell use without installing Callee, run the complete `npx`
command:

```bash
npx --yes @baldaworks/callee@latest --version
```

Every setup target creates the same starter IDs. After running one setup
command from the host table, inspect the installed agents and the safe,
read-only workflow directly. These quick-start examples use the one-shot form:

```bash
npx --yes @baldaworks/callee@latest agent list
npx --yes @baldaworks/callee@latest agent view workflows/investigate
npx --yes @baldaworks/callee@latest agent run workflows/investigate --message "Explain this project's architecture and main entry points"
```

Validate one agent file without loading its referenced children:

```bash
npx --yes @baldaworks/callee@latest agent validate .callee/roles/reviewer.md
```

Import a remote Callee catalog subtree into the current project root:

```bash
npx --yes @baldaworks/callee@latest agent import acme/platform-agents --prefix vendor
```

When you are ready to make changes, run GoalKeeper through the same entrypoint:

```bash
npx --yes @baldaworks/callee@latest agent run roles/reviewer --message "Review the current changes"
npx --yes @baldaworks/callee@latest agent run workflows/goalkeeper --message "Implement the requested feature"
```

The remaining CLI examples use `callee` for readability and assume the global
npm installation above.

### Secondary alternative: install from Go source

If you prefer the Go toolchain, install the executable with the Go version
declared in [`go.mod`](go.mod):

```bash
go install github.com/baldaworks/callee/cmd/callee@latest
callee --version
```

Ensure the Go installation directory (normally `$GOBIN` or `$GOPATH/bin`) is
on `PATH`.

`agent run` always requires a real controlling TTY. Initial input, missing parameters, permission questions, and REPL turns use the TTY. Info lifecycle events for every `Role`, `Sequential`, and `Loop` visit, plus received and answered ACP permission requests, use stderr, so nonempty stderr alone does not indicate failure; use the command exit status. Lifecycle durations use standard Go strings with units, such as `43.453998585s`.

`agent run` emits run-wide metrics on its final `agent run finished` event and
per-Role metrics on each Role's `agent finished` event. The successful root
artifact is written once to stdout only after provider cleanup and the final
stderr metrics event. See [Execution metrics](docs/reference/execution-metrics.md)
for the complete field, presence, and aggregation contract.

## Manual host setup

The one-shot `npx ... setup` commands above are the recommended installation
path. Use the following steps only when you need to manage the host integration
yourself. These marketplace commands install the two Callee skills but do not
write the six starter agents:

### Marketplace hosts

Codex:

```bash
codex plugin marketplace add baldaworks/callee
codex plugin add callee@callee
```

If Codex already has a `callee` marketplace registration, remove it with
`codex plugin marketplace remove callee` before adding it again.

Claude Code:

```bash
claude plugin marketplace add baldaworks/callee
claude plugin install callee@callee --scope project
```

Grok Build:

```bash
grok plugin marketplace add baldaworks/callee
grok plugin install callee@callee --trust
```

Copilot CLI:

```bash
copilot plugin marketplace add baldaworks/callee
copilot plugin install callee@callee
```

### File-based hosts

For a manual file-based installation, copy the repository assets into the
matching project directories without replacing unrelated or customized files:

| Host | Source | Project destination |
| --- | --- | --- |
| OpenCode skills | `internal/cli/assets/opencode/skills/` | `.opencode/skills/` |
| OpenCode commands | `internal/cli/assets/opencode/commands/` | `.opencode/commands/` |
| Cursor skills | `internal/cli/assets/cursor/skills/` | `.cursor/skills/` |

The OpenCode command files provide `/callee` and `/callee-create-agent`
wrappers around the `callee-run-agent` and `callee-create-agent` skills.
Cursor also exposes the same skills through the repository's
[Cursor marketplace](.cursor-plugin/marketplace.json).

To install the editable starter Roles and workflows manually, copy the contents
of `internal/cli/assets/starter/` into `.callee/`, preserving the `roles/` and
`workflows/` directories. Review existing files before replacing them. Starter
Roles select only `spec.provider.type`; provider model, mode, and reasoning use
the ACP backend defaults. Pass `--agent-root <dir>` to `callee` when these
resources should be discovered from and written under a different root.

## Agent format

Agents are discovered recursively below two roots:

- `$XDG_CONFIG_HOME/callee`, or `$HOME/.config/callee` when
  `XDG_CONFIG_HOME` is unset;
- the current project's `.callee` directory.

Pass `callee --agent-root <dir> ...` to use one custom discovery root instead.
When set, Callee ignores both default roots and treats `<dir>` as the only
agent catalog and the default write target for generated or installed Callee
resources, including `agent import`.

`callee agent import <repo> [--ref <git-ref>] [--path <remote-dir>] [--prefix <namespace>] [--force]` clones a remote git repository into a temporary checkout, discovers resources recursively under `--path` (default `.callee`), and stages the resulting local tree before writing anything. When `<repo>` is in `owner/repo` form, Callee treats it as GitHub shorthand and expands it to `https://github.com/owner/repo.git` before cloning. `--prefix` rewrites imported IDs and imported internal child refs into a namespace. Existing destination files are preserved unless `--force` is supplied, and `git` must be available on `PATH`.

Lowercase `.md`, `.yaml`, and `.yml` regular files are supported; symlinked
files are skipped. Directories such as `roles/` and `workflows/` are optional
ID namespaces; `kind` alone determines behavior. The final extension is
removed from the ID, so `.callee/roles/reviewer.md` and
`.callee/roles/reviewer.yaml` both have ID `roles/reviewer` and conflict if
both exist. IDs must be unique across both roots and all formats. Project
agents do not shadow user agents; any duplicate makes discovery fail.

All agents use the same Kubernetes-style `apiVersion`/`kind`/`spec` envelope.

## Agent kinds

### Role

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews changes.
  provider:
    type: codex
  permissions:
    mode: ask
  params:
    focus: Review focus
---
You are a reviewer.

Task:
{{ .Input }}

Focus:
{{ .Params.focus }}
```

A Role body must contain exactly one unconditional bare `{{ .Prompt }}` or
`{{ .Input }}` insertion. `.Prompt` is the immutable original root prompt;
`.Input` is this node occurrence's rendered input.

### ACP provider configuration

Provider configuration is nested below `spec.provider`; flat provider fields
are not supported.

| `type` | Default ACP command | Runtime prerequisite |
| --- | --- | --- |
| `codex` | Current Callee executable with `bridge codex` | Installed and authenticated Codex CLI |
| `claude` | `npx -y @zed-industries/claude-code-acp@latest` | Node.js with `npx`, npm registry access on the first uncached run, and credentials accepted by the Claude Code ACP adapter |
| `opencode` | `opencode acp` | Installed and authenticated `opencode` CLI |
| `copilot` | `copilot --acp --stdio` | Installed and authenticated `copilot` CLI |
| `grok` | `grok agent stdio` | Installed `grok` CLI authenticated with `grok login` or `XAI_API_KEY` |
| `cursor` | `agent acp` | Installed and authenticated Cursor CLI; see the [Cursor ACP documentation](https://cursor.com/docs/cli/acp) |
| `generic_acp` | None | An ACP executable named by nonblank `cmd` |

The Codex ACP bridge is built into Callee, so the default does not download or
launch a separate npm bridge package. Other provider commands must be on
`PATH` when the Role runs. Callee does not install or authenticate runtime
providers. A host integration and a provider may use the same product name,
but setup of one does not satisfy the runtime prerequisites of the other.

Run `callee bridge codex --help` to inspect the embedded bridge directly. It
uses stdin/stdout for ACP and does not require a controlling TTY. Place the
root `--debug` flag before `bridge codex` for Callee diagnostics; place the
bridge's own `--debug` flag after `bridge codex` for bridge diagnostics.

All provider types accept the following fields:

- `type` (required): one of the seven values above.
- `cmd`: executable override. Use `extraArgs` for its arguments.
- `model`: backend-specific model identifier.
- `reasoning`: backend-specific reasoning value, such as `high` when the
  backend supports it.
- `mode`: backend-specific session mode.
- `extraArgs`: ordered arguments appended to the resolved command.
- `timeout`: positive Go duration applied separately to provider process
  startup, the session creation/prepare sequence, and each provider turn; the
  default is `15m`.

For example:

```markdown
  provider:
    type: generic_acp
    cmd: my-acp-agent
    model: provider-model
    reasoning: high
    mode: review
    extraArgs:
      - --stdio
    timeout: 20m
```

Empty `model`, `reasoning`, and `mode` fields defer to the backend. Support for
nonempty values is backend-specific. Gemini is not a supported Callee provider
type.

Each Role may set `spec.permissions.mode` to `ask`, `allow`, or `deny`; omission
defaults to `ask`. `ask` presents the provider-supplied choices on the
controlling TTY and pauses the active provider-turn budget while
`--repl-timeout` bounds the operator wait. `allow` automatically prefers
`allow_once` and then `allow_always`; `deny` similarly prefers `reject_once`
and then `reject_always`. A missing compatible option fails the run. See
[ACP permission requests](docs/guides/acp-permissions.md) for the exact policy,
session, timeout, and failure contract.

To temporarily use the external Codex bridge instead, override the executable
and put every argument in `extraArgs`:

```markdown
  provider:
    type: codex
    cmd: npx
    extraArgs:
      - -y
      - '@normahq/codex-acp-bridge@1.7.7'
```

See the runnable [`reviewer`](examples/roles/reviewer.md) example.

### Sequential

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Plans, implements, and validates.
  children:
    - ref: roles/planner
      alias: planner
    - ref: roles/implementer
      alias: implementer
      input: |
        Plan:
        {{ .State.outputs.planner }}
    - ref: roles/reviewer
      alias: validator
  output: |
    {{ .State.outputs.validator }}
---
{{ .Input }}
```

`Sequential` runs children in source order. Without an explicit child `input`, the first child receives the composite input and later children receive their predecessor's output. Escalation is sticky across the remaining sequential children and propagates upward after they finish. See the runnable [`investigate`](examples/workflows/investigate.md) example.

### Loop

```markdown
---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Repeats a worker and validator until the validator escalates.
  children:
    - ref: roles/implementer
      alias: worker
      input: |
        Goal:
        {{ .Input }}

        {{ with index .State.outputs "validator" }}
        Previous validation:
        {{ . }}
        {{ end }}
    - ref: roles/reviewer
      alias: validator
      canEscalate: true
      input: |
        Goal:
        {{ .Input }}

        Worker result:
        {{ .State.outputs.worker }}

        Validate the result. If it satisfies the goal, return your validation
        and escalate to finish the loop. Otherwise return actionable feedback
        normally so the next iteration can improve it.
  maxIterations: 5
  onExhausted: fail
  output: |
    GoalKeeper finished with result:
    {{ .State.outputs.validator }}
---
{{ .Input }}
```

A `Loop` repeats its ordered children up to `maxIterations`. It consumes escalation from an authorized child and completes. Set `canEscalate: true` on every edge from the nearest `Loop` to the Role that may finish it; omitted values default to `false`. `onExhausted` is `fail` by default or may be `complete`. `Parallel` is not part of v1alpha1. See the runnable [`goalkeeper`](examples/workflows/goalkeeper.md) example.

### Children and composition

Children may reference any supported kind, including another `Loop`. A child mapping supports `ref`, optional globally unique `alias`, `canEscalate`, `input`, shallow `state`, and Role-only `params`. Aliases match `^[a-z][a-z0-9_]*$` and replace the occurrence's effective ID. `canEscalate` is occurrence-specific, so two aliases of the same Role may have different authority.

## YAML representation and JSON Schema

Markdown is the canonical authoring format: its physical body becomes `spec.body` and `spec.body` must not also appear in frontmatter. A `.yaml` or `.yml` file represents the same complete resource object and must author `spec.body` inline.

Callee validates both representations against the checked-in [Draft 2020-12 JSON Schema](internal/agent/schema.json), whose exact bytes are embedded in the binary. Use `callee agent schema <Role|Sequential|Loop>` when you want a standalone schema document for one kind. For editor integration, use the raw schema from the repository:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/baldaworks/callee/main/internal/agent/schema.json
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Reviews changes.
  provider:
    type: codex
  permissions:
    mode: ask
  params:
    focus: Review focus
  body: |
    You are a reviewer.

    Task:
    {{ .Input }}

    Focus:
    {{ .Params.focus }}
```

This YAML object is canonically identical to the Markdown Role above. The same representation rule applies to `Sequential` and `Loop`.

`agent validate` performs schema, semantic, state, and template validation for exactly one file. It intentionally does not resolve workflow child references; use `agent view <id>` or `doctor` to validate the discovered graph.

## Templates and state

All workflow-aware surfaces use Go `text/template` with `missingkey=zero`, a pinned deterministic Sprig v3.3.0 positive allowlist, and strict explicit-input UTC `dateParse`/`dateFormat` helpers. Environment, filesystem, network, clock, random, UUID, crypto-generation, and mutating dictionary helpers are unavailable.

The common template root exposes:

- `.Prompt`: immutable original user prompt.
- `.Input`: current node input.
- `.State`: one JSON-compatible root-run state object.
- `.Params`: current Role parameter map.
- `.Output`: natural child-derived output, only while rendering composite `spec.output`.

Every successful nonblank node artifact is promoted to `.State.outputs[effectiveId]`. The engine owns `outputs`; authored state cannot replace it. Repeated loop visits use last-successful-write-wins.

State modifiers are shallow top-level replacements. String leaves are templates evaluated against one pre-node snapshot, and the complete modifier commits atomically.

### Runtime parameters

`spec.params` declares required Role inputs and their descriptions. A direct
Role uses its resource ID as the effective node ID; a child alias replaces that
ID for one workflow occurrence. `callee agent view <agent-id>` reports every
unbound parameter using the required `<effective-node-id>.<name>` key.

For example, after saving the Role example above as
`.callee/roles/focused-reviewer.md`:

```bash
callee agent view roles/focused-reviewer
callee agent run roles/focused-reviewer \
  --message "Review the current changes" \
  --param roles/focused-reviewer.focus=security
```

Repeat `--param` for literal values. Use
`--param-file <effective-node-id>.<name>=<path>` for exact multiline file
contents; stdin (`-`) is not accepted. Parameters bound by a composite child's
`params` mapping are omitted from the runtime requirements. If a required value
is not supplied by a flag, Callee asks for it through the controlling terminal
immediately before that Role runs.

## Control and REPL

Set `spec.interactive: true` only on a `Role` that needs multiple operator turns in
one provider session. Composite agents do not have a REPL field. PromptKit
generation also enables this field automatically for templates whose
`metadata.mode` is `interactive`, or explicitly with
`callee promptkit role create ... --interactive`.

Callee injects a versioned control protocol into every executed Role. Every
REPL turn must end with exactly one final record:

```text
callee.control.v1.await
callee.control.v1.return
callee.control.v1.escalate
callee.control.v1.fail
```

Artifact text, when present, is separated from the record by exactly one empty
line. `await` requires question text and retains the same visit session for
another operator turn. `return` requires an artifact and completes normally.
`escalate` is available only when the Role is inside a Loop and returns control
to the nearest Loop. `fail` aborts the root.

Every Role visit starts a fresh provider session, including repeated Loop
visits; only `await` turns within one REPL visit reuse a session. A prepared
REPL visit emits one `entering repl` / `exiting repl` lifecycle pair, with all
`await` turns inside it. `agent run` requires a controlling TTY even when
`--message` and every parameter are supplied. The default maximum wait for each
operator prompt is `30m`; change it with `--repl-timeout`. Hosts must answer on
the TTY and must not send `/done`, `quit`, or `exit` to choose completion.

## Doctor and graphs

```bash
callee doctor
callee doctor --timeout 90s
callee doctor --graph text
callee doctor --graph mermaid
callee doctor --graph dot
```

Plain doctor completes static schema/template/graph validation before provider startup, groups Roles by provider process identity, checks ACP initialization and disposable session creation, and sends no model prompt. Graph modes are static-only and never start providers.

## PromptKit

Callee embeds the pinned PromptKit catalog through [PromptKitty](https://github.com/baldaworks/promptkitty):

```bash
callee promptkit list
callee promptkit search "write requirements document" --type template
callee promptkit show review-code
callee promptkit role create go-reviewer \
  --template review-code \
  --description "Reviews Go code" \
  --provider codex \
  --prompt-param code \
  --bind language=Go
```

`search` uses PromptKitty's deterministic, in-memory BM25 index from vecgo. It
ranks component names, descriptions, metadata, and complete Markdown bodies
with weights `4 / 2 / 1 / 1`, while preserving the existing table and JSON
component shapes. Callee exposes catalog `list`, `search`, and `show` commands
plus its own `role create`; PromptKitty's standalone `assemble` and `setup`
commands are not mounted.

Generated Roles use the v1alpha1 envelope and Go templates. Unbound PromptKit parameters become `spec.params`; literal template examples in assembled PromptKit text are escaped safely. A template whose `metadata.mode` is `interactive` automatically generates `spec.interactive: true`, so its questions and confirmation gates run through the same provider session when the Role is executed. Use `--interactive` to force that behavior for an ordinary template.

Unless `--output` is supplied, `role create go-reviewer` writes
`.callee/roles/go-reviewer.md`; it creates parent directories but refuses to
replace an existing file unless `--force` is set. `--dry-run` prints the
generated Markdown without writing it. The parameter selected by
`--prompt-param` is filled from the root message. `--bind` and `--bind-file`
freeze author-time values; every other declared template parameter becomes a
runtime `spec.params` entry. A configurable `persona` must be supplied with
`--persona`, not through the parameter flags.

## Distribution and limits

The npm distribution uses CGO-disabled native executables for macOS and Linux
on AMD64/ARM64 and Windows AMD64 behind the `@baldaworks/callee` launcher. Each
root run is ephemeral: no Callee thread store, persisted workflow state, server
transport, or cross-process continuation is created.

## License

Callee is released under the [MIT License](LICENSE). See
[Third-party notices](THIRD_PARTY_NOTICES.md) for embedded and statically linked
dependencies.

## OpenAI Build Week

Callee was built during OpenAI Build Week using Codex and GPT-5.6 as the primary development system. The project was directed by a human operator, but the implementation loop, design iteration, CLI behavior, workflow semantics, graph tooling, setup flows, examples, and documentation were produced through sustained collaboration with Codex and GPT-5.6.

Codex and GPT-5.6 were used to:

- design the `Role`, `Sequential`, and `Loop` agent model;
- implement CLI commands, runtime behavior, and validation flows;
- build graph inspection, doctor checks, and setup integrations;
- create starter agents, examples, and user-facing documentation;
- refine repository UX, packaging, and release-facing presentation.

The human role remained product direction, architecture review, scope control, and final acceptance of what shipped.
