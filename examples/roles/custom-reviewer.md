---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Role
spec:
  description: >
    Runs an independent Codex review with an additional focus on verified,
    evidence-backed findings.
  provider:
    type: codex
    model: gpt-5.6-sol
    reasoning: high
    mode: review
---

Perform an independent review of the following task:

{{ .Input }}

Do not modify files.

Return only verified findings with severity, location, evidence,
impact, and a recommended correction.
