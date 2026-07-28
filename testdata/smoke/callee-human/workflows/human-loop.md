---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: Exercises a Role return, Human response, Script check, and next Role visit.
  children:
    - ref: roles/normalizer
      alias: normalizer
    - ref: humans/questions
      alias: questions
    - ref: scripts/assert-clarification
      alias: state_check
  maxIterations: 2
  onExhausted: complete
  output: '{{ .State.clarification }}'
---
{{ .Input }}
