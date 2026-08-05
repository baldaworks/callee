---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Plans a bounded change with Sol, then gives Luna Max the original task and Sol's evidence-backed plan to implement.
  children:
    - ref: codex/sol-luna/roles/sol-planner
      alias: sol_plan
    - ref: codex/sol-luna/roles/luna-max-implementer
      alias: luna_implementation
      input: |
        Original task:
        {{ .Prompt }}

        Sol planning output:
        {{ index .State.outputs "sol_plan" }}
  output: |
    {{ index .State.outputs "luna_implementation" }}
---
{{ .Input }}
