---
name: run-role
description: Run, continue, and combine project-defined Callee roles for coding work. Use when the user asks to delegate investigation, review, implementation, or testing through Callee, optionally with a named role.
---

# Run Callee roles

## Resolve the CLI

Use the same Callee command launcher for the whole workflow:

1. Try `callee --version`. If it succeeds, use `callee` for every command.
2. If `callee` is not available, use
   `npx --yes @baldaworks/callee@0.10.0` as the command prefix.
3. If the fallback fails because the host blocks network or npm cache access,
   including `EAI_AGAIN` or `EROFS`, request the required approval and retry
   the exact command. Do not interpret a failed command as an empty catalog.
4. If neither launcher can run, report the launcher failure and the one-time
   CLI installation requirement instead of guessing about roles.

The examples below use `callee`; substitute the pinned `npx` prefix when the
local CLI is unavailable.

## Route a task

The user may name a role in ordinary language. Never require a role choice or
a conversation handle, and do not introduce a command syntax for role
selection.

For every fresh task, discover the current role catalog before selecting work:

```bash
callee role list --json
```

The catalog includes every role's description, effective top-level `repl`
value, and a `params` object containing every parameter name and description.
Select capabilities by their stated purpose, not by a hard-coded role name.
Keep role IDs internal to the dispatch. After selecting a role, inspect it with
`role view "<selected-role-id>" --json`. Treat its top-level `repl` value as
authoritative for dispatch. Use `--markdown` only when the normalized role
definition is needed.

- When the user naturally names a role, resolve that mention against the
  catalog. Prefer a case-insensitive role ID match after ignoring surrounding
  words such as `role`; otherwise use an unambiguous matching description.
  Run that role as the required first stage. Do not silently substitute a
  different role.
- Use one role for a focused task with a clear matching capability.
- Build a multi-stage workflow when the request implies complementary work.
  Run independent discovery or review stages in parallel. Run a stage that
  relies on another stage's evidence after that evidence is available.
- A naturally named role is a first-stage constraint, not an exclusive lock.
  Add other stages only when they are clearly needed to fulfill the requested
  outcome.
- Add a modifying stage only when the user asked for changes. A review-only or
  investigation-only request stays read-only.
- Do not invoke a modifying stage when prior stages find no actionable work.
- If a named role has no unambiguous catalog match, ask a concise clarification
  rather than guessing. If no capability clearly fits, or the intended workflow
  is otherwise ambiguous, ask a concise clarification in terms of capabilities,
  not role IDs.

For a dependent stage, include the original task and a concise handoff with
actionable findings, evidence and provenance, relevant files, constraints, and
unresolved conflicts. Do not pass raw transcripts. If a prerequisite stage
fails, preserve successful independent results and report that the dependent
work could not be completed.

## Dispatch one-shot stages

When the selected role has `repl: false`, execute one model turn with JSON
output:

```bash
callee exec --role "<selected-role-id>" \
  --message "<stage task>" \
  --param "<name>=<value>" --json
```

For a fresh stage, supply every parameter declared by the selected role.
Infer values that are explicit in the user request or stage handoff. Ask a
concise question for any value that cannot be inferred safely. An empty value
must still be passed explicitly. Use `--message-file` or
`--param-file "<name>=<path>"` when exact multiline file content is the intended
input; file flags accept paths, not stdin.

Use the returned content as the evidence for later stages. When continuing a
stage already active in this host conversation, pass its latest opaque thread
handle internally:

```bash
callee exec --role "<selected-role-id>" \
  --thread-id "<opaque-thread-handle>" --message "<stage task>" \
  --json
```

Do not pass `--timeout` on the first attempt. Callee uses `provider.timeout`
when the role declares it and otherwise uses the CLI default of 15 minutes. If
the first attempt ends specifically because its timeout expired, retry the same
stage with an explicit, larger `--timeout`. Preserve the existing thread handle
for a continued stage; repeat all required inputs when retrying a fresh stage.

Do not pass `--param` or `--param-file` when continuing a thread. Parameters
initialize the role context only when the thread starts; follow-ups are sent as
raw messages to the existing context.

## Dispatch interactive stages

When the selected role has `repl: true`, launch it with a real TTY and without
`--json`:

```bash
callee agent --role "<selected-role-id>" \
  --message "<stage task>" \
  --param "<name>=<value>"
```

Treat controlling-terminal support as a launch prerequisite, not a runtime
probe. Before invoking `callee agent`, select and configure a process runner
whose behavior guarantees that the child starts with a controlled PTY whose
slave is its controlling terminal. Do not launch with a generic PTY merely to
discover whether it is sufficient, and do not assume that an interactive input
stream provides a controlling terminal. The child must be able to open its
controlling terminal independently of stdin, stdout, and stderr.

Start the role only after the runner guarantees this arrangement. Keep a
bidirectional terminal channel available for the lifetime of the process, and
capture stdout and stderr separately from terminal interaction. If no available
runner can make those guarantees, report that the interactive stage is
unavailable; do not switch to `callee exec` or treat a combined transcript as
the result.

Supply parameter values already explicit in the request, and let the terminal
collect only values that remain missing. Do not treat missing declared
parameters as evidence that a non-REPL role should be interactive. Read role
responses from the terminal channel and write answers back through that same
channel. Keep the same `callee agent` process alive throughout the exchange;
do not restart it for each answer. While a response requests information,
answer it. Once the response is a completed artifact rather than a question,
write `quit` to the terminal and wait for the process to exit. The process
writes that final Markdown artifact to stdout and diagnostics to stderr. Use
only that Markdown as the stage result.

Do not pass `--thread-id` to start a replacement interactive process. REPL
continuation happens inside the live TTY process. If that process is no longer
available, start a fresh interactive stage and explain that its prior context
cannot be resumed.

Keep only this short-lived routing state in the current host conversation: the
selected role ID, task summary, and most recent opaque thread handle for each
active stage. The model decides whether a new request naturally continues one
of those stages or starts fresh. It may keep several active conversations.
Never write this state to a file or assume it survives a cleared, compacted, or
restarted host conversation. When context is unavailable, ask to start fresh.

If a resumed `exec` call reports `resumed: false`, replace the retained handle
with the returned one and say only that the previous context was unavailable
and work continued in a new conversation. Do not expose handles, role IDs, or
raw transcripts unless the user explicitly requests diagnostics.

## Report results

Give a concise result and a human-language trace of the capability stages.
State what changed, checks run, remaining findings, and any unavailable
dependent work. Do not expose internal identifiers or handles.

## Setup

Callee integrations are CLI wrappers and require no server configuration. To
install a host integration and create an editable starter role, run:

```bash
callee setup <codex|claude|grok|copilot|opencode>
```

Keep provider fields under the role's `provider` section. The `repl` flag and
`params` description map are top-level role fields. Do not add Gemini support
or a persistent Callee role-session store.
