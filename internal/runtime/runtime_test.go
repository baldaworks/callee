package runtime

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/baldaworks/callee/internal/role"
	acpagent "github.com/normahq/go-adk-acpagent/v2"
	"google.golang.org/adk/v2/agent"
)

func TestNormalize(t *testing.T) {
	for _, kind := range role.SupportedTypes() {
		metadata := role.Metadata{Description: "x", Type: kind, Model: "m", Reasoning: "high", Mode: "review", ExtraArgs: []string{"--stdio"}}
		metadata.Cmd = "/bin/agent"
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
	n       int
	prompts []string
	roles   []string
	closed  bool
}

func (f *fakeConversation) Start(_ context.Context, r role.Role, p string) (string, string, error) {
	f.n++
	f.prompts = append(f.prompts, p)
	f.roles = append(f.roles, r.ID)

	return fmt.Sprintf("t%d", f.n), "started", nil
}
func (f *fakeConversation) Reply(_ context.Context, _ string, p string) (string, error) {
	f.prompts = append(f.prompts, p)

	return "replied", nil
}
func (f *fakeConversation) Close() error {
	f.closed = true

	return nil
}

type fakeFactory struct {
	n             int
	conversations map[string]*fakeConversation
}

func (f *fakeFactory) New(provider Provider) (Conversation, error) {
	f.n++
	c := &fakeConversation{}

	if f.conversations == nil {
		f.conversations = map[string]*fakeConversation{}
	}

	f.conversations[provider.Key()] = c

	return c, nil
}
func TestManagerSharesProviderRuntimeAcrossRoles(t *testing.T) {
	f := &fakeFactory{}
	m := NewManager(f)
	reviewer := testRole("reviewer", "codex")
	reviewer.Metadata.Mode = "review"
	explorer := testRole("explorer", "codex")
	explorer.Metadata.Mode = "ask"

	a, _, err := m.Start(context.Background(), reviewer, "one")
	if err != nil {
		t.Fatal(err)
	}

	b, _, err := m.Start(context.Background(), explorer, "two")
	if err != nil {
		t.Fatal(err)
	}

	if a == b || f.n != 1 {
		t.Fatal(a, b, f.n)
	}

	if len(a) < 5 || a[:4] != "cal_" {
		t.Fatal("thread ID is not opaque", a)
	}

	if _, err := m.Reply(context.Background(), a, "reply"); err != nil {
		t.Fatal(err)
	}

	_ = m.Close()

	provider, err := ProviderFor(reviewer)
	if err != nil {
		t.Fatal(err)
	}

	if !f.conversations[provider.Key()].closed {
		t.Fatal("not closed")
	}
}

func TestManagerSeparatesProviderCommands(t *testing.T) {
	f := &fakeFactory{}
	m := NewManager(f)
	first := testRole("first", "codex")
	second := testRole("second", "codex")
	second.Metadata.ExtraArgs = []string{"--profile", "other"}

	if _, _, err := m.Start(context.Background(), first, "one"); err != nil {
		t.Fatal(err)
	}

	if _, _, err := m.Start(context.Background(), second, "two"); err != nil {
		t.Fatal(err)
	}

	if f.n != 2 {
		t.Fatalf("provider starts = %d, want 2", f.n)
	}
}

type crashingConversation struct{ fakeConversation }

func (c *crashingConversation) Reply(context.Context, string, string) (string, error) {
	return "", io.EOF
}

type crashingFactory struct {
	n            int
	conversation *crashingConversation
}

func (f *crashingFactory) New(Provider) (Conversation, error) {
	f.n++
	if f.conversation == nil {
		f.conversation = &crashingConversation{}
	}

	return f.conversation, nil
}

func TestManagerInvalidatesThreadsAfterRuntimeCrash(t *testing.T) {
	f := &crashingFactory{}
	m := NewManager(f)
	reviewer := testRole("reviewer", "codex")
	explorer := testRole("explorer", "codex")

	threadID, _, err := m.Start(context.Background(), reviewer, "one")
	if err != nil {
		t.Fatal(err)
	}

	otherThreadID, _, err := m.Start(context.Background(), explorer, "other")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := m.Reply(context.Background(), threadID, "two"); err == nil {
		t.Fatal("reply error = nil")
	}

	if _, err := m.Reply(context.Background(), threadID, "three"); err == nil || !strings.Contains(err.Error(), "no longer available") {
		t.Fatal(err)
	}

	if _, err := m.Reply(context.Background(), otherThreadID, "three"); err == nil || !strings.Contains(err.Error(), "no longer available") {
		t.Fatal(err)
	}

	if _, _, err := m.Start(context.Background(), reviewer, "four"); err != nil {
		t.Fatal(err)
	}

	if f.n != 2 {
		t.Fatalf("runtime starts = %d, want 2", f.n)
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
	state := roleSessionState(r)

	acpState, ok := state[acpagent.SessionStateKey].(map[string]any)
	if !ok {
		t.Fatalf("ACP state = %#v", state)
	}

	values, ok := acpState["config_values"].([]acpagent.SessionConfigValue)
	if !ok || len(values) != 2 {
		t.Fatalf("session config values = %#v", acpState["config_values"])
	}

	if values[0] != acpagent.SelectSessionConfigValue("model", "gpt-5.6-sol") || values[1] != acpagent.SelectSessionConfigValue("mode", "review") {
		t.Fatalf("session config values = %#v", values)
	}

	meta := acpState["meta"].(map[string]any)
	codex := meta["codex"].(map[string]any)

	config := codex["config"].(map[string]any)
	if got, want := config["model_reasoning_effort"], "high"; got != want {
		t.Fatalf("reasoning effort = %q, want %q", got, want)
	}
}

func testRole(id, kind string) role.Role {
	return role.Role{ID: id, Metadata: role.Metadata{Description: id, Type: kind}, Template: "{{ prompt }}"}
}
