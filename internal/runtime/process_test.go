package runtime

import (
	"context"
	"iter"
	"strings"
	"testing"

	resource "github.com/baldaworks/callee/internal/agent"
	acp "github.com/coder/acp-go-sdk"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	adkagent "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
)

func TestNormaAgentSessionPrepareStopsBeforePrompt(t *testing.T) {
	t.Parallel()

	var (
		prompted       bool
		boundSessionID acp.SessionId
		boundRole      resource.Resource
	)

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

	process, err := newNormaProcess(agentInstance, nil, nil, func(sessionID acp.SessionId, role resource.Resource) {
		boundSessionID = sessionID
		boundRole = role
	})
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

	prepared, err := process.NewSession(context.Background(), role, "worker")
	if err != nil {
		t.Fatalf("ProviderProcess.NewSession() error: %v", err)
	}

	if err := prepared.Prepare(context.Background()); err != nil {
		t.Fatalf("AgentSession.Prepare() error: %v", err)
	}

	if prompted {
		t.Fatal("AgentSession.Prepare() continued to provider prompt")
	}

	if boundSessionID != "test" || boundRole.ID != "worker" {
		t.Errorf("permission binding = (%q, %q), want (test, worker)", boundSessionID, boundRole.ID)
	}

	if boundRole.Spec.Provider == nil || boundRole.Spec.Provider.Type != "codex" {
		t.Errorf("bound role = %+v, want original Role configuration", boundRole)
	}
}

func TestPermissionSessionIDRejectsInvalidState(t *testing.T) {
	t.Parallel()

	for _, state := range []any{nil, "session", map[string]any{}, map[string]any{"session_id": " "}} {
		if _, err := permissionSessionID(state); err == nil || !strings.Contains(err.Error(), "session") {
			t.Errorf("permissionSessionID(%#v) error = %v, want session diagnostic", state, err)
		}
	}
}
