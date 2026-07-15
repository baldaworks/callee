package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/role"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	"google.golang.org/adk/v2/agent"
)

func TestNormalize(t *testing.T) {
	for _, kind := range role.SupportedTypes() {
		metadata := role.Metadata{Description: "x", Type: kind, Model: "m", Reasoning: "high", Mode: "review", ExtraArgs: []string{"--stdio"}, Cmd: "/bin/agent"}
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

type fakeConversation struct {
	prompts   []string
	roles     []string
	threadIDs []string
	closed    bool
	result    Result
	err       error
}

func (f *fakeConversation) Run(_ context.Context, r role.Role, prompt, threadID string) (Result, error) {
	f.prompts = append(f.prompts, prompt)
	f.roles = append(f.roles, r.ID)
	f.threadIDs = append(f.threadIDs, threadID)

	return f.result, f.err
}

func (f *fakeConversation) Close() error {
	f.closed = true

	return nil
}

type fakeFactory struct {
	conversation *fakeConversation
	err          error
	providers    []Provider
}

func (f *fakeFactory) New(_ context.Context, provider Provider) (Conversation, error) {
	f.providers = append(f.providers, provider)
	if f.err != nil {
		return nil, f.err
	}

	return f.conversation, nil
}

func TestRunOnceExecutesAndClosesRuntime(t *testing.T) {
	conversation := &fakeConversation{result: Result{Content: "started", ThreadID: "acp-123"}}
	factory := &fakeFactory{conversation: conversation}
	r := testRole("reviewer", "codex")

	got, err := RunOnce(context.Background(), factory, r, "review this", "acp-previous")
	if err != nil {
		t.Fatal(err)
	}

	if got != (Result{Content: "started", ThreadID: "acp-123"}) {
		t.Fatalf("result = %#v", got)
	}

	if !conversation.closed {
		t.Fatal("runtime was not closed")
	}

	if !reflect.DeepEqual(conversation.prompts, []string{"review this"}) || !reflect.DeepEqual(conversation.roles, []string{"reviewer"}) || !reflect.DeepEqual(conversation.threadIDs, []string{"acp-previous"}) {
		t.Fatalf("conversation = %#v", conversation)
	}

	if len(factory.providers) != 1 || factory.providers[0].Type() != "codex" {
		t.Fatalf("providers = %#v", factory.providers)
	}
}

func TestRunOnceClosesRuntimeAfterExecutionError(t *testing.T) {
	conversation := &fakeConversation{err: errors.New("agent failed")}

	_, err := RunOnce(context.Background(), &fakeFactory{conversation: conversation}, testRole("reviewer", "codex"), "review this", "")
	if err == nil || !errors.Is(err, conversation.err) {
		t.Fatalf("RunOnce error = %v", err)
	}

	if !conversation.closed {
		t.Fatal("runtime was not closed")
	}
}

func TestRunOnceReturnsFactoryError(t *testing.T) {
	factoryErr := errors.New("agent unavailable")

	_, err := RunOnce(context.Background(), &fakeFactory{err: factoryErr}, testRole("reviewer", "codex"), "review this", "")
	if err == nil || !errors.Is(err, factoryErr) {
		t.Fatalf("RunOnce error = %v", err)
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
	buildNormaAgent = func(ctx context.Context, provider Provider) (agent.Agent, interface{ Close() error }, error) {
		if provider.Type() != "codex" {
			t.Fatalf("provider type = %q, want codex", provider.Type())
		}

		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("check context has no deadline")
		}

		return nil, closer, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := (NormaFactory{}).Check(ctx, testRole("reviewer", "codex")); err != nil {
		t.Fatal(err)
	}

	if !closer.closed {
		t.Fatal("runtime was not closed")
	}
}

func TestRoleSessionState(t *testing.T) {
	r := testRole("reviewer", "codex")
	r.Metadata.Model = "gpt-5.6-sol"
	r.Metadata.Mode = "review"
	r.Metadata.Reasoning = "high"
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

func TestGrokRoleSessionStateUsesOnlyACPConfigValues(t *testing.T) {
	r := testRole("reviewer", "grok")
	r.Metadata.Model = "grok-4.5"
	r.Metadata.Mode = "plan"
	r.Metadata.Reasoning = "high"

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
	r.Metadata.Mode = "review"
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

func TestCodexProviderUsesRuntimeBridgeVersion(t *testing.T) {
	provider, err := ProviderFor(testRole("reviewer", "codex"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"npx", "-y", "@normahq/codex-acp-bridge@1.7.3"}
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
	return role.Role{ID: id, Metadata: role.Metadata{Description: id, Type: kind}, Template: "{{ prompt }}"}
}
