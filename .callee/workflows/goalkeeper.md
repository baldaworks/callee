---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Repeats a Luna worker and validator until validation succeeds.
  children:
    - ref: roles/worker
      alias: worker
      input: |
        Goal:
        {{ .Input }}

        {{ with .State.outputs.validator }}
        Previous validation feedback:
        {{ . }}
        {{ end }}
    - ref: roles/validator
      alias: validator
      input: |
        Goal:
        {{ .Input }}

        Worker result:
        {{ .State.outputs.worker }}
  maxIterations: 5
  onExhausted: fail
  output: |
    GoalKeeper finished with result:
    {{ .State.outputs.validator }}
---
{{ .Input }}
