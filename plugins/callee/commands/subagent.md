---
description: Run a Callee role as a subagent.
argument-hint: "<role> <task> [--new]"
---

Use the bundled `callee` skill to process the raw arguments below.

```text
$ARGUMENTS
```

The first argument is the Callee role ID and the remaining text is its task.
Use MCP mode when Callee's role-list tool is available. Otherwise use the
skill's npx CLI fallback. Return the subagent's content and identify whether
the result came from MCP or CLI mode.
