# Importing agents

Use `agent import` to copy a Callee catalog subtree from a remote git repository
into the current write root. Callee stages and validates the resulting local
registry before writing any file.

## Prerequisites

The local `git` executable must be available on `PATH`. Confirm the source is
trusted: imported Script resources may execute local commands when later run.

## Import a catalog

```bash
callee agent import acme/platform-agents
callee agent import https://github.com/acme/platform-agents.git --path catalog/frontend
callee agent import acme/platform-agents --prefix vendor
```

An `owner/repo` source expands to its GitHub HTTPS clone URL. `--path` defaults
to `.callee`. Use `--ref <git-ref>` to select a branch, tag, or commit.

Only lowercase `.md`, `.yaml`, and `.yml` resources are discovered recursively.
Documentation and structurally valid documents for another API version are
skipped; malformed documents and documents declaring Callee's current API are
validated strictly.

## Namespace imported IDs

`--prefix vendor` rewrites each imported resource ID as
`vendor/<original-id>`. It also rewrites a child reference when that target is
part of the same import set. References to existing local resources remain
unchanged.

The destination is the project `.callee/` directory by default. With global
`--agent-root <dir>`, that directory is both the only discovery root and the
import destination.

## Existing files and validation

Existing destination files remain unchanged by default. `--force` overwrites
only destinations selected by the current import. Before either behavior is
committed, Callee validates the staged complete registry, including local and
imported resources. A schema error, duplicate ID, missing child, effective-ID
collision, or cycle leaves the destination unchanged.

Successful output separates created, overwritten, and unchanged paths. Inspect
the imported tree before execution:

```bash
callee agent list
callee agent view vendor/workflows/review
```

See [Agent resources](../reference/agent-resources.md#discovery-and-ids) for the
discovery contract and [CLI reference](../reference/cli.md) for adjacent catalog
commands.
