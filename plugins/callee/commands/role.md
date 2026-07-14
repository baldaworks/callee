---
description: Run or continue a Callee role.
argument-hint: "<role> <task>"
---

Use the bundled `callee` skill to process the raw arguments below as
`role:<role> <task>`.

```text
$ARGUMENTS
```

The first argument is the Callee role ID and the remaining text is its task.
Use MCP mode when Callee's `callee.role` tool is available.
Otherwise use the skill's npx CLI fallback. Return the role's content and
identify whether the result came from MCP or CLI mode.
