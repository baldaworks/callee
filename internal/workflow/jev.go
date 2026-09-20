package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
	jevapi "github.com/baldaworks/callee/internal/jev"
	"github.com/baldaworks/callee/internal/registry"
)

func (r *runState) jev(ctx context.Context, node *registry.ResolvedNode, input string) (result nodeResult, resultErr error) {
	if node.Resource.Spec.API == nil {
		return result, fmt.Errorf("agent %q has no Jev API configuration", node.EffectiveID)
	}

	result.jevTrace = jevapi.Trace{API: node.Resource.Spec.API.Type, RequestedModel: node.Resource.JevModel()}
	if r.jevEvaluator == nil {
		return result, fmt.Errorf("agent %q has no Jev evaluator", node.EffectiveID)
	}

	snapshot, err := cloneState(r.state)
	if err != nil {
		return result, err
	}

	templateData := agent.TemplateData{Prompt: r.prompt, Input: input, State: snapshot}

	var evidence any
	if strings.TrimSpace(node.Resource.Spec.Body) != "" {
		evidence, err = renderRestricted(node.ResourceID+" spec.body", node.Resource.Spec.Body, templateData)
	} else {
		evidence, err = renderStateValue(node.ResourceID+" spec.evidence", node.Resource.Spec.Evidence, templateData)
	}

	if err != nil {
		return result, err
	}

	questions := make(map[string]agent.JevQuestion, len(node.Resource.Spec.Questions))
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

	request := jevapi.Request{Model: node.Resource.JevModel(), State: evidence, Questions: questions}

	evaluated, trace, err := r.jevEvaluator.Evaluate(ctx, *node.Resource.Spec.API, request)
	if trace.API == "" {
		trace.API = node.Resource.Spec.API.Type
	}

	if trace.RequestedModel == "" {
		trace.RequestedModel = node.Resource.JevModel()
	}

	result.jevTrace = trace
	if err != nil {
		return result, fmt.Errorf("agent %q evaluate Jev: %w", node.EffectiveID, err)
	}

	artifact, err := jevapi.MarshalResult(evaluated)
	if err != nil {
		return result, fmt.Errorf("agent %q: %w", node.EffectiveID, err)
	}

	var structured map[string]any
	if err := json.Unmarshal(artifact, &structured); err != nil {
		return result, fmt.Errorf("agent %q decode Jev state result: %w", node.EffectiveID, err)
	}

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
