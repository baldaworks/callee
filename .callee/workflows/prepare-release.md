---
apiVersion: callee.metalagman.dev/v1alpha1
kind: Loop
spec:
  description: >
    Prepares and independently reviews a release commit, pushes it to main,
    and waits for required GitHub Actions on the exact commit without tagging
    or publishing.
  children:
    - ref: roles/implementer
      alias: preparer
      input: |
        Release preparation request:
        {{ .Input }}

        This is the pre-release preparation phase. Read AGENTS.md and
        docs/contributing/release.md before changing files. Select the release
        version from repository evidence; the operator does not supply it.

        Determine the version before editing version-bearing files:

        - fetch current `origin/main` and tags without changing the worktree,
          then require a clean local `main` whose HEAD equals `origin/main`;
          fail safely instead of stashing, discarding, or including local work;
        - find the highest stable `vX.Y.Z` tag reachable from `origin/main` and
          inspect every commit in `vX.Y.Z..origin/main`, rejecting a release
          when there are no unreleased commits or when history is ambiguous;
        - apply the highest required bump: a breaking-change marker (`!` or a
          `BREAKING CHANGE` footer) selects the next major version, except that
          this project's existing `0.y.z` series advances to the next minor;
          `feat` selects minor; other releasable fixes or changes select patch;
        - do not let merge commits, an existing release-preparation commit, or
          already-tagged history inflate the bump; fail rather than inventing a
          version when the commit evidence cannot be classified safely;
        - verify that the selected version and tag do not already exist locally
          or remotely, and report the base tag, classified commits, chosen bump,
          and resulting version.

        Prepare the worktree as a release candidate:

        - keep the initially clean worktree limited to release-preparation
          changes and never absorb files created by concurrent work;
        - update every checked-in release-version surface consistently, using
          the repository tests to locate drift instead of relying on a stale
          hard-coded filename list;
        - review dependency versions and third-party notices when embedded or
          statically linked components changed;
        - inspect the complete release diff, format every changed Go file, and
          run the full local quality gate required by AGENTS.md and the release
          guide, including relevant plugin validation;
        - run `go tool govulncheck ./...` for parity with the tag workflows;
        - verify the current branch, worktree, and remote target before any
          mutation, then create the intended release commit and push it to
          `origin/main` without a tag; use the conventional commit subject
          `chore(release): prepare vX.Y.Z` and never force-push;
        - record the pushed commit SHA, confirm remote main resolves to that
          exact SHA, and wait for every required GitHub Actions run on that SHA;
        - report the prepared version, changed files, commit SHA, push result,
          and exact local and remote check results.

        Commit and push only the intended release candidate. Do not create or
        move a tag, create a GitHub release, publish packages, force-push, or
        bypass branch protection. If the worktree is not clean at the start, or
        the push target or intended commit scope is ambiguous, fail safely
        instead of guessing.

        {{ with index .State.outputs "reviewer" }}
        Previous independent review:
        {{ . }}

        Address every material finding, then repeat the complete affected
        validation before returning an updated preparation report.
        {{ end }}
    - ref: roles/reviewer
      alias: reviewer
      canEscalate: true
      input: |
        Release preparation request:
        {{ .Input }}

        Preparer report:
        {{ index .State.outputs "preparer" }}

        Perform an independent, read-only pre-release review. Read AGENTS.md
        and docs/contributing/release.md, inspect the actual worktree and full
        diff, and do not rely only on the preparer report.

        Verify that:

        - the base tag is the highest stable release reachable from remote main,
          the full unreleased commit range was classified, and the selected
          version is the minimal SemVer bump required by the documented policy;
        - the selected version and prospective tag did not already exist before
          preparation, and every checked-in version-bearing surface agrees with
          the selected version;
        - plugin manifests, marketplace metadata, distributed skill fallbacks,
          tests, dependencies, and notices are consistent where applicable;
        - every required local quality gate and relevant plugin validation has
          current, successful evidence, including `go tool govulncheck ./...`;
        - the release commit contains only intended version-preparation changes,
          its SHA exactly matches `origin/main`, and every required GitHub
          Actions run for that exact SHA completed successfully;
        - no tag, GitHub release, package publication, force-push, branch
          protection bypass, or unrelated remote mutation was performed.

        If the candidate is ready for the separate annotated-tag step, return
        concise approval with the version, exact commit SHA, reviewed diff, and
        local and remote check evidence, then escalate to finish the loop.
        Otherwise return actionable findings normally so the next preparation
        phase can correct them with a new ordinary commit and a new exact-SHA
        gate. Do not escalate when evidence is missing, stale, incomplete, or
        uncertain.
  maxIterations: 5
  onExhausted: fail
  output: |
    Pre-release gate finished:
    {{ index .State.outputs "reviewer" }}
---
{{ .Input }}
