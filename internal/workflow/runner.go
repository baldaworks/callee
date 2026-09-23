package workflow

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	evaluationapi "github.com/baldaworks/callee/internal/evaluation"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const cleanupTimeout = 10 * time.Second

// Interactor owns all controlling-TTY input and display for a workflow run.
type Interactor interface {
	Prompt(ctx context.Context, label string) (string, error)
	Display(text string) error
}

// Runner executes one resolved root using shared ephemeral state.
type Runner struct {
	Root                *registry.ResolvedNode
	Factory             runtime.ProcessFactory
	Interactor          Interactor
	Params              map[string]string
	Pauses              *PauseController
	Metrics             *RunMetrics
	InteractiveOverride *bool
	PermissionOverride  *agent.PermissionMode
	EvaluationEvaluator evaluationapi.Evaluator
}

// Run executes the root and returns its sole final artifact only after every
// started provider process closes successfully.
func (r Runner) Run(ctx context.Context, prompt string) (artifact string, resultErr error) {
	metrics := r.Metrics
	if metrics == nil {
		metrics = &RunMetrics{}
	}

	metrics.reset()

	if r.Root == nil {
		return "", fmt.Errorf("workflow root is required")
	}

	if r.Factory == nil {
		return "", fmt.Errorf("workflow process factory is required")
	}

	if err := ValidatePolicyOverrides(PolicyOverrides{
		Interactive: r.InteractiveOverride,
		Permissions: r.PermissionOverride,
	}); err != nil {
		return "", err
	}

	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("workflow prompt must not be blank")
	}

	interactiveOverrideSet := r.InteractiveOverride != nil
	interactiveOverride := false

	if interactiveOverrideSet {
		interactiveOverride = *r.InteractiveOverride
	}

	permissionOverrideSet := r.PermissionOverride != nil

	permissionOverride := agent.PermissionMode("")
	if permissionOverrideSet {
		permissionOverride = *r.PermissionOverride
	}

	evaluationEvaluator := r.EvaluationEvaluator
	if evaluationEvaluator == nil {
		evaluationEvaluator = evaluationapi.NewEvaluator()
	}

	processCtx, cancelProcessStarts := context.WithCancel(ctx)
	run := &runState{
		prompt: prompt,
		state: map[string]any{
			"outputs":     map[string]string{},
			"scripts":     map[string]any{},
			"evaluations": map[string]any{},
		},
		factory:                r.Factory,
		processCtx:             processCtx,
		cancelProcessStarts:    cancelProcessStarts,
		interactor:             r.Interactor,
		params:                 copyStrings(r.Params),
		processes:              make(map[string]runtime.ProviderProcess),
		processStarts:          make(map[string]*processStart),
		visits:                 make(map[string]int),
		pauses:                 r.Pauses,
		metrics:                metrics,
		interactiveOverrideSet: interactiveOverrideSet,
		interactiveOverride:    interactiveOverride,
		permissionOverrideSet:  permissionOverrideSet,
		permissionOverride:     permissionOverride,
		evaluationEvaluator:    evaluationEvaluator,
	}

	if err := ValidateRuntimeParams(r.Root, run.params); err != nil {
		return "", err
	}

	if err := ValidateParallelPreflight(r.Root, run.params, PolicyOverrides{
		Interactive: r.InteractiveOverride,
		Permissions: r.PermissionOverride,
	}); err != nil {
		return "", err
	}

	defer func() {
		run.cancelProcessStarts()

		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if err := run.close(cleanupCtx); err != nil {
			artifact = ""
			resultErr = errors.Join(resultErr, fmt.Errorf("cleanup workflow providers: %w", err))
		}
	}()

	result, err := run.runADK(ctx, r.Root, prompt)
	if err != nil {
		return "", err
	}

	if result.outcome == outcomeEscalate {
		return "", fmt.Errorf("unconsumed escalation from agent %q (resource %q, path %q) reached root %q", result.sourceID, result.sourceResourceID, result.sourcePath, r.Root.ResourceID)
	}

	if result.outcome == outcomeFail {
		return "", fmt.Errorf("agent %q failed: %s", result.sourceID, diagnosticDetail(result.artifact))
	}

	if strings.TrimSpace(result.artifact) == "" {
		return "", fmt.Errorf("root agent %q returned an empty artifact", r.Root.ResourceID)
	}

	return result.artifact, nil
}

type nodeResult struct {
	outcome          outcome
	artifact         string
	sourceID         string
	sourceResourceID string
	sourcePath       string
	roleMetrics      roleMetrics
	evaluationTrace  evaluationapi.Trace
}

type startedProcess struct {
	key     string
	process runtime.ProviderProcess
	cancel  context.CancelFunc
}

type processStartResult struct {
	process runtime.ProviderProcess
	err     error
}

type processStart struct {
	done   chan struct{}
	result processStartResult
}

type runState struct {
	prompt                 string
	state                  map[string]any
	stateMu                sync.RWMutex
	factory                runtime.ProcessFactory
	processCtx             context.Context
	cancelProcessStarts    context.CancelFunc
	interactor             Interactor
	params                 map[string]string
	processes              map[string]runtime.ProviderProcess
	processStarts          map[string]*processStart
	started                []startedProcess
	processMu              sync.Mutex
	visits                 map[string]int
	visitMu                sync.Mutex
	pauses                 *PauseController
	metrics                *RunMetrics
	interactiveOverrideSet bool
	interactiveOverride    bool
	permissionOverrideSet  bool
	permissionOverride     agent.PermissionMode
	evaluationEvaluator    evaluationapi.Evaluator
}

type visitOptions struct {
	appendFinish func(*zerolog.Event) *zerolog.Event
}

func (r *runState) visit(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
	execute func() (nodeResult, error),
) (result nodeResult, resultErr error) {
	return r.visitWithOptions(ctx, node, input, visitOptions{}, execute)
}

func (r *runState) visitWithOptions(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
	opts visitOptions,
	execute func() (nodeResult, error),
) (result nodeResult, resultErr error) {
	r.visitMu.Lock()
	r.visits[node.EffectiveID]++
	visit := r.visits[node.EffectiveID]
	r.visitMu.Unlock()

	if node.Kind == agent.RoleKind {
		result.roleMetrics = newRoleMetrics(node.Resource.Spec.Provider)
	} else if node.Resource.IsEvaluation() {
		result.evaluationTrace = evaluationapi.Trace{
			Service: serviceForEvaluationKind(node.Kind), RequestedModel: node.Resource.EvaluationModel(),
		}
	}

	logger := r.lifecycleLoggerForVisit(ctx, node, visit)
	started := time.Now()

	logger.Info().Msg("running agent")

	defer func() {
		writeLifecycleFinish(logger, "agent finished", result, resultErr, started, agent.IsRoleKind(node.Kind), node.Resource.IsEvaluation(), opts.appendFinish)
	}()

	if err := r.applyState(node, input); err != nil {
		return result, err
	}

	return execute()
}

func (r *runState) lifecycleLogger(ctx context.Context, node *registry.ResolvedNode) zerolog.Logger {
	r.visitMu.Lock()
	visit := r.visits[node.EffectiveID]
	r.visitMu.Unlock()

	return r.lifecycleLoggerForVisit(ctx, node, visit)
}

func (r *runState) lifecycleLoggerForVisit(ctx context.Context, node *registry.ResolvedNode, visit int) zerolog.Logger {
	logger := log.Ctx(ctx).With().
		Str("id", node.EffectiveID).
		Str("kind", string(node.Kind)).
		Int("visit", visit).
		Logger()
	if node.EffectiveID != node.ResourceID {
		logger = logger.With().Str("ref", node.ResourceID).Logger()
	}

	return logger
}

func writeLifecycleFinish(
	logger zerolog.Logger,
	message string,
	result nodeResult,
	resultErr error,
	started time.Time,
	includeRoleMetrics bool,
	includeEvaluationTrace bool,
	appendFinish func(*zerolog.Event) *zerolog.Event,
) {
	event := logger.Info()

	if resultErr != nil {
		event.Str("status", "error")
	} else {
		status := "completed"

		if result.outcome == outcomeFail {
			status = "failed"
		}

		event.Str("status", status).Str("outcome", result.outcome.String())
	}

	if includeRoleMetrics {
		if result.roleMetrics.resolved {
			model, reasoning := roleConfigurationValue(result.roleMetrics.model), roleConfigurationValue(result.roleMetrics.reasoning)
			if result.roleMetrics.redactSelections {
				model, reasoning = "redacted", "redacted"
			}

			event = event.
				Str("role_provider", result.roleMetrics.provider).
				Str("role_model", model).
				Str("role_reasoning", reasoning)
		}

		event = appendUsageMetrics(event, "role", result.roleMetrics.usage)

		if result.roleMetrics.turnStarted {
			event = event.
				Dur("role_duration", logging.RoundElapsed(result.roleMetrics.duration)).
				Dur("role_wait_duration", logging.RoundElapsed(result.roleMetrics.wait))
		}
	}

	if includeEvaluationTrace {
		event = appendEvaluationTrace(event, result.evaluationTrace)
	}

	if appendFinish != nil {
		event = appendFinish(event)
	}

	event.Dur("duration", logging.RoundElapsed(time.Since(started))).Msg(message)
}

func appendEvaluationTrace(event *zerolog.Event, trace evaluationapi.Trace) *zerolog.Event {
	if trace.Service != "" {
		event = event.Str("evaluation_service", boundedLogValue(trace.Service))
	}

	if trace.RequestedModel != "" {
		event = event.Str("evaluation_requested_model", boundedLogValue(trace.RequestedModel))
	}

	if trace.Attempts > 0 {
		event = event.Int("evaluation_attempts", trace.Attempts)
	}

	if trace.Model != "" {
		event = event.Str("evaluation_model", boundedLogValue(trace.Model))
	}

	if trace.Provider != "" {
		event = event.Str("evaluation_provider", boundedLogValue(trace.Provider))
	}

	if trace.RequestID != "" {
		event = event.Str("evaluation_request_id", boundedLogValue(trace.RequestID))
	}

	if trace.Usage != nil {
		event = event.Int64("evaluation_input_tokens", trace.Usage.InputTokens).Int64("evaluation_output_tokens", trace.Usage.OutputTokens)
		if trace.Usage.Cost != nil {
			event = event.Float64("evaluation_cost", *trace.Usage.Cost)
		}
	}

	if trace.ErrorClass != "" {
		event = event.Str("evaluation_error_class", boundedLogValue(string(trace.ErrorClass)))
	}

	return event
}

func boundedLogValue(value string) string {
	const limit = 256

	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}

	return string(runes[:limit])
}

func appendUsageMetrics(event *zerolog.Event, prefix string, usage runtime.UsageMetrics) *zerolog.Event {
	event = event.Str(prefix+"_token_usage", string(usage.Status()))
	if usage.TurnsReported == 0 {
		return event
	}

	event = event.
		Int64(prefix+"_input_tokens", usage.InputTokens).
		Int64(prefix+"_output_tokens", usage.OutputTokens).
		Int64(prefix+"_total_tokens", usage.TotalTokens)
	if usage.CachedReadTokens != 0 {
		event = event.Int64(prefix+"_cached_read_tokens", usage.CachedReadTokens)
	}

	return event
}

func (r *runState) role(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
) (result nodeResult, resultErr error) {
	params, err := r.roleParams(ctx, node, input)
	if err != nil {
		return result, err
	}

	templateData := agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.snapshotState(),
		Params: params,
	}

	body, err := render(node.ResourceID+" spec.body", node.Resource.Spec.Body, templateData)
	if err != nil {
		return result, err
	}

	effective := node.Resource
	if node.Kind == agent.DynamicRoleKind {
		effective, err = node.Resource.RenderDynamicProvider(templateData)
		if err != nil {
			return result, err
		}
	}

	result.roleMetrics = newRoleMetrics(effective.Spec.Provider)
	result.roleMetrics.redactSelections = node.Kind == agent.DynamicRoleKind

	provider, err := runtime.ProviderForAgent(effective)
	if err != nil {
		if node.Kind == agent.DynamicRoleKind {
			return result, dynamicProviderError(node.EffectiveID, "runtime configuration rejected", err)
		}

		return result, err
	}

	process, err := r.process(ctx, effective, provider)
	if err != nil {
		if node.Kind == agent.DynamicRoleKind {
			return result, dynamicProviderError(node.EffectiveID, "process startup failed", err)
		}

		return result, err
	}

	session, err := r.newSession(ctx, node, effective, process)
	if session != nil {
		result.roleMetrics.applySessionConfiguration(session)
	}

	if err != nil {
		if node.Kind == agent.DynamicRoleKind {
			return result, dynamicProviderError(node.EffectiveID, "session setup failed", err)
		}

		return result, fmt.Errorf("agent %q: %w", node.EffectiveID, err)
	}

	interactive := r.roleInteractive(node)
	if interactive {
		logger := r.lifecycleLogger(ctx, node)
		started := time.Now()

		logger.Info().Msg("entering repl")

		defer func() {
			writeLifecycleFinish(logger, "exiting repl", result, resultErr, started, false, false, nil)
		}()
	}

	turnInput := body + controlInstructions(interactive, node.WithinLoop, node.CanEscalate)
	providerTimeout := effective.ProviderTimeout()
	turnCtx, cancelTurn := withActiveTimeout(ctx, providerTimeout, r.pauses)
	roleStarted := time.Now()
	waitStarted := r.operatorWaitDuration()
	result.roleMetrics.turnStarted = true

	defer func() {
		result.roleMetrics.applySessionConfiguration(session)
		result.roleMetrics.duration = time.Since(roleStarted)
		result.roleMetrics.wait = r.operatorWaitDuration() - waitStarted
		r.metrics.add(result.roleMetrics.usage)
	}()

	return r.runRoleTurns(ctx, node, session, turnInput, turnCtx, cancelTurn, providerTimeout, interactive, result)
}

func dynamicProviderError(id, stage string, err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("agent %q: spec.provider %s: %w", id, stage, context.Canceled)
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("agent %q: spec.provider %s: %w", id, stage, context.DeadlineExceeded)
	default:
		return fmt.Errorf("agent %q: spec.provider %s", id, stage)
	}
}

func (r *runState) roleInteractive(node *registry.ResolvedNode) bool {
	return r.rolePolicy(node).Interactive
}

func (r *runState) rolePolicy(node *registry.ResolvedNode) RolePolicy {
	overrides := PolicyOverrides{}
	if r.interactiveOverrideSet {
		overrides.Interactive = &r.interactiveOverride
	}

	if r.permissionOverrideSet {
		overrides.Permissions = &r.permissionOverride
	}

	policy, _ := ResolveNodeRolePolicy(node, overrides)

	return policy
}

func (r *runState) runRoleTurns(
	ctx context.Context,
	node *registry.ResolvedNode,
	session runtime.AgentSession,
	turnInput string,
	turnCtx context.Context,
	cancelTurn context.CancelFunc,
	providerTimeout time.Duration,
	interactive bool,
	result nodeResult,
) (nodeResult, error) {
	for {
		logger := r.lifecycleLogger(ctx, node)
		turnResult, err := runTurnWithHeartbeat(turnCtx, logger, session, turnInput)

		cancelTurn()
		result.roleMetrics.usage.AddTurn(turnResult.Usage)

		if err != nil {
			return result, fmt.Errorf("agent %q turn: %w", node.EffectiveID, err)
		}

		parsed, err := parseResponse(turnResult.Content, interactive)
		if err != nil {
			return result, fmt.Errorf("agent %q: %w", node.EffectiveID, err)
		}

		if parsed.outcome == outcomeEscalate && !node.CanEscalate {
			return result, fmt.Errorf(
				"agent %q (resource %q, path %q) attempted unauthorized escalation",
				node.EffectiveID,
				node.ResourceID,
				strings.Join(node.Path, " -> "),
			)
		}

		switch parsed.outcome {
		case outcomeAwait:
			if r.interactor == nil {
				return result, fmt.Errorf("agent %q requested operator input without an interactor", node.EffectiveID)
			}

			if err := r.interactor.Display(parsed.artifact); err != nil {
				return result, fmt.Errorf("display agent %q response: %w", node.EffectiveID, err)
			}

			answer, err := r.interactor.Prompt(ctx, node.EffectiveID+" response")
			if err != nil {
				return result, fmt.Errorf("read agent %q response: %w", node.EffectiveID, err)
			}

			turnInput = answer + replReminder()
			turnCtx, cancelTurn = withActiveTimeout(ctx, providerTimeout, r.pauses)
		case outcomeReturn, outcomeEscalate:
			if strings.TrimSpace(parsed.artifact) != "" {
				r.promote(node.EffectiveID, parsed.artifact)
			}

			result.outcome = parsed.outcome
			result.artifact = parsed.artifact
			result.sourceID = node.EffectiveID
			result.sourceResourceID = node.ResourceID
			result.sourcePath = strings.Join(node.Path, " -> ")

			return result, nil
		case outcomeFail:
			result.outcome = outcomeFail
			result.artifact = parsed.artifact
			result.sourceID = node.EffectiveID
			result.sourceResourceID = node.ResourceID
			result.sourcePath = strings.Join(node.Path, " -> ")

			return result, nil
		}
	}
}

func (r *runState) operatorWaitDuration() time.Duration {
	waits, ok := r.interactor.(interface{ WaitDuration() time.Duration })
	if !ok {
		return 0
	}

	return waits.WaitDuration()
}

func (r *runState) newSession(
	ctx context.Context,
	node *registry.ResolvedNode,
	effective agent.Resource,
	process runtime.ProviderProcess,
) (runtime.AgentSession, error) {
	sessionCtx, cancelSession := context.WithTimeout(ctx, effective.ProviderTimeout())
	defer cancelSession()

	role := effective
	policy := r.rolePolicy(node)
	interactive := policy.Interactive
	role.Spec.Interactive = &interactive
	role.Spec.LegacyREPL = nil
	role.Spec.Permissions = &agent.Permissions{Mode: policy.Permissions}

	session, err := process.NewSession(sessionCtx, role, node.EffectiveID)
	if err != nil {
		return nil, err
	}

	if err := session.Prepare(sessionCtx); err != nil {
		return session, err
	}

	return session, nil
}

func (r *runState) compositeInput(node *registry.ResolvedNode, input string) (string, error) {
	localInput, err := render(node.ResourceID+" spec.body", node.Resource.Spec.Body, agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.snapshotState(),
	})
	if err != nil {
		return "", err
	}

	if strings.TrimSpace(localInput) == "" {
		return "", fmt.Errorf("agent %q spec.body rendered an empty input", node.EffectiveID)
	}

	return localInput, nil
}

func (r *runState) childInput(parent, child *registry.ResolvedNode, index int, localInput, previous string) (string, error) {
	if child.Edge.Input == "" {
		if index == 0 {
			return previous, nil
		}

		return previous, nil
	}

	return render(parent.ResourceID+" child "+child.EffectiveID+" input", child.Edge.Input, agent.TemplateData{
		Prompt: r.prompt,
		Input:  localInput,
		State:  r.snapshotState(),
	})
}

func (r *runState) finishComposite(node *registry.ResolvedNode, localInput, naturalOutput string) (nodeResult, error) {
	output := naturalOutput
	if node.Resource.Spec.Output != "" {
		rendered, err := renderOutput(node.ResourceID+" spec.output", node.Resource.Spec.Output, agent.TemplateData{
			Prompt: r.prompt,
			Input:  localInput,
			Output: naturalOutput,
			State:  r.snapshotState(),
		})
		if err != nil {
			return nodeResult{}, err
		}

		output = rendered
	}

	if strings.TrimSpace(output) == "" {
		return nodeResult{}, fmt.Errorf("agent %q returned an empty artifact", node.EffectiveID)
	}

	r.promote(node.EffectiveID, output)

	return nodeResult{
		outcome:          outcomeReturn,
		artifact:         output,
		sourceID:         node.EffectiveID,
		sourceResourceID: node.ResourceID,
		sourcePath:       strings.Join(node.Path, " -> "),
	}, nil
}

func (r *runState) applyState(node *registry.ResolvedNode, input string) error {
	effective := make(map[string]any, len(node.Resource.Spec.State)+len(node.Edge.State))
	for key, value := range node.Resource.Spec.State {
		effective[key] = value
	}

	for key, value := range node.Edge.State {
		effective[key] = value
	}

	if len(effective) == 0 {
		return nil
	}

	snapshot := r.snapshotState()

	rendered := make(map[string]any, len(effective))
	for _, key := range sortedStateKeys(effective) {
		value := effective[key]

		converted, err := renderStateValue(node.EffectiveID+" state."+key, value, agent.TemplateData{
			Prompt: r.prompt,
			Input:  input,
			State:  snapshot,
		})
		if err != nil {
			return err
		}

		rendered[key] = converted
	}

	r.stateMu.Lock()
	for key, value := range rendered {
		r.state[key] = value
	}
	r.stateMu.Unlock()

	return nil
}

func (r *runState) roleParams(ctx context.Context, node *registry.ResolvedNode, input string) (map[string]string, error) {
	values := make(map[string]string, len(node.Resource.Spec.Params))
	names := make([]string, 0, len(node.Resource.Spec.Params))

	for name := range node.Resource.Spec.Params {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		if binding, ok := node.Edge.Params[name]; ok {
			value, err := renderRestricted(node.ResourceID+" parameter "+name, binding, agent.TemplateData{
				Prompt: r.prompt,
				Input:  input,
				State:  r.snapshotState(),
			})
			if err != nil {
				return nil, err
			}

			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("agent %q parameter %q binding rendered blank", node.EffectiveID, name)
			}

			values[name] = value

			continue
		}

		key := node.EffectiveID + "." + name

		value, ok := r.params[key]
		if !ok {
			if node.WithinParallel {
				return nil, fmt.Errorf("Parallel %q requires parameter %q before fan-out", node.ParallelBoundaryID, key)
			}

			if r.interactor == nil {
				return nil, fmt.Errorf("agent %q requires parameter %q", node.EffectiveID, name)
			}

			var err error

			value, err = r.interactor.Prompt(ctx, key+" — "+strings.TrimSpace(node.Resource.Spec.Params[name]))
			if err != nil {
				return nil, fmt.Errorf("read parameter %q: %w", key, err)
			}

			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("parameter %q must not be blank", key)
			}

			r.params[key] = value
		}

		values[name] = value
	}

	return values, nil
}

func (r *runState) process(ctx context.Context, role agent.Resource, provider runtime.Provider) (runtime.ProviderProcess, error) {
	key := provider.Key()

	r.processMu.Lock()
	if process := r.processes[key]; process != nil {
		r.processMu.Unlock()

		return process, nil
	}

	start := r.processStarts[key]
	if start == nil {
		start = &processStart{done: make(chan struct{})}

		r.processStarts[key] = start
		go r.completeProcessStart(role, provider, start)
	}
	r.processMu.Unlock()

	select {
	case <-start.done:
		if start.result.err != nil {
			return nil, fmt.Errorf("start agent %q provider: %w", role.ID, start.result.err)
		}

		return start.result.process, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (r *runState) completeProcessStart(role agent.Resource, provider runtime.Provider, start *processStart) {
	process, cancel, err := startProviderProcess(r.processCtx, role.ProviderTimeout(), r.factory, provider)
	key := provider.Key()

	r.processMu.Lock()

	start.result = processStartResult{process: process, err: err}
	if err == nil {
		r.processes[key] = process
		r.started = append(r.started, startedProcess{key: key, process: process, cancel: cancel})
	}

	close(start.done)
	delete(r.processStarts, key)
	r.processMu.Unlock()
}

func startProviderProcess(ctx context.Context, timeout time.Duration, factory runtime.ProcessFactory, provider runtime.Provider) (runtime.ProviderProcess, context.CancelFunc, error) {
	lifetimeCtx, cancelLifetime := context.WithCancel(context.WithoutCancel(ctx))
	started := make(chan processStartResult, 1)

	go func() {
		process, err := factory.Start(lifetimeCtx, provider)
		started <- processStartResult{process: process, err: err}
	}()

	startupCtx, cancelStartup := context.WithTimeout(ctx, timeout)
	defer cancelStartup()

	select {
	case result := <-started:
		if result.err != nil {
			cancelLifetime()

			return nil, nil, result.err
		}

		return result.process, cancelLifetime, nil
	case <-startupCtx.Done():
		cancelLifetime()

		go closeLateProcess(started)

		return nil, nil, startupCtx.Err()
	}
}

func closeLateProcess(started <-chan processStartResult) {
	result := <-started
	if result.process != nil {
		_ = result.process.Close()
	}
}

func (r *runState) close(ctx context.Context) error {
	waitErr := r.waitForProcessStarts(ctx)

	r.processMu.Lock()
	startedProcesses := append([]startedProcess(nil), r.started...)
	r.processMu.Unlock()

	type keyedError struct {
		key string
		err error
	}

	keyed := make([]keyedError, 0, len(startedProcesses))

	for index := len(startedProcesses) - 1; index >= 0; index-- {
		started := startedProcesses[index]

		closeErr := closeProcess(ctx, started.process)
		started.cancel()

		if closeErr != nil {
			keyed = append(keyed, keyedError{key: started.key, err: closeErr})
		}
	}

	sort.Slice(keyed, func(i, j int) bool { return keyed[i].key < keyed[j].key })

	errs := make([]error, 0, len(keyed)+1)
	if waitErr != nil {
		errs = append(errs, fmt.Errorf("wait for provider starts: %w", waitErr))
	}

	for _, item := range keyed {
		errs = append(errs, fmt.Errorf("close provider group %q: %w", item.key, item.err))
	}

	return errors.Join(errs...)
}

func (r *runState) waitForProcessStarts(ctx context.Context) error {
	for {
		r.processMu.Lock()

		starts := make([]*processStart, 0, len(r.processStarts))
		for _, start := range r.processStarts {
			starts = append(starts, start)
		}
		r.processMu.Unlock()

		if len(starts) == 0 {
			return nil
		}

		for _, start := range starts {
			select {
			case <-start.done:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

func closeProcess(ctx context.Context, process runtime.ProviderProcess) error {
	closed := make(chan error, 1)

	go func() {
		closed <- process.Close()
	}()

	select {
	case err := <-closed:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *runState) promote(effectiveID, artifact string) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	outputs := r.state["outputs"].(map[string]string)
	outputs[effectiveID] = artifact
}

func (r *runState) snapshotState() map[string]any {
	r.stateMu.RLock()
	defer r.stateMu.RUnlock()

	return cloneState(r.state)
}

// ValidateRuntimeParams validates supplied qualified parameter values.
func ValidateRuntimeParams(root *registry.ResolvedNode, values map[string]string) error {
	known := make(map[string]bool)
	bound := make(map[string]bool)

	var visit func(*registry.ResolvedNode)

	visit = func(node *registry.ResolvedNode) {
		if agent.IsRoleKind(node.Kind) {
			for name := range node.Resource.Spec.Params {
				key := node.EffectiveID + "." + name
				known[key] = true
				_, bound[key] = node.Edge.Params[name]
			}
		}

		for _, child := range node.Children {
			visit(child)
		}
	}
	visit(root)

	for key, value := range values {
		switch {
		case !known[key]:
			return fmt.Errorf("unknown workflow parameter %q", key)
		case bound[key]:
			return fmt.Errorf("workflow parameter %q is already bound by its child occurrence", key)
		case strings.TrimSpace(value) == "":
			return fmt.Errorf("workflow parameter %q must not be blank", key)
		}
	}

	return nil
}

func render(name, body string, data agent.TemplateData) (string, error) {
	parsed, err := agent.ParseTemplate(name, body)
	if err != nil {
		return "", err
	}

	return agent.RenderTemplate(parsed, data)
}

func renderRestricted(name, body string, data agent.TemplateData) (string, error) {
	parsed, err := agent.ParseRestrictedTemplate(name, body)
	if err != nil {
		return "", err
	}

	return agent.RenderTemplate(parsed, data)
}

func renderOutput(name, body string, data agent.TemplateData) (string, error) {
	parsed, err := agent.ParseOutputTemplate(name, body)
	if err != nil {
		return "", err
	}

	return agent.RenderTemplate(parsed, data)
}

func renderStateValue(name string, value any, data agent.TemplateData) (any, error) {
	switch typed := value.(type) {
	case string:
		return renderRestricted(name, typed, data)
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			converted, err := renderStateValue(fmt.Sprintf("%s[%d]", name, index), item, data)
			if err != nil {
				return nil, err
			}

			result[index] = converted
		}

		return result, nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for _, key := range sortedStateKeys(typed) {
			item := typed[key]

			converted, err := renderStateValue(name+"."+key, item, data)
			if err != nil {
				return nil, err
			}

			result[key] = converted
		}

		return result, nil
	default:
		return typed, nil
	}
}

func cloneState(state map[string]any) map[string]any {
	return cloneStateValue(state).(map[string]any)
}

func cloneStateValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, item := range typed {
			cloned[key] = cloneStateValue(item)
		}

		return cloned
	case map[string]string:
		cloned := make(map[string]string, len(typed))
		for key, item := range typed {
			cloned[key] = item
		}

		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for index, item := range typed {
			cloned[index] = cloneStateValue(item)
		}

		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return typed
	}
}

func copyStrings(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}

	return result
}

func sortedStateKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func diagnosticDetail(detail string) string {
	if strings.TrimSpace(detail) == "" {
		return "workflow control failure"
	}

	return detail
}
