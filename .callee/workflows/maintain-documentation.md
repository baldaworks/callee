---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: >
    Maintains technical documentation through iterative writing and independent
    review until the documentation satisfies the project goal.
  children:
    - ref: roles/technical-writer
      alias: writer
      input: |
        Documentation goal:
        {{ .Input }}

        This is the writing phase. Inspect the relevant repository sources and
        create or update only the documentation required by the goal. Preserve
        verified project behavior and existing documentation conventions.

        Return your writing report normally. Do not escalate from this phase;
        only the reviewer controls completion of the documentation loop.

        {{ with index .State.outputs "reviewer" }}
        Previous review feedback:
        {{ . }}

        Address every material finding before returning the updated writing
        outcome.
        {{ end }}
    - ref: roles/technical-writer
      alias: reviewer
      input: |
        Documentation goal:
        {{ .Input }}

        Writer report:
        {{ index .State.outputs "writer" }}

        This is an independent read-only review phase. Do not modify files.
        Inspect the actual documentation and authoritative repository sources,
        not only the writer report. Verify that the goal is fully addressed,
        factual claims and examples are supported, commands are current, the
        structure serves the intended reader, and no material ambiguity or
        contradiction remains.

        If the documentation satisfies the goal, return concise approval with
        the evidence and checks used, then escalate to finish the loop. If it
        does not, return actionable findings normally so the next writing phase
        can address them. Do not escalate on an incomplete or uncertain result.
  maxIterations: 5
  onExhausted: fail
  output: |
    Documentation workflow finished:
    {{ index .State.outputs "reviewer" }}
---
{{ .Input }}
