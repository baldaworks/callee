---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Runs independent investigation in parallel, then produces one plan.
  children:
    - ref: workflows/parallel-review
      alias: parallel_review
    - ref: roles/architect
      alias: architect
      input: |
        Original task:
        {{ .Input }}

        Parallel review:
        {{ index .State.outputs "parallel_review" }}
  output: |
    {{ index .State.outputs "architect" }}
---
{{ .Input }}
