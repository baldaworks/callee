# GoalKeeper pack

Give GoalKeeper a coding task. Its worker implements the change, then an
independent validator reviews the actual files and tests. Review feedback goes
back to the worker until the validator accepts the result or the Loop reaches
its five-iteration limit.

## Pack contents

The pack bundles one workflow and both Roles it needs:

| Resource | Responsibility |
| --- | --- |
| [GoalKeeper Loop](workflows/goalkeeper.md) | Runs implementation and validation, carries feedback, and bounds iteration. |
| [Implementer Role](roles/implementer.md) | Makes a bounded change and runs relevant checks; workflow alias: `worker`. |
| [Reviewer Role](roles/reviewer.md) | Reviews the actual changes without editing files; workflow alias: `validator`. |

```mermaid
flowchart TB
    subgraph pack["GoalKeeper pack: examples/goalkeeper/"]
        readme["README.md: index and installation guide"]
        loop["workflows/goalkeeper.md<br/>Loop: at most 5 iterations"]
        worker["roles/implementer.md<br/>Role: worker · Codex"]
        validator["roles/reviewer.md<br/>Role: validator · Codex"]
        readme -. "documents" .-> loop
        loop -->|"first child"| worker
        loop -->|"second child · canEscalate: true"| validator
    end
```

The resource namespace is `goalkeeper`. The workflow references its bundled
`goalkeeper/roles/implementer` and `goalkeeper/roles/reviewer`; it needs no shared
example Roles.

## Install Callee

Install [Node.js with npm](https://nodejs.org/en/download), then install the
Callee CLI:

```bash
npm install --global @baldaworks/callee@latest
callee --version
```

Alternatively, use the [Go installation instructions](../../docs/getting-started/installation.md#install-from-go-source).

Importing the pack also requires `git` on `PATH`. Both bundled Roles use Codex,
so running the workflow requires an installed, authenticated Codex CLI and a
controlling terminal. See [provider prerequisites](../../docs/guides/acp-providers.md).
Installing Callee and importing the pack do not install or authenticate Codex.

## Install the pack in your project

Run this from the root of the project you want GoalKeeper to work on:

```bash
callee agent import baldaworks/callee \
  --path examples/goalkeeper \
  --prefix goalkeeper
```

The command installs three editable resources in your project's catalog:

```text
.callee/
└── goalkeeper/
    ├── roles/
    │   ├── implementer.md
    │   └── reviewer.md
    └── workflows/
        └── goalkeeper.md
```

Keep `--prefix goalkeeper`: it matches the workflow's Role references. The
README is documentation and is skipped during import. Existing destination
files are preserved; add `--force` only when you intend to replace your local
copies with the repository versions. See [import behavior](../../docs/guides/importing-agents.md).

## Inspect and run

From the same project root, inspect the installed workflow and its resolved
Roles before running it:

```bash
callee agent view goalkeeper/workflows/goalkeeper
callee agent run goalkeeper/workflows/goalkeeper \
  --message "Implement the requested change and verify it meets the goal."
```

Replace the message with your task. The worker can edit project files and run
checks. The reviewer retains `ask` permission mode and is instructed to inspect
without editing. A successful run returns the validator's final result prefixed
with `GoalKeeper finished with result:`.

## How the feedback loop works

```mermaid
flowchart TB
    goal["Original goal"]
    worker["Worker: implement the goal<br/>using previous validation when available"]
    workerResult["Shared state: outputs.worker"]
    validator["Validator: inspect actual files and tests<br/>using the goal and worker result"]
    decision{"Validator result?"}
    accepted["Escalate: finish successfully<br/>return final validator output"]
    feedback["Shared state: outputs.validator<br/>actionable feedback for the worker"]
    remaining{"Fewer than 5 iterations used?"}
    exhausted["Fail: iteration limit exhausted"]
    failed["Fail: unrecoverable validation condition"]

    goal --> worker
    goal --> validator
    worker --> workerResult --> validator --> decision
    decision -->|"Goal satisfied"| accepted
    decision -->|"More work needed: normal return"| feedback
    feedback --> remaining
    remaining -->|"Yes"| worker
    remaining -->|"No"| exhausted
    decision -->|"Unrecoverable: fail"| failed
```

Each iteration runs worker then validator. Only the validator child has
escalation authority: a normal validator return requests another iteration;
escalation completes the Loop. Child failures also fail the run. Each Role visit
starts a fresh provider session; the root run's shared state carries results
and feedback between visits.

## Inspect the checked-in example

For contributors working in a checkout of the Callee repository, validate the
physical resources and inspect the pack without importing it:

```bash
callee agent validate examples/goalkeeper/workflows/goalkeeper.md
callee agent validate examples/goalkeeper/roles/implementer.md
callee agent validate examples/goalkeeper/roles/reviewer.md
callee --agent-root examples agent view goalkeeper/workflows/goalkeeper
```

Use `examples` as the catalog root because the checked-in resource references
include the `goalkeeper` namespace.
