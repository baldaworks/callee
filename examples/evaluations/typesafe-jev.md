---
apiVersion: callee.metalagman.dev/v1alpha1
kind: TypeSafeJev
spec:
  description: Judges whether a request needs urgent attention through TypeSafe.
  model: jev-1.13
  evidence:
    request: "{{ .Input }}"
    source: operator
  questions:
    urgent:
      type: noul
      instructions: Is the request urgent?
      criteria:
        "true": It needs prompt attention.
        "false": It can follow the normal queue.
    priority:
      type: choice
      instructions: Choose the best priority class.
      criteria:
        low: Routine work.
        medium: Important but not time critical.
        high: Time critical or blocking.
---
