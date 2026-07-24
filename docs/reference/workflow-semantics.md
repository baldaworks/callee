# Workflow semantics

This reference describes how a resolved Callee tree executes. Read [Agent resource format](agent-resources.md) first for syntax, discovery, aliases, and template surfaces.

## Root-run contract

`callee agent run <agent-id>` resolves one root and requires a nonblank prompt. The run owns:

- one immutable original prompt;
- one shared, ephemeral state object initialized with an empty `outputs` map;
- one set of runtime parameter values;
- reusable provider processes;
- fresh provider sessions for individual Role visits.

The run succeeds only when the root produces a nonblank artifact and every started provider closes successfully. Callee writes that artifact once to stdout after cleanup. Lifecycle and provider diagnostics use stderr. A nonempty stderr stream is therefore not by itself a failure signal; use the process exit status.

## Node entry and state

Every visit follows the same outer sequence:

1. Increment the visit count and emit `running agent` on stderr.
2. Combine the resource's `spec.state` with the incoming child edge's `state`.
3. Render all string leaves against one pre-node state snapshot.
4. Atomically commit the state modifier.
5. Execute the kind-specific behavior.
6. Emit `agent finished` with status, outcome when available, and a Go duration string. Role visits also emit the per-visit fields defined in [Execution metrics](execution-metrics.md).

If state rendering fails, no part of that visit's modifier is committed and no provider is started for the node.

## Role execution

For a Role visit, Callee resolves parameters in sorted name order. A child-edge binding wins; otherwise the runner uses a qualified CLI value and finally prompts on the controlling terminal. The completed map is available as `.Params` while rendering the Role body.

Callee then:

1. renders the Role body from `.Prompt`, current `.Input`, shared `.State`, and resolved `.Params`;
2. resolves or reuses the ACP provider process;
3. creates and prepares a fresh provider session;
4. sends the rendered body plus Callee's control instructions;
5. interprets the final text and either returns, awaits, escalates, or fails.

While one provider turn remains in flight, Callee emits `agent turn heartbeat`
every 10 seconds with `turn_duration=<elapsed>`. This lifecycle event starts
immediately before the turn call and stops as soon as that call returns or
errors. It does not include Role rendering, process startup, session prepare,
REPL idle time between turns, or composite execution.

A normal non-REPL response without a control record is treated as a successful artifact when it is nonempty. Explicit control records use the rules below.

## Sequential execution

A Sequential renders its `spec.body` to obtain nonblank local input. It then visits children once in source order.

- The first child receives local input unless its edge defines `input`.
- Each later child naturally receives the preceding child's artifact unless its edge defines `input`.
- A child `input` template sees the composite local input as `.Input`, the immutable root prompt as `.Prompt`, and current shared state as `.State`.
- Failure stops the sequence immediately.

Authorized escalation is sticky. When a child escalates, Sequential records the escalation but continues running its remaining children. After the last child, the Sequential propagates escalation upward with the final child's artifact. This allows cleanup or finalization children to run while preserving a descendant's request to return to an enclosing Loop. A later failure overrides the pending escalation.

Without sticky escalation, the natural output is the last child's artifact. An optional `spec.output` renders a replacement from the composite local `.Input`, natural `.Output`, root `.Prompt`, and final `.State`.

## Escalation authorization

Escalation is an edge-level capability of a resolved Role occurrence. It is not enabled merely because a Role is somewhere below a Loop.

- `children[].canEscalate` defaults to `false`, including for scalar child references.
- Beginning at the nearest enclosing Loop, every edge on the path to the Role must set `canEscalate: true`.
- One omitted or false edge denies the capability for that Role occurrence and every descendant along that path, until another Loop establishes a new boundary.
- A `canEscalate: true` edge with no Loop ancestor is rejected during static graph resolution.
- A nested Loop starts an independent authorization boundary. Its direct child edges determine authority inside it; outer-edge values neither grant nor deny the inner capability.

For example, this path authorizes `reviewer` because both edges opt in:

```text
Loop --canEscalate=true--> Sequential --canEscalate=true--> Role reviewer
```

Changing either value to `false`, or omitting it, denies `reviewer`. By contrast, this path uses only the inner edge for `reviewer` because the nested Loop resets the boundary:

```text
outer Loop --canEscalate=false--> inner Loop --canEscalate=true--> Role reviewer
```

Callee includes the `callee.control.v1.escalate` instruction only in prompts for authorized Role occurrences. This omission is guidance, not the enforcement boundary: if any one-shot or REPL Role nevertheless emits the record while its resolved capability is false, the runner rejects it as an attempted unauthorized escalation and stops the workflow immediately.

## Loop execution

A Loop renders local input once, then runs its ordered children for at most `maxIterations` iterations.

- At the start of the first iteration, natural input is the Loop's local input.
- At the start of later iterations, natural input is the preceding iteration's last child artifact.
- Inside an iteration, each child's artifact becomes the next child's natural input.
- Explicit child `input` templates still see the Loop's local input as `.Input`; state outputs are the usual way to reference prior work.
- Failure stops the Loop and fails the root.

An authorized descendant may emit `callee.control.v1.escalate`. The nearest Loop consumes that escalation and completes immediately, applying its optional `spec.output`. A nested Loop therefore consumes an escalation from its own subtree; its successful result continues through the outer composition and cannot complete the outer Loop. Sticky escalation propagated by a Sequential is likewise consumed by the nearest enclosing Loop after the Sequential finishes its remaining children.

If no escalation occurs before the bound:

- `onExhausted: fail` fails with `maximum iterations exhausted`;
- `onExhausted: complete` completes from the last natural child artifact, applying `spec.output` if present.

`fail` is the default exhaustion policy.

## Artifact promotion

Every successful nonblank Role artifact is written to `State.outputs[effectiveId]`. A successfully completed composite promotes its final output under its own effective ID. A Sequential that propagates sticky escalation also promotes its final artifact before returning the escalation.

Failed outcomes are not promoted. Repeated successful visits to the same effective ID replace the previous value.

## Control records and REPL

Callee recognizes four exact final-line records:

```text
callee.control.v1.await
callee.control.v1.return
callee.control.v1.escalate
callee.control.v1.fail
```

When artifact or diagnostic text precedes a record, exactly one empty line must separate it from the record. A malformed `callee.control.` final line is an error.

| Record | Allowed context | Preceding text | Effect |
| --- | --- | --- | --- |
| `await` | REPL Role only | Required | Display text on the TTY, ask the operator for another turn, and reuse the visit session. |
| `return` | Any Role | Required | Complete the Role successfully. |
| `escalate` | Role occurrence whose resolved `canEscalate` is `true` | Optional | Return control toward the nearest Loop. |
| `fail` | Any Role | Optional diagnostic | Fail the workflow. |

Set `spec.interactive: true` only on a Role. Every REPL response must contain one valid final control record; a missing record is an error. The same provider session is retained across `await` turns, while a later visit to that Role still creates a new session.

An unauthorized `escalate` record is an error even though the parser recognizes it. Callee reports the effective agent ID, source resource ID, and resolved path; no later Sequential child runs after that error.

The CLI obtains the next operator response from the controlling terminal, not stdin. Empty replies are retried. Enter `/abort` to abort the workflow; strings such as `/done`, `quit`, or `exit` do not select a Callee outcome. The Role must choose `return`, `escalate`, or `fail` in its control record.

## Parameters

Runtime parameters are keyed by effective node ID:

```bash
callee agent run workflows/review \
  --message "Review the current changes" \
  --param validator.focus=security \
  --param-file worker.context=./request.md
```

Both flags are repeatable. A key may appear only once across both forms. `--param-file` reads the exact file contents and does not accept `-` for stdin. Missing unbound values are requested immediately before the Role visit and cached for later visits to the same effective ID. Blank values are rejected.

Use `callee agent view <agent-id>` to inspect the required qualified keys before running.

## TTY, permissions, and timeouts

`agent run` always opens `/dev/tty`, even when `--message` and all parameters are supplied. This keeps operator prompts and ACP permission choices separate from stdout and stderr and means noninteractive environments without a controlling TTY cannot run a tree.

When an ACP provider requests permission, Callee applies the current Role visit's `spec.permissions.mode`. The default `ask` policy uses the controlling TTY for an interactive numbered selection; `allow` and `deny` select compatible provider options automatically. The complete selection and failure contract is defined in [ACP permission requests](../guides/acp-permissions.md).

Two timeout controls have different purposes:

- `spec.provider.timeout`, default `15m`, independently bounds process startup, session creation/preparation, and each provider turn;
- `--repl-timeout`, default `30m`, bounds each operator prompt, including the initial prompt, missing parameters, REPL responses, and permission selection.

The active provider-turn timeout pauses only while `ask` waits for the operator. Automatic `allow` and `deny` decisions do not pause it. This pause applies only to the current turn's active-time budget; the linked permission guide describes how it interacts with `--repl-timeout` and the other provider operations.

## Failure and cleanup

The root fails when validation or graph resolution fails, a node or provider returns an error, a Role attempts unauthorized escalation, a Role emits `fail`, a Loop exhausts under `fail`, an unconsumed escalation reaches the root, or the root artifact is blank.

Started provider processes close in reverse start order under a 10-second cleanup context. Cleanup failure is joined with any existing error. It also clears a successful artifact, preserving the contract that stdout receives a root artifact only after all provider cleanup succeeds.
