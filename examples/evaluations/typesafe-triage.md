---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Classifies a request, then turns the typed result into an action.
  children:
    - ref: evaluations/typesafe-jev
      alias: triage
    - ref: evaluations/typesafe-follow-up
      alias: recommendation
      input: |
        Original request:
        {{ .Input }}

        Validated triage result:
        {{ index .State.outputs "triage" }}
  output: |
    {{ index .State.outputs "recommendation" }}
---
{{ .Input }}
