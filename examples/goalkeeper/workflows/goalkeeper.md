---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Repeats a worker and validator until the validator escalates.
  children:
    - ref: goalkeeper/roles/implementer
      alias: worker
      input: |
        Goal:
        {{ .Input }}

        {{ with index .State.outputs "validator" }}
        Previous validation:
        {{ . }}
        {{ end }}
    - ref: goalkeeper/roles/reviewer
      alias: validator
      canEscalate: true
      input: |
        Goal:
        {{ .Input }}

        Worker result:
        {{ .State.outputs.worker }}

        Validate the result. If it satisfies the goal, return your validation
        and escalate to finish the loop. Otherwise return actionable feedback
        normally so the next iteration can improve it. Reserve
        fail for unrecoverable validation conditions.
  maxIterations: 5
  onExhausted: fail
  output: |
    GoalKeeper finished with result:
    {{ .State.outputs.validator }}
---
{{ .Input }}
