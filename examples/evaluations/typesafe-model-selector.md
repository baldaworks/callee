---
apiVersion: callee.metalagman.dev/v1alpha1
kind: TypeSafeJev
spec:
  description: Selects the smallest capable Codex model for a review request.
  model: jev-1.13.0
  evidence:
    request: "{{ .Input }}"
  questions:
    model:
      type: choice
      instructions: Choose the smallest capable model for this code review request.
      criteria:
        gpt-5.6-luna: Straightforward, narrow, low-risk review work.
        gpt-5.6-terra: Typical repository review requiring balanced reasoning.
        gpt-5.6-sol: Complex, ambiguous, or high-risk review work.
---
