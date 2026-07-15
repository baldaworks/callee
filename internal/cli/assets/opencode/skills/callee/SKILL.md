---
name: callee
description: Route natural-language coding tasks through project-defined Callee roles. Use when the user asks to investigate, review, change, test, or combine those kinds of work.
user-invocable: true
---

# Callee

## Invocation

Use `$callee <task>`. The user may optionally name a role in ordinary language,
such as `$callee With the reviewer role, review these changes.` Never require a
role choice or a conversation handle, and do not introduce a command syntax for
role selection.

## Route a task

For every fresh task, discover the current role catalog before selecting work:

```bash
npx --yes @baldaworks/callee@0.7.0 role list --json
```

The catalog includes every role's description and a `params` object containing
every parameter name and description. Select capabilities by their stated
purpose, not by a hard-coded role name. Keep role IDs internal to the dispatch.
Use `role view "<selected-role-id>" --json` only when provider metadata is
needed for diagnostics, or `--markdown` when the normalized role definition is
needed. Do not add a redundant view call during ordinary routing.

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

## Dispatch stages

Every Callee prompt made by this skill must use JSON output:


```bash
npx --yes @baldaworks/callee@0.7.0 prompt --role "<selected-role-id>" \
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
npx --yes @baldaworks/callee@0.7.0 prompt --role "<selected-role-id>" \
  --thread-id "<opaque-thread-handle>" --message "<stage task>" --json
```

Do not pass `--param` or `--param-file` when continuing a thread. Parameters
initialize the role context only when the thread starts; follow-ups are sent as
raw messages to the existing context.

Keep only this short-lived routing state in the current host conversation: the
selected role ID, task summary, and most recent opaque thread handle for each
active stage. The model decides whether a new request naturally continues one
of those stages or starts fresh. It may keep several active conversations.
Never write this state to a file or assume it survives a cleared, compacted, or
restarted host conversation. When context is unavailable, ask to start fresh.

If a resumed call reports `resumed: false`, replace the retained handle with the
returned one and say only that the previous context was unavailable and work
continued in a new conversation. Do not expose handles, role IDs, or raw
transcripts unless the user explicitly requests diagnostics.

## Report results

Give a concise result and a human-language trace of the capability stages.
State what changed, checks run, remaining findings, and any unavailable
dependent work. Do not expose internal identifiers or handles.

## Setup

Callee integrations are CLI wrappers and require no server configuration. To
install a host integration and create an editable starter role, run:

```bash
npx --yes @baldaworks/callee@0.7.0 setup <codex|claude|grok|copilot|opencode>
```

Keep provider fields flat. A top-level `params` description map is allowed; do
not add nested provider configuration, Gemini support, or a persistent Callee
role-session store.
