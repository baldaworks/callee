---
apiVersion: callee.metalagman.dev/v1alpha1
kind: DynamicRole
spec:
  description: Reviews a change with a Codex model selected at visit time.
  provider:
    type: codex
    model: '{{ .State.model }}'
    reasoning: high
    mode: review
    timeout: 20m
  permissions:
    mode: ask
  state:
    model: gpt-5.6-terra
---
Review the requested change. Return concrete findings with file locations and
verification evidence.

{{ .Input }}
