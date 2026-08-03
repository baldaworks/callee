package workflow

import (
	"context"
	"fmt"
	"iter"
	"sort"
	"strconv"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/workflowagent"
	adkrunner "google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/workflow"
	"google.golang.org/genai"
)

const (
	adkAppName      = "callee"
	adkWorkflowName = "callee_workflow"
	adkUserID       = "callee"
)

// nodeExecution carries Callee execution errors as workflow data. ADK wraps
// callback errors without preserving their original error chain, so business
// errors must cross node boundaries in the typed output instead.
type nodeExecution struct {
	result nodeResult
	err    error
}

// rootRunOutput uniquely marks the terminal event of the ephemeral ADK graph.
// Consumers intentionally select it by type instead of relying on NodeInfo
// paths, which are ADK implementation details.
type rootRunOutput struct {
	execution nodeExecution
}

type adkCompiler struct {
	run  *runState
	next int
}

func (r *runState) runADK(ctx context.Context, root *registry.ResolvedNode, prompt string) (nodeResult, error) {
	compiler := &adkCompiler{run: r}

	rootNode, err := compiler.compile(root)
	if err != nil {
		return nodeResult{}, err
	}

	terminal := workflow.NewFunctionNode(
		compiler.nextName(),
		func(_ adkagent.Context, execution nodeExecution) (rootRunOutput, error) {
			return rootRunOutput{execution: execution}, nil
		},
		workflow.NodeConfig{},
	)

	wfAgent, err := workflowagent.New(workflowagent.Config{
		Name:  adkWorkflowName,
		Edges: workflow.Chain(workflow.Start, rootNode, terminal),
	})
	if err != nil {
		return nodeResult{}, fmt.Errorf("compile ADK workflow: %w", err)
	}

	sessions := session.InMemoryService()

	created, err := sessions.Create(ctx, &session.CreateRequest{
		AppName: adkAppName,
		UserID:  adkUserID,
	})
	if err != nil {
		return nodeResult{}, fmt.Errorf("create ADK workflow session: %w", err)
	}

	runner, err := adkrunner.New(adkrunner.Config{
		AppName:        adkAppName,
		Agent:          wfAgent,
		SessionService: sessions,
	})
	if err != nil {
		return nodeResult{}, fmt.Errorf("create ADK workflow runner: %w", err)
	}

	output, err := collectRootRunOutput(ctx, runner.Run(
		ctx,
		adkUserID,
		created.Session.ID(),
		genai.NewContentFromText(prompt, genai.RoleUser),
		adkagent.RunConfig{},
	))
	if err != nil {
		return nodeResult{}, err
	}

	if output.execution.err != nil {
		return nodeResult{}, output.execution.err
	}

	return output.execution.result, nil
}

func collectRootRunOutput(ctx context.Context, events func(func(*session.Event, error) bool)) (rootRunOutput, error) {
	var (
		output rootRunOutput
		count  int
	)

	for event, err := range events {
		if err != nil {
			return rootRunOutput{}, fmt.Errorf("run ADK workflow: %w", err)
		}

		if event == nil {
			continue
		}

		terminal, ok := event.Output.(rootRunOutput)
		if !ok {
			continue
		}

		output = terminal
		count++
	}

	if count == 0 && ctx.Err() != nil {
		return rootRunOutput{}, ctx.Err()
	}

	if count != 1 {
		return rootRunOutput{}, fmt.Errorf("ADK workflow returned %d terminal outputs, want exactly one", count)
	}

	return output, nil
}

func (c *adkCompiler) compile(node *registry.ResolvedNode) (workflow.Node, error) {
	name := c.nextName()

	switch node.Kind {
	case agent.RoleKind:
		return c.leaf(name, node, c.run.role), nil
	case agent.ScriptKind:
		return c.leaf(name, node, c.run.script), nil
	case agent.HumanKind:
		return c.leaf(name, node, c.run.human), nil
	case agent.RouterKind:
		return c.compileRouter(name, node)
	case agent.SequentialKind, agent.LoopKind:
		children := make([]workflow.Node, 0, len(node.Children))
		for _, child := range node.Children {
			compiled, err := c.compile(child)
			if err != nil {
				return nil, err
			}

			children = append(children, compiled)
		}

		return workflow.NewDynamicNode(
			name,
			func(ctx adkagent.Context, input string, _ func(*session.Event) error) (nodeExecution, error) {
				result, err := c.run.visit(ctx, node, input, func() (nodeResult, error) {
					if node.Kind == agent.SequentialKind {
						return c.run.sequentialADK(ctx, node, children, input)
					}

					return c.run.loopADK(ctx, node, children, input)
				})

				return nodeExecution{result: result, err: err}, nil
			},
			workflow.NodeConfig{},
		), nil
	default:
		return nil, fmt.Errorf("agent %q has unsupported kind %q", node.ResourceID, node.Kind)
	}
}

type routerDispatchInput struct {
	route   string
	payload string
}

type routerBranchOutput struct {
	execution nodeExecution
}

type routerNode struct {
	workflow.BaseNode

	run   *runState
	node  *registry.ResolvedNode
	inner *workflow.Workflow
}

func (c *adkCompiler) compileRouter(name string, node *registry.ResolvedNode) (workflow.Node, error) {
	children := make([]workflow.Node, 0, len(node.Children))
	for _, child := range node.Children {
		compiled, err := c.compile(child)
		if err != nil {
			return nil, err
		}

		children = append(children, compiled)
	}

	dispatch := workflow.NewEmittingFunctionNode(
		c.nextName(),
		func(ctx adkagent.Context, input routerDispatchInput, emit func(*session.Event) error) (any, error) {
			event := session.NewEvent(ctx, ctx.InvocationID())
			event.Routes = []string{input.route}

			event.Output = input.payload
			if err := emit(event); err != nil {
				return nil, err
			}

			return nil, nil
		},
		workflow.NodeConfig{},
	)

	edges := []workflow.Edge{{From: workflow.Start, To: dispatch}}

	for index, child := range node.Children {
		compiledChild := children[index]
		branch := workflow.NewDynamicNode(
			c.nextName(),
			func(ctx adkagent.Context, payload string, _ func(*session.Event) error) (routerBranchOutput, error) {
				childInput, err := c.run.childInput(node, child, index, payload, payload)
				if err != nil {
					return routerBranchOutput{execution: nodeExecution{err: err}}, nil
				}

				execution, err := workflow.RunNode[nodeExecution](ctx, compiledChild, childInput)
				if err != nil {
					return routerBranchOutput{}, fmt.Errorf("run compiled child %q: %w", child.EffectiveID, err)
				}

				return routerBranchOutput{execution: execution}, nil
			},
			workflow.NodeConfig{},
		)

		route := workflow.Route(workflow.StringRoute(child.Edge.Route))
		if child.Edge.Default {
			route = workflow.Default
		}

		edges = append(edges, workflow.Edge{From: dispatch, To: branch, Route: route})
	}

	inner, err := workflow.New("", edges, workflow.WithMaxConcurrency(1))
	if err != nil {
		return nil, fmt.Errorf("compile router %q ADK graph: %w", node.ResourceID, err)
	}

	return &routerNode{
		BaseNode: workflow.NewBaseNode(name, "", workflow.NodeConfig{}),
		run:      c.run,
		node:     node,
		inner:    inner,
	}, nil
}

func (n *routerNode) Run(ctx adkagent.Context, input any) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		text, ok := input.(string)
		if !ok {
			execution := nodeExecution{err: fmt.Errorf("router %q received input type %T, want string", n.node.ResourceID, input)}
			event := session.NewEvent(ctx, ctx.InvocationID())

			event.Output = execution
			if !yield(event, nil) {
				return
			}

			return
		}

		result, err := n.run.visit(ctx, n.node, text, func() (nodeResult, error) {
			return n.execute(ctx, text)
		})

		event := session.NewEvent(ctx, ctx.InvocationID())

		event.Output = nodeExecution{result: result, err: err}
		if !yield(event, nil) {
			return
		}
	}
}

func (n *routerNode) execute(ctx adkagent.Context, input string) (nodeResult, error) {
	route, err := render(n.node.ResourceID+" spec.route", n.node.Resource.Spec.Route, agent.TemplateData{
		Prompt: n.run.prompt,
		Input:  input,
		State:  n.run.state,
	})
	if err != nil {
		return nodeResult{}, err
	}

	route = strings.TrimSpace(route)

	selected, usedDefault := selectedRouterChild(n.node, route)
	if selected == nil {
		return nodeResult{}, unmatchedRouteError(n.node, route)
	}

	n.logSelection(ctx, selected, route, usedDefault)

	payload, err := n.run.compositeInput(n.node, input)
	if err != nil {
		return nodeResult{}, err
	}

	var (
		branch routerBranchOutput
		count  int
	)

	for event, err := range n.inner.RunNode(ctx, routerDispatchInput{route: route, payload: payload}) {
		if err != nil {
			return nodeResult{}, fmt.Errorf("run router %q ADK graph: %w", n.node.ResourceID, err)
		}

		if event == nil {
			continue
		}

		output, ok := event.Output.(routerBranchOutput)
		if !ok {
			continue
		}

		branch = output
		count++
	}

	if count != 1 {
		return nodeResult{}, fmt.Errorf("router %q ADK graph returned %d selected branch outputs, want exactly one", n.node.ResourceID, count)
	}

	if branch.execution.err != nil {
		return nodeResult{}, branch.execution.err
	}

	result := branch.execution.result
	if result.outcome == outcomeFail {
		return result, nil
	}

	if result.outcome == outcomeEscalate {
		if strings.TrimSpace(result.artifact) != "" {
			n.run.promote(n.node.EffectiveID, result.artifact)
		}

		return result, nil
	}

	return n.run.finishComposite(n.node, payload, result.artifact)
}

func selectedRouterChild(node *registry.ResolvedNode, route string) (*registry.ResolvedNode, bool) {
	var fallback *registry.ResolvedNode

	for _, child := range node.Children {
		if child.Edge.Route == route && !child.Edge.Default {
			return child, false
		}

		if child.Edge.Default {
			fallback = child
		}
	}

	return fallback, fallback != nil
}

func unmatchedRouteError(node *registry.ResolvedNode, route string) error {
	if route == "" {
		return fmt.Errorf("router %q rendered a blank route and has no default child", node.EffectiveID)
	}

	routes := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		if !child.Edge.Default {
			routes = append(routes, child.Edge.Route)
		}
	}

	sort.Strings(routes)

	const maxAvailableRoutes = 8

	omitted := 0
	if len(routes) > maxAvailableRoutes {
		omitted = len(routes) - maxAvailableRoutes
		routes = routes[:maxAvailableRoutes]
	}

	for index, available := range routes {
		routes[index] = strconv.Quote(boundedRouteKey(available))
	}

	if omitted > 0 {
		routes = append(routes, fmt.Sprintf("… (%d more)", omitted))
	}

	return fmt.Errorf(
		"router %q route %q did not match any child (available routes: %s)",
		node.EffectiveID,
		boundedRouteKey(route),
		strings.Join(routes, ", "),
	)
}

func (n *routerNode) logSelection(ctx context.Context, child *registry.ResolvedNode, route string, usedDefault bool) {
	logger := n.run.lifecycleLogger(ctx, n.node)

	event := logger.Info().Str("selected_child", child.EffectiveID)
	if !usedDefault {
		event = event.Str("route", boundedRouteKey(route))
		if routeKeyTruncated(route) {
			event = event.Bool("route_truncated", true)
		}

		event.Msg("selected router child")

		return
	}

	event = event.Bool("default", true)
	if route == "" {
		event.Str("route_match", "blank").Msg("selected router child")

		return
	}

	event.
		Str("route_match", "unknown").
		Str("route_key", boundedRouteKey(route))

	if routeKeyTruncated(route) {
		event = event.Bool("route_key_truncated", true)
	}

	event.Msg("selected router child")
}

func boundedRouteKey(route string) string {
	runes := []rune(route)
	if len(runes) <= maxRouteDiagnosticRunes {
		return route
	}

	return string(runes[:maxRouteDiagnosticRunes]) + "…"
}

func routeKeyTruncated(route string) bool {
	return len([]rune(route)) > maxRouteDiagnosticRunes
}

const maxRouteDiagnosticRunes = 128

func (c *adkCompiler) leaf(
	name string,
	node *registry.ResolvedNode,
	execute func(context.Context, *registry.ResolvedNode, string) (nodeResult, error),
) workflow.Node {
	return workflow.NewFunctionNode(
		name,
		func(ctx adkagent.Context, input string) (nodeExecution, error) {
			result, err := c.run.visit(ctx, node, input, func() (nodeResult, error) {
				return execute(ctx, node, input)
			})

			return nodeExecution{result: result, err: err}, nil
		},
		workflow.NodeConfig{},
	)
}

func (c *adkCompiler) nextName() string {
	c.next++

	return fmt.Sprintf("callee_node_%06d", c.next)
}

func (r *runState) sequentialADK(
	ctx adkagent.Context,
	node *registry.ResolvedNode,
	children []workflow.Node,
	input string,
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

		childExecution, err := workflow.RunNode[nodeExecution](ctx, children[index], childInput)
		if err != nil {
			return nodeResult{}, fmt.Errorf("run compiled child %q: %w", child.EffectiveID, err)
		}

		if childExecution.err != nil {
			return nodeResult{}, childExecution.err
		}

		childResult := childExecution.result
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

func (r *runState) loopADK(
	ctx adkagent.Context,
	node *registry.ResolvedNode,
	children []workflow.Node,
	input string,
) (nodeResult, error) {
	localInput, err := r.compositeInput(node, input)
	if err != nil {
		return nodeResult{}, err
	}

	var (
		previousIteration = localInput
		naturalOutput     string
	)

	for iteration := 0; iteration < *node.Resource.Spec.MaxIterations; iteration++ {
		previous := previousIteration

		for index, child := range node.Children {
			childInput, err := r.childInput(node, child, index, localInput, previous)
			if err != nil {
				return nodeResult{}, err
			}

			childExecution, err := workflow.RunNode[nodeExecution](ctx, children[index], childInput)
			if err != nil {
				return nodeResult{}, fmt.Errorf("run compiled child %q: %w", child.EffectiveID, err)
			}

			if childExecution.err != nil {
				return nodeResult{}, childExecution.err
			}

			childResult := childExecution.result
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
