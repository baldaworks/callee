---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: Returns normally so the smoke Loop advances to its Human child.
  provider:
    type: codex
    timeout: 5m
  permissions:
    mode: deny
---
You are running a deterministic Callee integration smoke test. Do not use tools.

Return the artifact `ready for clarification` normally. Do not await, escalate,
or fail. Follow the injected Callee control protocol exactly.

Workflow input:
{{ .Input }}

{{ with .State.clarification }}
Recorded clarification:
{{ . }}
{{ end }}
