# Typed evaluation examples

These resources are runnable directly from the repository root. They call the
native TypeSafe System One or OpenRouter Decisions API; no ACP provider is
started.

List and inspect the examples:

```bash
callee --agent-root examples agent list --kind TypeSafeJev
callee --agent-root examples agent list --kind OpenRouterDecision
callee --agent-root examples agent view evaluations/typesafe-triage
```

## TypeSafe Jev

Set the native TypeSafe credential, then run a single batched judgment:

```bash
export TYPESAFE_API_KEY="<your-key>"
callee --agent-root examples agent run evaluations/typesafe-jev \
  --message "Production checkout is returning HTTP 500 for every customer"
```

`typesafe-jev` asks both a Noul question and a Choice question in one request.
It pins `jev-1.13.0`; remove `spec.model` to use `TYPESAFE_DEFAULT_MODEL` and
then `jev-latest`. Set `TYPESAFE_BASE_URL` only when TypeSafe is available
through a trusted HTTPS proxy root.

The Sequential example makes two TypeSafe calls. The first classifies urgency
and priority. The second receives the original request plus the first step's
validated JSON result and chooses a response action:

```bash
callee --agent-root examples agent run evaluations/typesafe-triage \
  --message "A customer reports a suspected account takeover"
```

Its final stdout artifact is the second typed evaluation. During the run,
structured results are also available at `.State.evaluations.triage` and
`.State.evaluations.recommendation`; their compact JSON forms are stored at the
matching `.State.outputs` keys.

## OpenRouter Decisions

Set the OpenRouter credential and run the Decisions example:

```bash
export OPENROUTER_API_KEY="<your-key>"
callee --agent-root examples agent run evaluations/openrouter-decision \
  --message "Release changes authentication and has no rollback test"
```

This example selects `typesafe/jev-1.13`, but `OpenRouterDecision` accepts any
model identifier supported by the Decisions endpoint. It demonstrates Noul and
Score questions and preserves OpenRouter request, provider, token, and cost
metadata when the service returns them.

To use the npm launcher without a global install, replace `callee` in any
command with `npx --yes @baldaworks/callee@latest`.
