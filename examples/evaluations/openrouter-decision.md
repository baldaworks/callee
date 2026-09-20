---
apiVersion: callee.metalagman.dev/v1alpha1
kind: OpenRouterDecision
spec:
  description: Scores release risk through OpenRouter Decisions.
  model: typesafe/jev-1.13
  timeout: 30s
  evidence:
    change: "{{ .Input }}"
    production: true
  questions:
    safe:
      type: noul
      instructions: Is this change safe to release?
      criteria:
        "true": The evidence supports release.
        "false": The evidence shows material unresolved risk.
    risk:
      type: score
      instructions: Score the release risk from low to high.
      criteria:
        - Low risk
        - Moderate risk
        - High risk
---
