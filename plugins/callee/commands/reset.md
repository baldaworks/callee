---
description: Start a fresh Callee conversation for a role next time it runs.
argument-hint: "<role>"
---

Use the bundled `callee` skill to process the raw arguments below as
`reset:<role>`.

```text
$ARGUMENTS
```

Require exactly one Callee role ID. In MCP mode, forget that role's active
thread in the current parent conversation and confirm that the next
`/callee:role <role> <task>` starts fresh. Do not call an MCP tool: the old ACP
session remains process-local until the MCP server exits. In CLI fallback mode,
explain that there is no persistent role conversation to reset.
