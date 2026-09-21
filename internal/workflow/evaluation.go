package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
	evaluationapi "github.com/baldaworks/callee/internal/evaluation"
	"github.com/baldaworks/callee/internal/registry"
)

func (r *runState) evaluation(ctx context.Context, node *registry.ResolvedNode, input string) (result nodeResult, resultErr error) {
	result.evaluationTrace = evaluationapi.Trace{
		Service: serviceForEvaluationKind(node.Kind), RequestedModel: node.Resource.EvaluationModel(),
	}
	if r.evaluationEvaluator == nil {
		return result, fmt.Errorf("agent %q has no evaluation service", node.EffectiveID)
	}

	snapshot := r.snapshotState()

	templateData := agent.TemplateData{Prompt: r.prompt, Input: input, State: snapshot}

	var (
		evidence any
		err      error
	)
	if strings.TrimSpace(node.Resource.Spec.Body) != "" {
		evidence, err = renderRestricted(node.ResourceID+" spec.body", node.Resource.Spec.Body, templateData)
	} else {
		evidence, err = renderStateValue(node.ResourceID+" spec.evidence", node.Resource.Spec.Evidence, templateData)
	}

	if err != nil {
		return result, err
	}

	questions := make(map[string]agent.EvaluationQuestion, len(node.Resource.Spec.Questions))

	ids := make([]string, 0, len(node.Resource.Spec.Questions))
	for id := range node.Resource.Spec.Questions {
		ids = append(ids, id)
	}

	sort.Strings(ids)

	for _, id := range ids {
		question := node.Resource.Spec.Questions[id]

		question.Instructions, err = renderStateValue(node.ResourceID+" spec.questions."+id+".instructions", question.Instructions, templateData)
		if err != nil {
			return result, err
		}

		if question.Criteria != nil {
			question.Criteria, err = renderStateValue(node.ResourceID+" spec.questions."+id+".criteria", question.Criteria, templateData)
			if err != nil {
				return result, err
			}
		}

		questions[id] = question
	}

	config := evaluationapi.Config{
		Kind: node.Kind, Model: node.Resource.EvaluationModel(), Timeout: node.Resource.EvaluationTimeout(),
	}
	request := evaluationapi.Request{State: evidence, Questions: questions}

	evaluated, trace, err := r.evaluationEvaluator.Evaluate(ctx, config, request)
	if trace.Service == "" {
		trace.Service = serviceForEvaluationKind(node.Kind)
	}

	result.evaluationTrace = trace
	if err != nil {
		return result, fmt.Errorf("agent %q evaluate %s: %w", node.EffectiveID, node.Kind, err)
	}

	artifact, err := evaluationapi.MarshalResult(evaluated)
	if err != nil {
		return result, fmt.Errorf("agent %q: %w", node.EffectiveID, err)
	}

	var structured map[string]any
	if err := json.Unmarshal(artifact, &structured); err != nil {
		return result, fmt.Errorf("agent %q decode evaluation state result: %w", node.EffectiveID, err)
	}

	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	evaluations, ok := r.state["evaluations"].(map[string]any)
	if !ok {
		return result, fmt.Errorf("workflow evaluations state is unavailable")
	}

	outputs, ok := r.state["outputs"].(map[string]string)
	if !ok {
		return result, fmt.Errorf("workflow outputs state is unavailable")
	}

	evaluations[node.EffectiveID] = structured
	outputs[node.EffectiveID] = string(artifact)
	result.outcome = outcomeReturn
	result.artifact = string(artifact)
	result.sourceID = node.EffectiveID
	result.sourceResourceID = node.ResourceID
	result.sourcePath = strings.Join(node.Path, " -> ")

	return result, nil
}

func serviceForEvaluationKind(kind agent.Kind) string {
	if kind == agent.TypeSafeJevKind {
		return "typesafe"
	}

	if kind == agent.OpenRouterDecisionKind {
		return "openrouter"
	}

	return ""
}
