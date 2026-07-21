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
	"google.golang.org/genai"
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

				binding.Actions.StateDelta[acpagent.SessionStateKey] = map[string]any{
					"session_id": "test",
					"config_values": []map[string]any{
						{"id": "model", "value": "gpt-5.6-sol"},
						{"id": "reasoning_effort", "value": "high"},
					},
				}
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

	reporter, ok := prepared.(interface{ Configuration() SessionConfiguration })
	if !ok {
		t.Fatal("prepared Norma session does not report its configuration")
	}

	wantConfiguration := SessionConfiguration{Model: "gpt-5.6-sol", Reasoning: "high"}
	if got := reporter.Configuration(); got != wantConfiguration {
		t.Errorf("session configuration = %+v, want %+v", got, wantConfiguration)
	}
}

func TestNormaAgentSessionTurnUsesFinalResponseUsage(t *testing.T) {
	t.Parallel()

	agentInstance, err := adkagent.New(adkagent.Config{
		Name:        "usage_check",
		Description: "test agent",
		Run: func(ctx adkagent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				contextUsage := session.NewEvent(ctx, ctx.InvocationID())

				contextUsage.Partial = true

				contextUsage.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount: 100_000,
					TotalTokenCount:  50_000,
				}

				contextUsage.Actions.StateDelta[acpagent.SessionStateKey] = map[string]any{
					"config_values": []map[string]any{
						{"id": "model", "value": "gpt-5.6-sol"},
						{"id": "thought_level", "value": "medium"},
					},
				}
				if !yield(contextUsage, nil) {
					return
				}

				final := session.NewEvent(ctx, ctx.InvocationID())
				final.Content = genai.NewContentFromText("done", genai.RoleModel)
				final.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
					PromptTokenCount:        11,
					CandidatesTokenCount:    3,
					TotalTokenCount:         17,
					CachedContentTokenCount: 4,
				}
				yield(final, nil)
			}
		},
	})
	if err != nil {
		t.Fatalf("agent.New() error: %v", err)
	}

	process, err := newNormaProcess(agentInstance, nil, nil, nil)
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

	created, err := process.NewSession(context.Background(), role, role.ID)
	if err != nil {
		t.Fatalf("ProviderProcess.NewSession() error: %v", err)
	}

	result, err := created.Turn(context.Background(), "task")
	if err != nil {
		t.Fatalf("AgentSession.Turn() error: %v", err)
	}

	if result.Content != "done" {
		t.Errorf("TurnResult.Content = %q, want done", result.Content)
	}

	want := TokenUsage{InputTokens: 11, OutputTokens: 3, TotalTokens: 17, CachedReadTokens: 4}
	if result.Usage == nil || *result.Usage != want {
		t.Errorf("TurnResult.Usage = %+v, want %+v", result.Usage, want)
	}

	reporter := created.(interface{ Configuration() SessionConfiguration })
	wantConfiguration := SessionConfiguration{Model: "gpt-5.6-sol", Reasoning: "medium"}

	if got := reporter.Configuration(); got != wantConfiguration {
		t.Errorf("session configuration = %+v, want %+v", got, wantConfiguration)
	}
}

func TestSessionConfigurationFromStatePreservesFallbackForBlankValues(t *testing.T) {
	t.Parallel()

	fallback := SessionConfiguration{Model: "configured-model", Reasoning: "high"}
	state := map[string]any{
		"config_values": []map[string]any{
			{"id": "model", "value": "  "},
			{"id": "reasoning_effort", "value": ""},
		},
	}

	if got := sessionConfigurationFromState(state, fallback); got != fallback {
		t.Errorf("session configuration = %+v, want fallback %+v", got, fallback)
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
