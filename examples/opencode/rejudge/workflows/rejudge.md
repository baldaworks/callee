---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Sequential
spec:
  description: Runs three independent OpenCode reviewers over the same repository-scoped request, then asks a final judge to synthesize the panel output.
  children:
    - ref: opencode/rejudge/roles/deepseek-reviewer
      alias: deepseek_review
      input: |
        Review the repository and answer the following request.

        Request:
        {{ .Prompt }}
    - ref: opencode/rejudge/roles/mimo-reviewer
      alias: mimo_review
      input: |
        Review the repository and answer the following request.

        Request:
        {{ .Prompt }}
    - ref: opencode/rejudge/roles/minimax-reviewer
      alias: minimax_review
      input: |
        Review the repository and answer the following request.

        Request:
        {{ .Prompt }}
    - ref: opencode/rejudge/roles/glm-judge
      alias: glm_judge
      input: |
        Original request:
        {{ .Prompt }}

        Reviewer 1 (DeepSeek V4 Pro):
        {{ index .State.outputs "deepseek_review" }}

        Reviewer 2 (Mimo v2.5 Pro):
        {{ index .State.outputs "mimo_review" }}

        Reviewer 3 (MiniMax M3):
        {{ index .State.outputs "minimax_review" }}
  output: |
    {{ index .State.outputs "glm_judge" }}
---
{{ .Input }}
