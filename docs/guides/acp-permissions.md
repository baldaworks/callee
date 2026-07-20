# ACP permission requests

Use a Role's `spec.permissions.mode` to decide how Callee answers ACP permission requests during `callee agent run`:

```yaml
spec:
  provider:
    type: codex
  permissions:
    mode: ask
```

`permissions` belongs directly under `spec` and is valid only for `Role`. It is independent of backend-specific `spec.provider.mode`.

## Modes

The accepted lowercase modes have closed semantics:

| Mode | Behavior |
| --- | --- |
| `ask` | Display the provider's options on the controlling TTY and return the exact option ID selected by the operator. |
| `allow` | Select the first `allow_once` option; if absent, select the first `allow_always` option. |
| `deny` | Select the first `reject_once` option; if absent, select the first `reject_always` option. |

Omitting `permissions` defaults to `ask`. Unknown modes, empty permission objects, permission fields under `provider`, and permission fields on composite kinds fail resource validation.

Automatic modes never choose an option of the opposite decision or an unknown kind. If the provider offers no compatible option, the root run fails. The diagnostic identifies the effective Role ID, configured policy, and offered option kinds, but does not include tool arguments. Duplicate compatible options retain provider order, so the first one wins.

The policy is bound when a Role session is prepared. Two aliases of the same Role always use separate ACP sessions. Repeated Loop visits receive a fresh binding by default; `session: stateful` retains the binding with that Role occurrence's session for the owning Loop invocation. Provider process reuse does not share a decision between sessions.

## Interactive `ask` flow

For `ask`, Callee handles a provider request as follows:

1. If the request contains no options, return an ACP cancellation without prompting.
2. Pause the active provider-turn timeout and write the request title, tool kind, human-readable ACP content, and every option to the controlling TTY in provider order:

   ````text
   Permission required:
   Exec command approval [execute]

   The command needs network access.

   Command:
   ```sh
   curl example.com
   ```

   Working directory: `/tmp/work`

   1) Allow once [allow_once]
   2) Reject once [reject_once]
   Select:
   ````

   Text content is displayed in full. Images, audio, resources, diffs, and terminal references use concise descriptors. Callee never falls back to displaying `rawInput` when human-readable content is absent.

3. Read a one-based option number and return that entry's exact opaque ACP option ID. Callee does not derive the ID from its displayed name or kind.
4. Cancel the request for a nonnumeric or out-of-range selection. A blank response is prompted again; `/abort` aborts the workflow.
5. Resume the provider-turn timeout after the interaction, including cancellation and errors.

Providers define the choices in each request and may perform operations without requesting permission.

## Lifecycle logs

Callee writes one INFO event when an ACP permission request arrives and one terminal event when handling finishes:

- `permission request received` includes the effective Role `id`, policy, ACP session and tool-call IDs, title, tool kind, and option count.
- `permission request answered` includes the selected or cancelled outcome, selected option kind when applicable, and human-readable duration.
- `permission request failed` is an ERROR event with the failure and duration; an errored request does not also emit `answered`.

These diagnostics go to stderr and follow the normal text or `--json` logging format. They intentionally omit full content, raw tool input and output, commands, paths, option labels, and opaque option IDs. Full human-readable content is visible only in the controlling-TTY prompt for `ask`.

If no `permission request received` event appears, the provider did not send an ACP permission request to Callee. A Role permission mode controls how Callee answers a request; it does not force the provider to create one. In particular, a provider may execute operations already allowed by its sandbox without asking.

## Terminal and timeout requirements

`agent run` opens the controlling TTY before starting any provider, even when `--message`, parameters, and an automatic permission policy would avoid prompts. Permission input never comes from redirected stdin, and a run cannot start without a controlling TTY.

`--repl-timeout`, default `30m`, bounds each `ask` selection just like the initial prompt, missing Role parameters, and REPL responses. Reaching it returns an error rather than an ACP cancellation.

`spec.provider.timeout`, default `15m`, separately bounds process startup, session creation and preparation, and each provider turn. Only an interactive `ask` wait pauses the active turn budget. Automatic `allow` and `deny` decisions do not pause any timeout.

## Inspection and doctor

`callee agent view <agent-id>` reports both the authored permission object and the resolved effective policy for each Role occurrence. In text output, an omitted policy appears as `authoredPermissions=default permissions=ask`; JSON output uses `authoredPermissions` for the authored object and `permissions` for the resolved effective value.

`callee doctor` validates the field through normal schema and graph loading. Provider readiness checks create and prepare disposable sessions without a model prompt, so doctor does not trigger permission requests or prove that a provider will offer options compatible with an automatic policy.

See [Agent resource format](../reference/agent-resources.md) for Role fields and [Workflow semantics](../reference/workflow-semantics.md#tty-permissions-and-timeouts) for the surrounding run lifecycle.
