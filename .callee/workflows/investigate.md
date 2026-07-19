---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Explores a codebase and turns verified findings into an implementation-ready plan.
  children:
    - ref: roles/explorer
      alias: explorer
    - ref: roles/architect
      alias: architect
      input: |
        Original task:
        {{ .Prompt }}

        Explorer findings:
        {{ .State.outputs.explorer }}
  output: |
    {{ .State.outputs.architect }}
---
{{ .Input }}
