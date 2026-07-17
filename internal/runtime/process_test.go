package runtime

import (
	"context"
	"iter"
	"testing"

	resource "github.com/baldaworks/callee/internal/agent"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
)

func TestNormaAgentSessionPrepareStopsBeforePrompt(t *testing.T) {
	t.Parallel()

	prompted := false

	agentInstance, err := adkagent.New(adkagent.Config{
		Name:        "doctor_check",
		Description: "test agent",
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				binding := session.NewEvent(ctx, ctx.InvocationID())

				binding.Actions.StateDelta[acpagent.SessionStateKey] = map[string]any{"session_id": "test"}
				if !yield(binding, nil) {
					return
				}

				prompted = true

				yield(session.NewEvent(ctx, ctx.InvocationID()), nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New() error: %v", err)
	}

	process, err := newNormaProcess(agentInstance, nil, nil)
	if err != nil {
		t.Fatalf("newNormaProcess() error: %v", err)
	}

	role := resource.Resource{
		APIVersion: resource.APIVersion,
		Kind:       resource.RoleKind,
		ID:         "roles/check",
		Spec: resource.Spec{
			Description: "check",
			Provider:    &resource.Provider{Type: "codex"},
			Body:        "{{ .Input }}",
		},
	}

	prepared, err := process.NewSession(context.Background(), role)
	if err != nil {
		t.Fatalf("ProviderProcess.NewSession() error: %v", err)
	}

	if err := prepared.Prepare(context.Background()); err != nil {
		t.Fatalf("AgentSession.Prepare() error: %v", err)
	}

	if prompted {
		t.Fatal("AgentSession.Prepare() continued to provider prompt")
	}
}
