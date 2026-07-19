# Release process

Callee releases are tag-triggered. A release commit must pass the local quality gate and every required GitHub Actions run on `main` before an annotated version tag is created. Do not publish manually, tag an unchecked commit, or move an existing release tag.

## Release outputs

A `vX.Y.Z` tag triggers two independent workflows:

| Workflow | Output |
| --- | --- |
| [`release.yml`](../../.github/workflows/release.yml) | Re-runs checks and creates a GitHub release with generated notes. |
| [`omnidist-release.yml`](../../.github/workflows/omnidist-release.yml) | Re-runs checks, builds and verifies npm artifacts, and publishes them to npm. |

The omnidist profile builds CGO-disabled executables for macOS AMD64/ARM64, Linux AMD64/ARM64, and Windows AMD64. It stages the public `@baldaworks/callee` launcher and platform packages, adds the checked-in third-party license texts, verifies the staged npm distribution, and publishes with `NPM_PUBLISH_TOKEN`.

## Prepare the release commit

Choose `X.Y.Z`, then update every checked-in release-version surface consistently. Current version checks cover at least:

- `internal/cli.Version` in [`internal/cli/root.go`](../../internal/cli/root.go);
- `releaseVersion` and pinned skill expectations in [`plugins/callee/plugin_test.go`](../../plugins/callee/plugin_test.go);
- plugin manifests below [`plugins/callee`](../../plugins/callee);
- version-bearing repository marketplace manifests for Claude Code, Copilot CLI, and Grok Build;
- all version-bearing plugin manifests, including the Cursor plugin manifest;
- pinned fallback commands in distributed skill variants.

Use the existing plugin tests to locate drift rather than maintaining a second manual filename list:

```bash
go test ./plugins/callee
```

Review dependency versions and third-party notices when embedded or statically linked components change. The omnidist workflow contains explicit third-party component version text that must stay aligned with the build.

## Run the local gate

Review the complete changed Go diff and format every changed Go file, then run:

```bash
go test ./...
go test -race ./...
go tool golangci-lint run ./...
```

Run relevant plugin validation whenever plugin or marketplace assets changed. The tag workflows also run `go tool govulncheck ./...`; running it locally provides parity with that security gate. Resolve every required local-gate failure before the release commit is pushed.

## Push and verify the exact commit

Commit the release changes and push them to `main` without a tag. Record the commit SHA:

```bash
git rev-parse HEAD
git push origin main
```

Wait for all required GitHub Actions checks on that exact SHA. The normal branch gate consists of the test, lint, and security workflows configured under [`.github/workflows`](../../.github/workflows). Confirm the remote `main` commit and the checked SHA agree before tagging.

Do not use a successful run from an earlier commit as evidence for the release candidate.

## Create and push the annotated tag

Only after the exact commit passes its required remote gate:

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

Do not retag. If the candidate is wrong, prepare and validate a new commit and choose the appropriate new release version.

## Watch publication

Watch both tag-triggered workflows. Each repeats `go test`, the race detector, the configured linter, and `govulncheck` before publication.

For omnidist, verify the sequence completes:

1. build platform executables;
2. stage npm-only artifacts;
3. append third-party licenses;
4. verify staged artifacts;
5. upload and restore the staged bundle;
6. publish npm packages.

For the GitHub release, verify generated release notes and the expected tag are present.

## Post-release verification

Confirm the tag still resolves to the checked release commit:

```bash
git rev-list -n 1 vX.Y.Z
```

Then verify the public surfaces:

```bash
npx --yes @baldaworks/callee@X.Y.Z --version
```

Check the GitHub release and the npm launcher plus its platform packages. A successful GitHub release does not prove npm publication succeeded, and a successful npm job does not replace tag-to-commit verification.

## Prohibited shortcuts

- Do not create or push the tag before the remote gate passes on the exact commit.
- Do not publish npm or GitHub artifacts manually.
- Do not retag an unchecked or corrected commit.
- Do not regenerate the omnidist workflow without restoring its documented npm-only customization.
