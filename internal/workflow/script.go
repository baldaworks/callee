package workflow

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

func (r *runState) script(
	ctx context.Context,
	node *registry.ResolvedNode,
	input string,
) (nodeResult, error) {
	body, err := renderRestricted(node.ResourceID+" spec.body", node.Resource.Spec.Body, agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.state,
	})
	if err != nil {
		return nodeResult{}, err
	}

	cwd, err := r.renderScriptCwd(node, input)
	if err != nil {
		return nodeResult{}, err
	}

	env, err := r.renderScriptEnv(node, input)
	if err != nil {
		return nodeResult{}, err
	}

	scriptCtx, cancel := context.WithTimeout(ctx, node.Resource.ScriptTimeout())
	defer cancel()

	command := exec.CommandContext(scriptCtx, node.Resource.ScriptShell(), "-c", body)
	command.Dir = cwd
	command.Env = env

	var stdout, stderr bytes.Buffer

	command.Stdout = &stdout
	command.Stderr = &stderr

	err = command.Run()

	exitCode := 0

	timedOut := errors.Is(scriptCtx.Err(), context.DeadlineExceeded)
	if timedOut {
		exitCode = -1
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if !timedOut {
			return nodeResult{}, fmt.Errorf("agent %q: run script: %w", node.EffectiveID, err)
		}
	}

	status := "passed"
	if timedOut || exitCode != 0 {
		status = "failed"
	}

	entry := map[string]any{
		"status":   status,
		"exitCode": exitCode,
		"stdout":   stdout.String(),
		"stderr":   stderr.String(),
		"timedOut": timedOut,
	}
	r.recordScript(node.EffectiveID, entry)

	summary := summarizeScript(status, exitCode, timedOut)
	result := nodeResult{
		artifact:         summary,
		sourceID:         node.EffectiveID,
		sourceResourceID: node.ResourceID,
		sourcePath:       strings.Join(node.Path, " -> "),
	}

	switch {
	case timedOut:
		result.outcome = outcomeFail

		return result, nil
	case exitCode != 0 && node.Resource.NonZeroPolicy() == "fail":
		result.outcome = outcomeFail

		return result, nil
	default:
		result.outcome = outcomeReturn

		r.promote(node.EffectiveID, summary)

		return result, nil
	}
}

func (r *runState) renderScriptCwd(node *registry.ResolvedNode, input string) (string, error) {
	if node.Resource.Spec.Cwd == "" {
		return "", nil
	}

	return renderRestricted(node.ResourceID+" spec.cwd", node.Resource.Spec.Cwd, agent.TemplateData{
		Prompt: r.prompt,
		Input:  input,
		State:  r.state,
	})
}

func (r *runState) renderScriptEnv(node *registry.ResolvedNode, input string) ([]string, error) {
	values := make(map[string]string)

	for _, pair := range os.Environ() {
		name, value, found := strings.Cut(pair, "=")
		if !found {
			continue
		}

		values[name] = value
	}

	names := make([]string, 0, len(node.Resource.Spec.Env))
	for name := range node.Resource.Spec.Env {
		names = append(names, name)
	}

	sort.Strings(names)

	for _, name := range names {
		rendered, err := renderRestricted(node.ResourceID+" spec.env."+name, node.Resource.Spec.Env[name], agent.TemplateData{
			Prompt: r.prompt,
			Input:  input,
			State:  r.state,
		})
		if err != nil {
			return nil, err
		}

		values[name] = rendered
	}

	encodedNames := make([]string, 0, len(values))
	for name := range values {
		encodedNames = append(encodedNames, name)
	}

	sort.Strings(encodedNames)

	encoded := make([]string, 0, len(encodedNames))
	for _, name := range encodedNames {
		encoded = append(encoded, name+"="+values[name])
	}

	return encoded, nil
}

func (r *runState) recordScript(effectiveID string, entry map[string]any) {
	scripts := r.state["scripts"].(map[string]any)
	scripts[effectiveID] = entry
}

func summarizeScript(status string, exitCode int, timedOut bool) string {
	summary := fmt.Sprintf("status=%s exitCode=%d", status, exitCode)
	if timedOut {
		summary += " timedOut=true"
	}

	return summary
}
