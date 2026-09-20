---
apiVersion: callee.metalagman.dev/v1alpha1
kind: TypeSafeJev
spec:
  description: Chooses a response action from an earlier typed triage result.
  model: jev-1.13.0
  evidence:
    context: "{{ .Input }}"
    stage: follow-up
  questions:
    action:
      type: choice
      instructions: Choose the best next action for this request.
      criteria:
        normal_queue: Keep it in the normal work queue.
        expedite: Move it ahead of routine work.
        page_on_call: Notify the on-call responder immediately.
    confidence:
      type: score
      instructions: Score the strength of the evidence for the selected action.
      criteria:
        - Weak evidence
        - Mixed evidence
        - Strong evidence
---
