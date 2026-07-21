---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: >
    Completes a prepared release by safely tagging its exact green main commit,
    waiting for tag-triggered publishers, and independently verifying all
    public release surfaces.
  children:
    - ref: roles/release-operator
      alias: release_operator
      input: |
        Release request:
        {{ .Input }}

        Select the version automatically from the prepared repository state.

        {{ with index .State.outputs "release_verifier" }}
        Previous independent verification:
        {{ . }}

        Address only transient waiting or diagnostic findings. The release tag
        is immutable: never retag or publish manually.
        {{ end }}
    - ref: roles/release-verifier
      alias: release_verifier
      canEscalate: true
      input: |
        Release request:
        {{ .Input }}

        Release operator report:
        {{ index .State.outputs "release_operator" }}

        Independently inspect the actual repository and public release state.
        Escalate only when the exact checked commit, annotated tag, release
        workflows, GitHub release, and every npm package are all verified.
  maxIterations: 3
  onExhausted: fail
  output: |
    Release verification finished:
    {{ index .State.outputs "release_verifier" }}
---
{{ .Input }}
