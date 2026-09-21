package workflow

import (
	"errors"
	"fmt"
	"iter"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/rs/zerolog"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/adk/v2/workflow"
)

type parallelPlan struct {
	inputs []string
	stats  *parallelStats
}

type parallelBranchOutput struct {
	execution nodeExecution
}

type parallelStats struct {
	branches  int64
	started   atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64
	joined    atomic.Bool
}

type parallelNode struct {
	workflow.BaseNode

	run         *runState
	node        *registry.ResolvedNode
	inner       *workflow.Workflow
	branchNames []string
}

func (c *adkCompiler) compileParallel(name string, node *registry.ResolvedNode) (workflow.Node, error) {
	children := make([]workflow.Node, len(node.Children))
	for index, child := range node.Children {
		compiled, err := c.compile(child)
		if err != nil {
			return nil, err
		}

		children[index] = compiled
	}

	branches := make([]workflow.Node, len(children))
	branchNames := make([]string, len(children))

	for index, childNode := range node.Children {
		compiled := children[index]

		branchName, err := c.helperName(adkNodeRoleParallelBranch, node, childNode)
		if err != nil {
			return nil, err
		}

		branchNames[index] = branchName
		branches[index] = workflow.NewDynamicNode(
			branchName,
			func(ctx adkagent.Context, plan parallelPlan, _ func(*session.Event) error) (parallelBranchOutput, error) {
				plan.stats.started.Add(1)

				execution, err := workflow.RunNode[nodeExecution](ctx, compiled, plan.inputs[index], workflow.WithUseSubBranch())
				if err != nil {
					if ctx.Err() != nil {
						return parallelBranchOutput{}, fmt.Errorf("run compiled child %q: %w", childNode.EffectiveID, err)
					}

					execution.err = fmt.Errorf("run compiled child %q: %w", childNode.EffectiveID, err)
				}

				plan.stats.completed.Add(1)

				if execution.err != nil || execution.result.outcome == outcomeFail {
					plan.stats.failed.Add(1)
				}

				return parallelBranchOutput{execution: execution}, nil
			},
			workflow.NodeConfig{},
		)
	}

	joinName, err := c.helperName(adkNodeRoleParallelJoin, node, nil)
	if err != nil {
		return nil, err
	}

	join := workflow.NewJoinNode(joinName)
	edges := workflow.NewEdgeBuilder().AddFanOut(workflow.Start, branches...).AddFanIn(join, branches...).Build()

	graphName, err := c.helperName(adkNodeRoleParallelGraph, node, nil)
	if err != nil {
		return nil, err
	}

	inner, err := workflow.New(graphName, edges)
	if err != nil {
		return nil, fmt.Errorf("compile Parallel %q ADK graph: %w", node.ResourceID, err)
	}

	return &parallelNode{
		BaseNode:    workflow.NewBaseNode(name, "", workflow.NodeConfig{}),
		run:         c.run,
		node:        node,
		inner:       inner,
		branchNames: branchNames,
	}, nil
}

func (n *parallelNode) Run(ctx adkagent.Context, input any) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		text, ok := input.(string)
		if !ok {
			execution := nodeExecution{err: fmt.Errorf("Parallel %q received input type %T, want string", n.node.ResourceID, input)}
			event := session.NewEvent(ctx, ctx.InvocationID())
			event.Output = execution
			yield(event, nil)

			return
		}

		stats := &parallelStats{branches: int64(len(n.node.Children))}
		result, err := n.run.visitWithOptions(ctx, n.node, text, visitOptions{
			appendFinish: func(event *zerolog.Event) *zerolog.Event {
				return event.
					Int64("parallel_branches", stats.branches).
					Int64("parallel_started", stats.started.Load()).
					Int64("parallel_completed", stats.completed.Load()).
					Int64("parallel_failed", stats.failed.Load()).
					Bool("parallel_joined", stats.joined.Load())
			},
		}, func() (nodeResult, error) {
			return n.execute(ctx, text, stats)
		})

		event := session.NewEvent(ctx, ctx.InvocationID())
		event.Output = nodeExecution{result: result, err: err}
		yield(event, nil)
	}
}

func (n *parallelNode) execute(ctx adkagent.Context, input string, stats *parallelStats) (nodeResult, error) {
	snapshot := n.run.snapshotState()

	localInput, err := render(n.node.ResourceID+" spec.body", n.node.Resource.Spec.Body, agent.TemplateData{
		Prompt: n.run.prompt,
		Input:  input,
		State:  snapshot,
	})
	if err != nil {
		return nodeResult{}, err
	}

	if strings.TrimSpace(localInput) == "" {
		return nodeResult{}, fmt.Errorf("agent %q spec.body rendered an empty input", n.node.EffectiveID)
	}

	plan := parallelPlan{inputs: make([]string, len(n.node.Children)), stats: stats}
	for index, child := range n.node.Children {
		plan.inputs[index] = localInput

		if child.Edge.Input == "" {
			continue
		}

		plan.inputs[index], err = render(n.node.ResourceID+" child "+child.EffectiveID+" input", child.Edge.Input, agent.TemplateData{
			Prompt: n.run.prompt,
			Input:  localInput,
			State:  snapshot,
		})
		if err != nil {
			return nodeResult{}, err
		}
	}

	var joined map[string]any

	joinCount := 0

	for event, runErr := range n.inner.RunNode(ctx, plan) {
		if runErr != nil {
			return nodeResult{}, fmt.Errorf("run Parallel %q ADK graph: %w", n.node.ResourceID, runErr)
		}

		if event == nil {
			continue
		}

		output, ok := event.Output.(map[string]any)
		if !ok {
			continue
		}

		joined = output
		joinCount++
	}

	if joinCount != 1 {
		return nodeResult{}, fmt.Errorf("Parallel %q ADK graph returned %d Join outputs, want exactly one", n.node.ResourceID, joinCount)
	}

	stats.joined.Store(true)

	results := make([]nodeExecution, len(n.node.Children))
	for index, name := range n.branchNames {
		value, ok := joined[name]
		if !ok {
			return nodeResult{}, fmt.Errorf("Parallel %q Join omitted child %q", n.node.ResourceID, n.node.Children[index].EffectiveID)
		}

		branch, ok := value.(parallelBranchOutput)
		if !ok {
			return nodeResult{}, fmt.Errorf("Parallel %q child %q Join output has type %T", n.node.ResourceID, n.node.Children[index].EffectiveID, value)
		}

		results[index] = branch.execution
	}

	return n.finish(localInput, results)
}

func (n *parallelNode) finish(localInput string, executions []nodeExecution) (nodeResult, error) {
	var (
		executionErrors []error
		failed          []nodeResult
	)

	hasExecutionError := false

	for _, execution := range executions {
		if execution.err != nil {
			hasExecutionError = true

			break
		}
	}

	for index, execution := range executions {
		child := n.node.Children[index]
		if execution.err != nil {
			executionErrors = append(executionErrors, fmt.Errorf("parallel child %q: %w", child.EffectiveID, execution.err))

			continue
		}

		if execution.result.outcome == outcomeFail {
			failed = append(failed, execution.result)
			if hasExecutionError {
				executionErrors = append(executionErrors, fmt.Errorf("parallel child %q failed: %s", child.EffectiveID, diagnosticDetail(execution.result.artifact)))
			}
		}
	}

	if len(executionErrors) > 0 {
		return nodeResult{}, errors.Join(executionErrors...)
	}

	if len(failed) == 1 {
		return failed[0], nil
	}

	if len(failed) > 1 {
		lines := make([]string, len(failed))
		for index, result := range failed {
			lines[index] = fmt.Sprintf("parallel child %q failed: %s", result.sourceID, strconv.Quote(diagnosticDetail(result.artifact)))
		}

		result := failed[0]
		result.artifact = strings.Join(lines, "\n")

		return result, nil
	}

	aggregate := parallelAggregate(n.node.Children, executions)

	result, err := n.run.finishComposite(n.node, localInput, aggregate)
	if err != nil {
		return nodeResult{}, err
	}

	for _, execution := range executions {
		if execution.result.outcome == outcomeEscalate {
			result.outcome = outcomeEscalate
			result.sourceID = execution.result.sourceID
			result.sourceResourceID = execution.result.sourceResourceID
			result.sourcePath = execution.result.sourcePath

			break
		}
	}

	return result, nil
}

func parallelAggregate(children []*registry.ResolvedNode, executions []nodeExecution) string {
	var result strings.Builder
	result.WriteByte('{')

	for index, child := range children {
		if index > 0 {
			result.WriteByte(',')
		}

		result.WriteString(strconv.Quote(child.EffectiveID))
		result.WriteByte(':')
		result.WriteString(strconv.Quote(executions[index].result.artifact))
	}

	result.WriteByte('}')

	return result.String()
}
