# Execution metrics

Use the lifecycle fields emitted by `callee agent run` to inspect a complete command or an individual Role or DynamicRole visit. Run-wide fields use the `agent_` prefix; per-visit fields use `role_`.

Metrics are INFO-level structured fields on stderr events. They do not alter
stdout: a successful run writes only the root artifact to stdout, after
workflow execution and cleanup succeed and after the final metrics event is
emitted. A nonempty stderr stream is expected, so use the command exit status
to determine success.

## Events and scopes

| Event | Metrics | Scope |
| --- | --- | --- |
| `agent finished` for a `Role` or `DynamicRole` | `role_*` | One visit to that provider-backed occurrence. Repeated Loop visits have separate scopes. |
| `agent finished` for `TypeSafeJev` or `OpenRouterDecision` | None | Evaluation visits retain general lifecycle fields and add bounded `evaluation_*` operational fields. These fields are not metrics and are not aggregated. |
| `agent finished` for `Script`, `Human`, `Sequential`, `Parallel`, `Loop`, or `Router` | None | Other lifecycle events retain their general `duration` field but do not receive `role_*` fields. Parallel additionally reports bounded `parallel_*` lifecycle fields. |
| `agent run finished` | `agent_*` | The complete `agent run` command, including every Role or DynamicRole visit reached by the selected root. |

A Role or DynamicRole selected directly as the root has the same `role_*` behavior as one nested under `Sequential`, `Parallel`, `Loop`, or `Router`. Aliases and repeated visits do not change the field meanings. Each visit reports separately, while the final `agent_*` token fields aggregate all attempted provider turns across all visits.

The final `agent run finished` event also has `status=completed` or `status=error` for work inside the agent metric boundary. A successful artifact is written afterward, so a stdout write failure can still make the command exit unsuccessfully after the metrics event reported `status=completed`; always use the command exit status as the final automation signal.

## Duration boundaries

Durations are wall-clock measurements rounded to the nearest second at the logging boundary and rendered as Go duration strings, such as `2s` or `0s`. Internal measurement and timeout handling retain their original precision. A positive duration below 500 milliseconds therefore appears as `0s`.

| Field | Start | End | Presence |
| --- | --- | --- | --- |
| `role_duration` | Immediately before the first provider turn in one provider-backed visit, after parameter resolution, rendering, process startup, and session creation and preparation. | When the visit returns an artifact or control outcome, or when the in-scope turn or REPL processing returns an error. | Present only if the visit reaches its first provider turn. |
| `role_wait_duration` | Same boundary as `role_duration`. | Same boundary as `role_duration`. | Present with `role_duration`; `0s` is valid. |
| `agent_duration` | Entry to the `agent run` command handler. | After workflow execution and cleanup of started providers, immediately before the final metrics event and any successful stdout artifact. | Always present on `agent run finished`. |
| `agent_wait_duration` | Same boundary as `agent_duration`. | Same boundary as `agent_duration`. | Always present on `agent run finished`; `0s` is valid. |

The existing unprefixed `duration` on an `agent finished` event covers the entire node visit. It begins earlier than `role_duration`, so the two fields are not interchangeable.

Both prefixed duration fields include operator wait that occurs inside their boundaries. Do not treat `duration - wait_duration` as provider execution time: the remainder can also include Callee orchestration, parsing, state updates, and cleanup within the applicable scope.

## Operator wait semantics

Wait duration measures elapsed time inside controlling-TTY prompt calls, including retries for blank input and prompts that end in timeout, abort, terminal closure, or another read error.

Only an interactive run opens the controlling TTY and can accumulate operator
wait. A non-interactive run performs no terminal reads, so its run wait is zero
and every reached one-shot Role has zero Role wait.

`agent_wait_duration` accumulates every operator prompt reached by the command:

- the initial prompt when `--message` is omitted;
- missing Role or DynamicRole parameters;
- Human-node responses;
- responses requested by a REPL Role after `callee.control.v1.await`;
- numbered ACP permission selections under `permissions.mode: ask`.

`role_wait_duration` is the portion of that same accumulated prompt time that occurs after the Role's first turn starts and before the visit ends. It therefore includes REPL responses and interactive permission selections for that visit, but excludes the initial prompt and missing parameter prompts, which occur before the Role timing boundary.

Automatic `allow` and `deny` permission decisions do not prompt and add no wait. An `ask` request with no options is cancelled without prompting and also adds no wait. Wait metrics do not change timeout behavior: [`--repl-timeout` and provider timeouts](workflow-semantics.md#tty-permissions-and-timeouts) retain their separate semantics, including pausing the active provider-turn timeout during an interactive permission decision.

## Provider-selection fields

Every successfully resolved Role-family `agent finished` event identifies the effective provider selections for that visit:

| Field | Meaning |
| --- | --- |
| `role_provider` | The visit's validated public provider type, such as `codex` or `generic_acp`. This value comes from the effective resource, not ACP session configuration. |
| `role_model` | The latest concrete model selection observed in ACP session configuration during preparation or a turn; otherwise the Role's explicit `spec.provider.model`. |
| `role_reasoning` | The latest concrete reasoning selection observed in ACP session configuration during preparation or a turn; otherwise the Role's explicit `spec.provider.reasoning`. ACP providers can report this selection as `reasoning`, `reasoning_effort`, or `thought_level`. |

Callee resolves model and reasoning independently. For each field, the latest concrete ACP value wins over the effective resource value. If ACP does not report a concrete value, the static or rendered selection remains the fallback. Only when neither source supplies a concrete value does Callee emit `backend-default`. This marker does not identify or make a claim about the backend's private default. `role_provider` is always the validated effective provider type and does not use the marker.

These three fields are present even when a static Role fails before its first
provider turn. A DynamicRole reports them after provider rendering succeeds,
using the effective rendered values. If rendering or validation fails before an
effective provider exists, its event omits these three fields rather than
logging authored template text. It still reports `role_token_usage=unavailable`.
Root and nested provider-backed leaves use the same resolution rules. Other
kinds do not receive `role_*` fields. Evaluation operational data appears only
as `evaluation_*` lifecycle log fields and is not aggregated into run metrics.

## Evaluation operational fields

A TypeSafeJev or OpenRouterDecision `agent finished` event adds safe
request-level fields when the corresponding value is available:

| Field | Meaning |
| --- | --- |
| `evaluation_service` | Stable service discriminator: `typesafe` or `openrouter`. |
| `evaluation_requested_model` | Model selected by the resource and environment resolution rules. |
| `evaluation_attempts` | Number of HTTP attempts made during this visit. |
| `evaluation_model` | Actual model reported by the service. |
| `evaluation_provider` | Provider metadata returned by OpenRouter. |
| `evaluation_request_id` | Bounded request identifier returned by the service. |
| `evaluation_input_tokens`, `evaluation_output_tokens` | Service-reported token usage for this request. |
| `evaluation_cost` | Service-reported cost when supplied. |
| `evaluation_error_class` | Bounded `configuration`, `authentication`, `rate_limit`, `transport`, `timeout`, `provider`, `response`, or `canceled` class on failure. |

These are lifecycle log fields rather than `role_*` or `agent_*` metrics.
They never include evidence, questions, answers, credentials, headers, state,
artifacts, sensitive endpoints, or raw provider errors.

## Token fields and aggregation

Callee reads provider usage metadata from a turn's final response. It records one attempted turn whenever a provider turn returns, whether or not that turn supplied usage or ended with an error. For every reported turn, Callee independently sums the provider's input, output, total, and cached-read values; it does not derive one token field from another.

| Role field | Run field | Meaning |
| --- | --- | --- |
| `role_token_usage` | `agent_token_usage` | Reporting completeness: `complete`, `partial`, or `unavailable`. |
| `role_input_tokens` | `agent_input_tokens` | Sum of provider-reported input tokens. |
| `role_output_tokens` | `agent_output_tokens` | Sum of provider-reported output tokens. |
| `role_total_tokens` | `agent_total_tokens` | Sum of provider-reported total tokens. |
| `role_cached_read_tokens` | `agent_cached_read_tokens` | Sum of provider-reported cached-read tokens. |

The Role fields aggregate attempted turns within one visit, including multiple turns in a REPL session. The run fields merge every Role or DynamicRole visit reached by the root, including repeated and nested visits.

### Reporting status and optional fields

| Status | Condition | Numeric token fields |
| --- | --- | --- |
| `complete` | At least one turn was attempted and every attempted turn reported usage. | Input, output, and total fields are present. |
| `partial` | At least one attempted turn reported usage and at least one did not. | Input, output, and total fields contain sums from reported turns only. |
| `unavailable` | No attempted turn reported usage, including a scope with no attempted turns. | Input, output, total, and cached-read fields are absent. |

`role_token_usage` is present on every Role and DynamicRole `agent finished`
event, even when the visit fails before its first provider turn and Role
duration fields are absent. `agent_token_usage` is always present on `agent run finished`.

When at least one turn reports usage, input, output, and total fields are emitted even when their aggregate value is zero. The cached-read field is more selective: `role_cached_read_tokens` or `agent_cached_read_tokens` appears only when its aggregate is nonzero. Its absence does not change the reporting status.

## Output example

The console logger renders structured fields as `key=value` pairs. A successful one-visit run with provider usage produces events shaped like:

```text
INF agent finished id=roles/worker kind=Role visit=1 status=completed outcome=return role_provider=codex role_model=gpt-5.6-sol role_reasoning=high role_token_usage=complete role_input_tokens=11 role_output_tokens=3 role_total_tokens=17 role_cached_read_tokens=4 role_duration=2s role_wait_duration=0s duration=3s
INF agent run finished id=roles/worker agent_duration=3s agent_wait_duration=0s agent_token_usage=complete status=completed agent_input_tokens=11 agent_output_tokens=3 agent_total_tokens=17 agent_cached_read_tokens=4
```

Field order is not an interface. Select records by their event message and read fields by name.

A Role that fails during parameter resolution, rendering, provider startup, session creation, or preparation still reports its provider-selection and token-status fields. It has no Role timing fields because no provider turn started:

```text
INF agent finished id=roles/worker kind=Role visit=1 status=error role_provider=codex role_model=backend-default role_reasoning=high role_token_usage=unavailable duration=0s
```

The exact error is reported separately. A non-Role `agent finished` event
retains its unprefixed lifecycle `duration` but does not copy these `role_*`
fields.

Parallel finish events add `parallel_branches`, `parallel_started`, `parallel_completed`, `parallel_failed`, and `parallel_joined`. These fields describe one activation after its branches drain. They contain counts and a boolean only; prompts, state, artifacts, and permission payloads are excluded. Parallel adds no metrics.

## No tool metrics

Callee deliberately does not emit `tool_*` execution metrics. It does not count tool calls or report per-tool durations, token usage, success rates, or input/output sizes. ACP permission lifecycle events may identify a tool call so an operator can audit the decision, but those identifiers are diagnostics rather than metrics.

Provider turn totals remain provider-reported aggregates. Callee does not attempt to isolate the portion attributable to tool activity.
