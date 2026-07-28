---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Human
spec:
  description: Collects one response for the Human smoke test.
  responseKey: clarification
---
Provide one smoke-test clarification for:
{{ .Input }}
