package workflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/agent"
)

func TestScriptRecordsStateAndPromotesSummary(t *testing.T) {
	t.Parallel()

	workdir := t.TempDir()
	root := resolvedRoot(t, scriptResource(t, "scripts/validator", `printf '%s|%s' "$(pwd)" "$FOO"; printf 'warn' >&2`))
	root.Resource.Spec.Cwd = workdir
	root.Resource.Spec.Env = map[string]string{"FOO": "{{ .Input }}"}

	run := &runState{
		prompt: "prompt",
		state: map[string]any{
			"outputs": map[string]string{},
			"scripts": map[string]any{},
		},
	}

	result, err := run.script(context.Background(), root, "build")
	if err != nil {
		t.Fatalf("runState.script() error: %v", err)
	}

	if result.outcome != outcomeReturn {
		t.Fatalf("runState.script() outcome = %v, want return", result.outcome)
	}

	if got, want := result.artifact, "status=passed exitCode=0"; got != want {
		t.Fatalf("runState.script() artifact = %q, want %q", got, want)
	}

	outputs := run.state["outputs"].(map[string]string)
	if got := outputs["scripts/validator"]; got != result.artifact {
		t.Fatalf("outputs[scripts/validator] = %q, want %q", got, result.artifact)
	}

	entry := run.state["scripts"].(map[string]any)["scripts/validator"].(map[string]any)
	if got, want := entry["status"], "passed"; got != want {
		t.Fatalf("script status = %#v, want %q", got, want)
	}

	if got, want := entry["exitCode"], 0; got != want {
		t.Fatalf("script exitCode = %#v, want %d", got, want)
	}

	if got, want := entry["stdout"], filepath.Clean(workdir)+"|build"; got != want {
		t.Fatalf("script stdout = %#v, want %q", got, want)
	}

	if got, want := entry["stderr"], "warn"; got != want {
		t.Fatalf("script stderr = %#v, want %q", got, want)
	}

	if got, want := entry["timedOut"], false; got != want {
		t.Fatalf("script timedOut = %#v, want %t", got, want)
	}
}

func TestRunnerScriptContinueExposesStateToReviewer(t *testing.T) {
	t.Parallel()

	validator := scriptResource(t, "scripts/validator", `printf 'bad out'; printf 'lint failed' >&2; exit 3`)
	validator.Spec.OnNonZero = "continue"

	reviewer := roleResource(t, "roles/reviewer", false, nil, `Task: {{ .Input }}
Exit={{ index (index .State.scripts "validator") "exitCode" }}
Err={{ index (index .State.scripts "validator") "stderr" }}`)

	root := resolvedRoot(t,
		validator,
		reviewer,
		compositeResource(t, "workflows/review", agent.SequentialKind, []agent.Child{
			{Ref: "scripts/validator", Alias: "validator"},
			{Ref: "roles/reviewer", Alias: "reviewer"},
		}, 0, "{{ .Input }}", ""),
	)

	process := &scriptedProcess{visits: map[string][][]string{
		"roles/reviewer": {{"retry"}},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "check")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "retry" {
		t.Fatalf("Runner.Run() = %q, want retry", got)
	}

	prompt := process.prompts["roles/reviewer"][0]
	for _, want := range []string{"status=failed exitCode=3", "Exit=3", "Err=lint failed"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("reviewer prompt = %q, want containing %q", prompt, want)
		}
	}
}

func TestRunnerScriptContinueAllowsAnotherLoopIteration(t *testing.T) {
	t.Parallel()

	validator := scriptResource(t, "scripts/validator", `printf 'lint failed' >&2; exit 3`)
	validator.Spec.OnNonZero = "continue"

	root := resolvedRoot(t,
		validator,
		roleResource(t, "roles/reviewer", false, nil, `Input={{ .Input }}
Exit={{ index (index .State.scripts "validator") "exitCode" }}`),
		compositeResource(t, "workflows/review", agent.LoopKind, []agent.Child{
			{Ref: "scripts/validator", Alias: "validator"},
			{Ref: "roles/reviewer", Alias: "reviewer", CanEscalate: true},
		}, 3, "{{ .Input }}", "{{ .State.outputs.reviewer }}"),
	)
	process := &scriptedProcess{visits: map[string][][]string{
		"roles/reviewer": {
			{"retry"},
			{"accepted\n\n" + controlEscalate},
		},
	}}

	got, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "check")
	if err != nil {
		t.Fatalf("Runner.Run() error: %v", err)
	}

	if got != "accepted" {
		t.Fatalf("Runner.Run() = %q, want accepted", got)
	}

	if prompts := process.prompts["roles/reviewer"]; len(prompts) != 2 {
		t.Fatalf("reviewer prompts = %d, want 2", len(prompts))
	}
}

func TestRunnerScriptFailStopsWorkflow(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t,
		scriptResource(t, "scripts/validator", `printf 'lint failed' >&2; exit 3`),
		roleResource(t, "roles/reviewer", false, nil, "{{ .Input }}"),
		compositeResource(t, "workflows/review", agent.SequentialKind, []agent.Child{
			{Ref: "scripts/validator", Alias: "validator"},
			{Ref: "roles/reviewer", Alias: "reviewer"},
		}, 0, "{{ .Input }}", ""),
	)

	process := &scriptedProcess{visits: map[string][][]string{
		"roles/reviewer": {{"unexpected"}},
	}}

	_, err := (Runner{Root: root, Factory: &scriptedFactory{process: process}}).Run(context.Background(), "check")
	if err == nil || !strings.Contains(err.Error(), `agent "validator" failed: status=failed exitCode=3`) {
		t.Fatalf("Runner.Run() error = %v, want validator failure", err)
	}

	if len(process.prompts["roles/reviewer"]) != 0 {
		t.Fatalf("reviewer prompts = %d, want 0", len(process.prompts["roles/reviewer"]))
	}
}

func TestScriptTimeoutRecordsFailureState(t *testing.T) {
	t.Parallel()

	root := resolvedRoot(t, scriptResource(t, "scripts/validator", "sleep 1"))
	root.Resource.Spec.Timeout = "10ms"

	run := &runState{
		prompt: "prompt",
		state: map[string]any{
			"outputs": map[string]string{},
			"scripts": map[string]any{},
		},
	}

	result, err := run.script(context.Background(), root, "build")
	if err != nil {
		t.Fatalf("runState.script() error: %v", err)
	}

	if result.outcome != outcomeFail {
		t.Fatalf("runState.script() outcome = %v, want fail", result.outcome)
	}

	if !strings.Contains(result.artifact, "timedOut=true") {
		t.Fatalf("runState.script() artifact = %q, want timedOut=true", result.artifact)
	}

	entry := run.state["scripts"].(map[string]any)["scripts/validator"].(map[string]any)
	if got, want := entry["status"], "failed"; got != want {
		t.Fatalf("script status = %#v, want %q", got, want)
	}

	if got, want := entry["timedOut"], true; got != want {
		t.Fatalf("script timedOut = %#v, want %t", got, want)
	}

	if got, want := entry["exitCode"], -1; got != want {
		t.Fatalf("script exitCode = %#v, want %d", got, want)
	}
}

func scriptResource(t *testing.T, id, body string) agent.Resource {
	t.Helper()

	resource := agent.Resource{
		APIVersion: agent.APIVersion,
		Kind:       agent.ScriptKind,
		ID:         id,
		Source:     id + ".md",
		Spec: agent.Spec{
			Description: id,
			Body:        body,
		},
	}

	if err := resource.Validate(); err != nil {
		t.Fatalf("script resource %q validation error: %v", id, err)
	}

	return resource
}
