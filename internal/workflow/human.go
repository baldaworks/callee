package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

func (r *runState) human(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
) (nodeResult, error) {
	if r.interactor == nil {
		return nodeResult{}, fmt.Errorf("agent %q requires an interactor", node.EffectiveID)
	}

	body, err := renderRestricted(node.ResourceID+" spec.body", node.Resource.Spec.Body, agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.state,
	})
	if err != nil {
		return nodeResult{}, err
	}

	if err := r.interactor.Display(body); err != nil {
		return nodeResult{}, fmt.Errorf("display agent %q prompt: %w", node.EffectiveID, err)
	}

	answer, err := r.interactor.Prompt(ctx, node.EffectiveID+" response")
	if err != nil {
		return nodeResult{}, fmt.Errorf("read agent %q response: %w", node.EffectiveID, err)
	}

	if strings.TrimSpace(answer) == "" {
		return nodeResult{}, fmt.Errorf("agent %q response must not be blank", node.EffectiveID)
	}

	r.state[node.Resource.Spec.ResponseKey] = answer
	r.promote(node.EffectiveID, answer)

	return nodeResult{
		outcome:          outcomeReturn,
		artifact:         answer,
		sourceID:         node.EffectiveID,
		sourceResourceID: node.ResourceID,
		sourcePath:       strings.Join(node.Path, " -> "),
	}, nil
}
