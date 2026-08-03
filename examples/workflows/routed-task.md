---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Classifies a task and dispatches exactly one routed handler.
  children:
    - ref: roles/classifier
      alias: classifier
    - ref: workflows/task-router
      alias: router
---
{{ .Input }}
