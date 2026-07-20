package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/baldaworks/callee/internal/agent"
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
	Root       *registry.ResolvedNode
	Factory    runtime.ProcessFactory
	Interactor Interactor
	Params     map[string]string
	Pauses     *PauseController
}

// Run executes the root and returns its sole final artifact only after every
// started provider process closes successfully.
func (r Runner) Run(ctx context.Context, prompt string) (artifact string, resultErr error) {
	if r.Root == nil {
		return "", fmt.Errorf("workflow root is required")
	}

	if r.Factory == nil {
		return "", fmt.Errorf("workflow process factory is required")
	}

	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("workflow prompt must not be blank")
	}

	run := &runState{
		prompt: prompt,
		state: map[string]any{
			"outputs": map[string]string{},
		},
		factory:    r.Factory,
		interactor: r.Interactor,
		params:     copyStrings(r.Params),
		processes:  make(map[string]runtime.ProviderProcess),
		visits:     make(map[string]int),
		pauses:     r.Pauses,
	}

	if err := validateRuntimeParams(r.Root, run.params); err != nil {
		return "", err
	}

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()

		if err := run.close(cleanupCtx); err != nil {
			artifact = ""
			resultErr = errors.Join(resultErr, fmt.Errorf("cleanup workflow providers: %w", err))
		}
	}()

	result, err := run.node(ctx, r.Root, prompt, executionContext{})
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

type runState struct {
	prompt     string
	state      map[string]any
	factory    runtime.ProcessFactory
	interactor Interactor
	params     map[string]string
	processes  map[string]runtime.ProviderProcess
	started    []startedProcess
	visits     map[string]int
	pauses     *PauseController
}

type executionContext struct {
	sessionScopes map[string]*loopSessionScope
}

type loopSessionScope struct {
	sessions map[string]runtime.AgentSession
}

func (e executionContext) withLoop(id string) executionContext {
	scopes := make(map[string]*loopSessionScope, len(e.sessionScopes)+1)
	for scopeID, scope := range e.sessionScopes {
		scopes[scopeID] = scope
	}

	scopes[id] = &loopSessionScope{sessions: make(map[string]runtime.AgentSession)}

	return executionContext{sessionScopes: scopes}
}

func (r *runState) node(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
	execution executionContext,
) (result nodeResult, resultErr error) {
	r.visits[node.EffectiveID]++

	logger := r.lifecycleLogger(ctx, node)
	started := time.Now()

	logger.Info().Msg("running agent")

	defer func() {
		writeLifecycleFinish(logger, "agent finished", result, resultErr, started)
	}()

	if err := r.applyState(node, input); err != nil {
		return nodeResult{}, err
	}

	switch node.Kind {
	case agent.RoleKind:
		return r.role(ctx, node, input, execution)
	case agent.SequentialKind:
		return r.sequential(ctx, node, input, execution)
	case agent.LoopKind:
		return r.loop(ctx, node, input, execution)
	default:
		return nodeResult{}, fmt.Errorf("agent %q has unsupported kind %q", node.ResourceID, node.Kind)
	}
}

func (r *runState) lifecycleLogger(ctx context.Context, node *registry.ResolvedNode) zerolog.Logger {
	logger := log.Ctx(ctx).With().
		Str("id", node.EffectiveID).
		Str("kind", string(node.Kind)).
		Int("visit", r.visits[node.EffectiveID]).
		Logger()
	if node.EffectiveID != node.ResourceID {
		logger = logger.With().Str("ref", node.ResourceID).Logger()
	}

	return logger
}

func writeLifecycleFinish(logger zerolog.Logger, message string, result nodeResult, resultErr error, started time.Time) {
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

	event.Dur("duration", time.Since(started)).Msg(message)
}

func (r *runState) role(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
	execution executionContext,
) (result nodeResult, resultErr error) {
	params, err := r.roleParams(ctx, node, input)
	if err != nil {
		return nodeResult{}, err
	}

	body, err := render(node.ResourceID+" spec.body", node.Resource.Spec.Body, agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.state,
		Params: params,
	})
	if err != nil {
		return nodeResult{}, err
	}

	provider, err := runtime.ProviderForAgent(node.Resource)
	if err != nil {
		return nodeResult{}, err
	}

	process, err := r.process(ctx, node.Resource, provider)
	if err != nil {
		return nodeResult{}, err
	}

	session, err := r.session(ctx, node, execution, process)
	if err != nil {
		return nodeResult{}, fmt.Errorf("agent %q: %w", node.EffectiveID, err)
	}

	if node.Resource.REPL() {
		logger := r.lifecycleLogger(ctx, node)
		started := time.Now()

		logger.Info().Msg("entering repl")

		defer func() {
			writeLifecycleFinish(logger, "exiting repl", result, resultErr, started)
		}()
	}

	turnInput := body + controlInstructions(node.Resource.REPL(), node.CanEscalate)
	for {
		turnCtx, cancelTurn := withActiveTimeout(ctx, node.Resource.ProviderTimeout(), r.pauses)
		content, err := session.Turn(turnCtx, turnInput)

		cancelTurn()

		if err != nil {
			return nodeResult{}, fmt.Errorf("agent %q turn: %w", node.EffectiveID, err)
		}

		parsed, err := parseResponse(content, node.Resource.REPL())
		if err != nil {
			return nodeResult{}, fmt.Errorf("agent %q: %w", node.EffectiveID, err)
		}

		if parsed.outcome == outcomeEscalate && !node.CanEscalate {
			return nodeResult{}, fmt.Errorf(
				"agent %q (resource %q, path %q) attempted unauthorized escalation",
				node.EffectiveID,
				node.ResourceID,
				strings.Join(node.Path, " -> "),
			)
		}

		switch parsed.outcome {
		case outcomeAwait:
			if r.interactor == nil {
				return nodeResult{}, fmt.Errorf("agent %q requested operator input without an interactor", node.EffectiveID)
			}

			if err := r.interactor.Display(parsed.artifact); err != nil {
				return nodeResult{}, fmt.Errorf("display agent %q response: %w", node.EffectiveID, err)
			}

			answer, err := r.interactor.Prompt(ctx, node.EffectiveID+" response")
			if err != nil {
				return nodeResult{}, fmt.Errorf("read agent %q response: %w", node.EffectiveID, err)
			}

			turnInput = answer + replReminder()
		case outcomeReturn, outcomeEscalate:
			if strings.TrimSpace(parsed.artifact) != "" {
				r.promote(node.EffectiveID, parsed.artifact)
			}

			return nodeResult{
				outcome:          parsed.outcome,
				artifact:         parsed.artifact,
				sourceID:         node.EffectiveID,
				sourceResourceID: node.ResourceID,
				sourcePath:       strings.Join(node.Path, " -> "),
			}, nil
		case outcomeFail:
			return nodeResult{
				outcome:          outcomeFail,
				artifact:         parsed.artifact,
				sourceID:         node.EffectiveID,
				sourceResourceID: node.ResourceID,
				sourcePath:       strings.Join(node.Path, " -> "),
			}, nil
		}
	}
}

func (r *runState) session(
	ctx context.Context,
	node *registry.ResolvedNode,
	execution executionContext,
	process runtime.ProviderProcess,
) (runtime.AgentSession, error) {
	if node.Session == agent.SessionModeStateful {
		scope, ok := execution.sessionScopes[node.SessionScopeID]
		if !ok {
			return nil, fmt.Errorf("stateful session scope %q is not active", node.SessionScopeID)
		}

		if session, ok := scope.sessions[node.EffectiveID]; ok {
			r.logSession(ctx, node, "reused")

			return session, nil
		}

		session, err := r.newSession(ctx, node, process)
		if err != nil {
			return nil, err
		}

		scope.sessions[node.EffectiveID] = session
		r.logSession(ctx, node, "created")

		return session, nil
	}

	session, err := r.newSession(ctx, node, process)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (r *runState) newSession(
	ctx context.Context,
	node *registry.ResolvedNode,
	process runtime.ProviderProcess,
) (runtime.AgentSession, error) {
	sessionCtx, cancelSession := context.WithTimeout(ctx, node.Resource.ProviderTimeout())
	defer cancelSession()

	session, err := process.NewSession(sessionCtx, node.Resource, node.EffectiveID)
	if err != nil {
		return nil, err
	}

	if err := session.Prepare(sessionCtx); err != nil {
		return nil, err
	}

	return session, nil
}

func (r *runState) logSession(ctx context.Context, node *registry.ResolvedNode, action string) {
	logger := r.lifecycleLogger(ctx, node)

	event := logger.Debug().Str("session", string(node.Session))
	if node.SessionScopeID != "" {
		event = event.Str("loop", node.SessionScopeID)
	}

	event.Msg("agent session " + action)
}

func (r *runState) sequential(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
	execution executionContext,
) (nodeResult, error) {
	localInput, err := r.compositeInput(node, input)
	if err != nil {
		return nodeResult{}, err
	}

	previous := localInput
	result := nodeResult{
		outcome:          outcomeReturn,
		sourceID:         node.EffectiveID,
		sourceResourceID: node.ResourceID,
		sourcePath:       strings.Join(node.Path, " -> "),
	}
	stickyEscalation := false

	for index, child := range node.Children {
		childInput, err := r.childInput(node, child, index, localInput, previous)
		if err != nil {
			return nodeResult{}, err
		}

		childResult, err := r.node(ctx, child, childInput, execution)
		if err != nil {
			return nodeResult{}, err
		}

		if childResult.outcome == outcomeFail {
			return childResult, nil
		}

		if childResult.outcome == outcomeEscalate {
			stickyEscalation = true
			result.sourceID = childResult.sourceID
			result.sourceResourceID = childResult.sourceResourceID
			result.sourcePath = childResult.sourcePath
		}

		previous = childResult.artifact
		result.artifact = childResult.artifact
	}

	if stickyEscalation {
		result.outcome = outcomeEscalate
		if strings.TrimSpace(result.artifact) != "" {
			r.promote(node.EffectiveID, result.artifact)
		}

		return result, nil
	}

	return r.finishComposite(node, localInput, result.artifact)
}

func (r *runState) loop(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
	execution executionContext,
) (nodeResult, error) {
	localInput, err := r.compositeInput(node, input)
	if err != nil {
		return nodeResult{}, err
	}

	var (
		previousIteration = localInput
		naturalOutput     string
		loopExecution     = execution.withLoop(node.EffectiveID)
	)

	for iteration := 0; iteration < *node.Resource.Spec.MaxIterations; iteration++ {
		previous := previousIteration

		for index, child := range node.Children {
			childInput, err := r.childInput(node, child, index, localInput, previous)
			if err != nil {
				return nodeResult{}, err
			}

			childResult, err := r.node(ctx, child, childInput, loopExecution)
			if err != nil {
				return nodeResult{}, err
			}

			if childResult.outcome == outcomeFail {
				return childResult, nil
			}

			previous = childResult.artifact
			naturalOutput = childResult.artifact

			if childResult.outcome == outcomeEscalate {
				return r.finishComposite(node, localInput, naturalOutput)
			}
		}

		previousIteration = naturalOutput
	}

	if node.Resource.ExhaustionPolicy() == "fail" {
		return nodeResult{
			outcome:          outcomeFail,
			artifact:         "maximum iterations exhausted",
			sourceID:         node.EffectiveID,
			sourceResourceID: node.ResourceID,
			sourcePath:       strings.Join(node.Path, " -> "),
		}, nil
	}

	return r.finishComposite(node, localInput, naturalOutput)
}

func (r *runState) compositeInput(node *registry.ResolvedNode, input string) (string, error) {
	localInput, err := render(node.ResourceID+" spec.body", node.Resource.Spec.Body, agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.state,
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
		State:  r.state,
	})
}

func (r *runState) finishComposite(node *registry.ResolvedNode, localInput, naturalOutput string) (nodeResult, error) {
	output := naturalOutput
	if node.Resource.Spec.Output != "" {
		rendered, err := renderOutput(node.ResourceID+" spec.output", node.Resource.Spec.Output, agent.TemplateData{
			Prompt: r.prompt,
			Input:  localInput,
			Output: naturalOutput,
			State:  r.state,
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

	snapshot, err := cloneState(r.state)
	if err != nil {
		return err
	}

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

	for key, value := range rendered {
		r.state[key] = value
	}

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
				State:  r.state,
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
	if process := r.processes[provider.Key()]; process != nil {
		return process, nil
	}

	process, cancel, err := startProviderProcess(ctx, role.ProviderTimeout(), r.factory, provider)
	if err != nil {
		return nil, fmt.Errorf("start agent %q provider: %w", role.ID, err)
	}

	r.processes[provider.Key()] = process
	r.started = append(r.started, startedProcess{key: provider.Key(), process: process, cancel: cancel})

	return process, nil
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
	var errs []error

	for index := len(r.started) - 1; index >= 0; index-- {
		started := r.started[index]

		closeErr := closeProcess(ctx, started.process)
		started.cancel()

		if closeErr != nil {
			errs = append(errs, fmt.Errorf("close provider group %q: %w", started.key, closeErr))
		}

		if ctx.Err() != nil {
			if !errors.Is(closeErr, ctx.Err()) {
				errs = append(errs, ctx.Err())
			}

			break
		}
	}

	return errors.Join(errs...)
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
	outputs := r.state["outputs"].(map[string]string)
	outputs[effectiveID] = artifact
}

func validateRuntimeParams(root *registry.ResolvedNode, values map[string]string) error {
	known := make(map[string]bool)
	bound := make(map[string]bool)

	var visit func(*registry.ResolvedNode)

	visit = func(node *registry.ResolvedNode) {
		if node.Kind == agent.RoleKind {
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

func cloneState(state map[string]any) (map[string]any, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("snapshot workflow state: %w", err)
	}

	var cloned map[string]any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("restore workflow state snapshot: %w", err)
	}

	return cloned, nil
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
