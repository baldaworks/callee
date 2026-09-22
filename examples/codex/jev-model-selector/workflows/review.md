---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Uses TypeSafe Jev to select a model, then runs a DynamicRole review.
  children:
    - ref: codex/jev-model-selector/evaluations/model-selector
      alias: model_selector
    - ref: codex/jev-model-selector/roles/dynamic-reviewer
      alias: reviewer
      input: "{{ .Prompt }}"
      state:
        model: '{{ .State.evaluations.model_selector.answers.model.choice }}'
  output: |
    Selected model: {{ .State.evaluations.model_selector.answers.model.choice }}

    {{ .State.outputs.reviewer }}
---
{{ .Input }}
