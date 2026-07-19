# ACP provider configuration

Each `Role` selects one ACP backend under `spec.provider`. Coding-host setup is separate: installing Callee skills into a host does not install a provider executable or authenticate it.

## Supported providers

| `type` | Default resolved command | Runtime prerequisite |
| --- | --- | --- |
| `codex` | Current Callee executable followed by `bridge codex` | Installed and authenticated Codex CLI. |
| `claude` | `npx -y @zed-industries/claude-code-acp@latest` | Node.js with `npx`, registry access for an uncached adapter, and adapter-accepted credentials. |
| `opencode` | `opencode acp` | Installed and authenticated OpenCode CLI. |
| `copilot` | `copilot --acp --stdio` | Installed and authenticated Copilot CLI. |
| `grok` | `grok agent stdio` | Installed Grok CLI authenticated by `grok login` or `XAI_API_KEY`. |
| `cursor` | `agent acp` | Installed and authenticated Cursor CLI. |
| `generic_acp` | None | A nonblank `cmd` naming an ACP executable. |

The supported type list is closed. Gemini is not a valid provider type.

## Provider fields

```yaml
provider:
  type: generic_acp
  cmd: my-acp-agent
  model: provider-model
  reasoning: high
  mode: review
  extraArgs:
    - --stdio
  timeout: 20m
```

| Field | Contract |
| --- | --- |
| `type` | Required supported public type. |
| `cmd` | Optional executable override. `generic_acp` requires a nonblank value. |
| `model` | Optional backend-specific session model. |
| `reasoning` | Optional backend-specific reasoning effort. |
| `mode` | Optional backend-specific session mode. |
| `extraArgs` | Optional ordered arguments appended to the resolved command; entries must be nonblank. |
| `timeout` | Optional positive Go duration; defaults to `15m`. |

`cmd` is a single executable string, not a shell command line. Put each argument in `extraArgs`. Empty model, reasoning, and mode values defer to the backend. Nonempty values are passed as ACP session configuration selections, but whether a value is accepted is provider-specific.

Provider configuration must remain nested. Flat provider fields in `spec` are not supported.

## Command resolution and reuse

Norma Runtime supplies the built-in command defaults for `claude`, `opencode`, `copilot`, and `grok`. Callee sets the Cursor default explicitly and replaces the Codex default with its own current executable plus `bridge codex`. A `cmd` override replaces the default executable while `extraArgs` remain ordered appended arguments.

Within one root run, Roles with the same public provider type and fully resolved command reuse one provider process. Model, mode, and reasoning select fresh session configuration and do not contribute to provider process identity. Every Role visit still creates and prepares a fresh session.

## Codex bridge

The embedded Codex bridge avoids a separate bridge-package download:

```yaml
provider:
  type: codex
  model: provider-model
  reasoning: high
  mode: review
```

The bridge process in turn starts the installed Codex CLI. Use `callee bridge codex --help` for bridge-specific controls such as message streaming, reasoning projection, and forwarded `codex app-server` arguments.

To test an external ACP bridge instead, override the executable and provide every argument separately:

```yaml
provider:
  type: codex
  cmd: npx
  extraArgs:
    - -y
    - '@normahq/codex-acp-bridge@1.7.7'
```

Pin external packages in durable resources when reproducibility matters.

## Timeout behavior

The effective provider timeout applies independently to:

- starting the provider process;
- creating and preparing a Role visit session;
- each provider turn.

It does not bound an entire root run. Repeated Loop visits and REPL turns each receive their own turn timeout. Operator interaction has a separate CLI timeout controlled by `--repl-timeout`; permission waits pause active turn timeout accounting.

## Permissions

ACP providers may request operator permission. During `agent run`, Callee renders the provider's options on the controlling TTY and returns the selected option ID. Invalid selections and requests with no choices are cancelled. This is an interactive runtime path; there is no resource field for pre-authorizing permissions.

## Validate provider readiness

Use doctor after resource and graph validation:

```bash
callee doctor
callee doctor --timeout 90s
```

Doctor groups Roles by provider process identity, starts each distinct process, and creates disposable sessions for distinct model/mode/reasoning configurations. It verifies session binding without sending a model prompt, then closes the process. Successful output names every Role and ends with `callee doctor: ok`.

Graph-only doctor modes do not check providers:

```bash
callee doctor --graph text
```

## Troubleshooting

| Failure | Interpretation |
| --- | --- |
| Executable not found | The resolved first command is absent from `PATH`. |
| Startup timeout | The ACP process did not initialize within `provider.timeout` or doctor's `--timeout`. |
| Session binding failure | The backend did not complete ACP session creation/preparation as expected. |
| Session configuration rejected | The selected model, mode, or reasoning value is unsupported by that backend. |
| Permission request cancelled | No option was supplied or the operator did not select a valid numbered choice. |
| Cleanup error | The provider did not close cleanly; a run suppresses any successful artifact in this case. |

Use `--debug` or `--trace` for Callee diagnostics. Provider stderr is forwarded separately from the successful stdout artifact.
