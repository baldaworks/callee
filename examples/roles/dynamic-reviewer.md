---
apiVersion: callee.metalagman.dev/v1alpha1
kind: DynamicRole
spec:
  description: Reviews a change with provider selections rendered from state.
  provider:
    type: '{{ .State.provider }}'
    model: '{{ .State.model }}'
    reasoning: '{{ .State.reasoning }}'
    mode: '{{ .State.mode }}'
    timeout: '{{ .State.timeout }}'
  state:
    provider: codex
    model: gpt-5.6-sol
    reasoning: high
    mode: review
    timeout: 20m
---
Review the requested change. Return concrete findings with file locations and
verification evidence.

{{ .Input }}
