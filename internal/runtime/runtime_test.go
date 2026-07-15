package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"iter"
	"reflect"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/role"
	acp "github.com/coder/acp-go-sdk"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

func TestNormalize(t *testing.T) {
	for _, kind := range role.SupportedTypes() {
		metadata := role.Metadata{API: role.CurrentAPI, Kind: role.RoleKind, Description: "x", Provider: role.Provider{Type: kind, Model: "m", Reasoning: "high", Mode: "review", ExtraArgs: []string{"--stdio"}, Cmd: "/bin/agent"}}
		r := role.Role{ID: kind, Metadata: metadata, Template: "{{ prompt }}"}

		cfg, err := Normalize(r)
		if err != nil {
			t.Fatal(err)
		}

		want, _ := role.RuntimeType(kind)
		if cfg.Type != want {
			t.Errorf("%s = %s", kind, cfg.Type)
		}
	}
}

type closeTracker struct{ closed bool }

func (c *closeTracker) Close() error {
	c.closed = true

	return nil
}

func TestNormaFactoryCheckClosesRuntime(t *testing.T) {
	original := buildNormaAgent

	t.Cleanup(func() { buildNormaAgent = original })

	closer := &closeTracker{}
	permissionHandler := func(_ context.Context, request acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeSelected(request.Options[0].OptionId)}, nil
	}
	buildNormaAgent = func(ctx context.Context, provider Provider, _ io.Writer, gotPermissionHandler acpagent.PermissionHandler) (agent.Agent, interface{ Close() error }, error) {
		if provider.Type() != "codex" {
			t.Fatalf("provider type = %q, want codex", provider.Type())
		}

		if gotPermissionHandler == nil {
			t.Fatal("permission handler was not passed to Norma Runtime")
		}

		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("check context has no deadline")
		}

		return nil, closer, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := (NormaFactory{PermissionHandler: permissionHandler}).Check(ctx, testRole("reviewer", "codex")); err != nil {
		t.Fatal(err)
	}

	if !closer.closed {
		t.Fatal("runtime was not closed")
	}
}

func TestNormaFactoryWrapsProviderDiagnosticsAsJSON(t *testing.T) {
	original := buildNormaAgent

	t.Cleanup(func() { buildNormaAgent = original })

	var output bytes.Buffer

	buildNormaAgent = func(_ context.Context, _ Provider, stderr io.Writer, _ acpagent.PermissionHandler) (agent.Agent, interface{ Close() error }, error) {
		if _, err := io.WriteString(stderr, "provider started\npartial"); err != nil {
			t.Fatal(err)
		}

		return nil, &closeTracker{}, nil
	}

	if err := (NormaFactory{Stderr: &output, JSONDiagnostics: true}).Check(context.Background(), testRole("reviewer", "codex")); err != nil {
		t.Fatal(err)
	}

	var first, second map[string]any

	decoder := json.NewDecoder(&output)
	if err := decoder.Decode(&first); err != nil {
		t.Fatal(err)
	}

	if err := decoder.Decode(&second); err != nil {
		t.Fatal(err)
	}

	for index, event := range []map[string]any{first, second} {
		if event["source"] != "provider" || event["level"] != "info" {
			t.Fatalf("event %d = %#v", index, event)
		}
	}

	if first["message"] != "provider started" || second["message"] != "partial" {
		t.Fatalf("events = %#v, %#v", first, second)
	}
}

func TestNormaFactoryPassesProviderDiagnosticsThroughWithoutJSON(t *testing.T) {
	original := buildNormaAgent

	t.Cleanup(func() { buildNormaAgent = original })

	var output bytes.Buffer

	buildNormaAgent = func(_ context.Context, _ Provider, stderr io.Writer, _ acpagent.PermissionHandler) (agent.Agent, interface{ Close() error }, error) {
		if _, err := io.WriteString(stderr, "provider started\n"); err != nil {
			t.Fatal(err)
		}

		return nil, &closeTracker{}, nil
	}

	if err := (NormaFactory{Stderr: &output}).Check(context.Background(), testRole("reviewer", "codex")); err != nil {
		t.Fatal(err)
	}

	if got, want := output.String(), "provider started\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func TestRoleSessionState(t *testing.T) {
	r := testRole("reviewer", "codex")
	r.Metadata.Provider.Model = "gpt-5.6-sol"
	r.Metadata.Provider.Mode = "review"
	r.Metadata.Provider.Reasoning = "high"
	state := roleSessionState(r, "")

	acpState, ok := state[acpagent.SessionStateKey].(map[string]any)
	if !ok {
		t.Fatalf("ACP state = %#v", state)
	}

	values, ok := acpState["config_values"].([]acpagent.SessionConfigValue)
	if !ok || len(values) != 3 {
		t.Fatalf("session config values = %#v", acpState["config_values"])
	}

	want := []acpagent.SessionConfigValue{
		acpagent.SelectSessionConfigValue("model", "gpt-5.6-sol"),
		acpagent.SelectSessionConfigValue("mode", "review"),
		acpagent.SelectSessionConfigValue("reasoning_effort", "high"),
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("session config values = %#v, want %#v", values, want)
	}

	if _, ok := acpState["meta"]; ok {
		t.Fatalf("ACP state must not contain vendor metadata: %#v", acpState)
	}
}

func TestNormaConversationReusesADKSessionAcrossTurns(t *testing.T) {
	var sessionIDs []string

	ag, err := agent.New(agent.Config{
		Name:        "test-agent",
		Description: "test agent",
		Run: func(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
			return func(yield func(*session.Event, error) bool) {
				sessionIDs = append(sessionIDs, ctx.Session().ID())

				event := session.NewEvent(ctx, ctx.InvocationID())
				event.Author = "test-agent"
				event.Content = genai.NewContentFromText("done", genai.RoleModel)
				event.TurnComplete = true
				event.Actions.StateDelta = map[string]any{
					acpagent.SessionStateKey: map[string]any{"session_id": "provider-thread"},
				}
				yield(event, nil)
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	conversation, err := newNormaConversation(ag, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	r := testRole("reviewer", "codex")
	for _, prompt := range []string{"first", "second"} {
		result, err := conversation.Run(context.Background(), r, prompt, "")
		if err != nil {
			t.Fatal(err)
		}

		if result.ThreadID != "provider-thread" || result.Content != "done" {
			t.Fatalf("result = %#v", result)
		}
	}

	if len(sessionIDs) != 2 || sessionIDs[0] != sessionIDs[1] {
		t.Fatalf("session IDs = %#v, want one reused ADK session", sessionIDs)
	}
}

func TestGrokRoleSessionStateUsesOnlyACPConfigValues(t *testing.T) {
	r := testRole("reviewer", "grok")
	r.Metadata.Provider.Model = "grok-4.5"
	r.Metadata.Provider.Mode = "plan"
	r.Metadata.Provider.Reasoning = "high"

	state := roleSessionState(r, "")

	acpState := state[acpagent.SessionStateKey].(map[string]any)
	values := acpState["config_values"].([]acpagent.SessionConfigValue)

	want := []acpagent.SessionConfigValue{
		acpagent.SelectSessionConfigValue("model", "grok-4.5"),
		acpagent.SelectSessionConfigValue("mode", "plan"),
		acpagent.SelectSessionConfigValue("reasoning_effort", "high"),
	}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("session config values = %#v, want %#v", values, want)
	}

	if _, ok := acpState["meta"]; ok {
		t.Fatalf("ACP state must not contain vendor metadata: %#v", acpState)
	}
}

func TestCodexRoleSessionStatePassesModeToACPBridge(t *testing.T) {
	r := testRole("reviewer", "codex")
	r.Metadata.Provider.Mode = "review"
	state := roleSessionState(r, "")

	acpState, ok := state[acpagent.SessionStateKey].(map[string]any)
	if !ok {
		t.Fatalf("ACP state = %#v", state)
	}

	values, ok := acpState["config_values"].([]acpagent.SessionConfigValue)
	if !ok {
		t.Fatalf("session config values = %#v", acpState["config_values"])
	}

	if want := []acpagent.SessionConfigValue{acpagent.SelectSessionConfigValue("mode", "review")}; !reflect.DeepEqual(values, want) {
		t.Fatalf("session config values = %#v, want %#v", values, want)
	}
}

func TestRoleSessionStateSeedsRawACPThreadID(t *testing.T) {
	r := testRole("reviewer", "codex")
	state := roleSessionState(r, "acp-session-123")

	acpState, ok := state[acpagent.SessionStateKey].(map[string]any)
	if !ok {
		t.Fatalf("ACP state = %#v", state)
	}

	if got := acpState["session_id"]; got != "acp-session-123" {
		t.Fatalf("session_id = %#v, want raw ACP handle", got)
	}

	if _, ok := acpState["config_values"]; ok {
		t.Fatalf("config_values = %#v, want absent", acpState["config_values"])
	}
}

func TestCodexProviderUsesPinnedBridgeVersion(t *testing.T) {
	provider, err := ProviderFor(testRole("reviewer", "codex"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"npx", "-y", "@normahq/codex-acp-bridge@1.7.4"}
	if !reflect.DeepEqual(provider.command, want) {
		t.Fatalf("Codex ACP command = %#v, want %#v", provider.command, want)
	}
}

func TestGrokProviderUsesRuntimeCommand(t *testing.T) {
	provider, err := ProviderFor(testRole("reviewer", "grok"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"grok", "agent", "stdio"}
	if !reflect.DeepEqual(provider.command, want) {
		t.Fatalf("Grok ACP command = %#v, want %#v", provider.command, want)
	}
}

func testRole(id, kind string) role.Role {
	return role.Role{ID: id, Metadata: role.Metadata{API: role.CurrentAPI, Kind: role.RoleKind, Description: id, Provider: role.Provider{Type: kind}}, Template: "{{ prompt }}"}
}
