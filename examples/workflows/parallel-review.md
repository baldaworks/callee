---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Parallel
spec:
  description: Explores a task and independently classifies its urgency.
  children:
    - ref: roles/explorer
      alias: exploration
    - ref: evaluations/typesafe-jev
      alias: triage
  output: |
    Codebase findings:
    {{ index .State.outputs "exploration" }}

    Typed urgency decision:
    {{ index .State.outputs "triage" }}
---
{{ .Input }}
