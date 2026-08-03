---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Classifies a task into implement, review, or general.
  provider:
    type: codex
---
Classify this task as exactly one of: implement, review, general.
Return only that route key.

{{ .Input }}
