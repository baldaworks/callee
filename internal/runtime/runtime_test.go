package runtime

import (
	"context"
	"fmt"
	"testing"

	"github.com/baldaworks/callee/internal/role"
)

func TestNormalize(t *testing.T) {
	for _, kind := range role.SupportedTypes() {
		metadata := role.Metadata{Description: "x", Type: kind, Model: "m", Reasoning: "high", Mode: "review", ExtraArgs: []string{"--stdio"}}
		if kind == "generic_acp" {
			metadata.Path = "/bin/agent"
		}
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
	closed  bool
}

func (f *fakeConversation) Start(_ context.Context, p string) (string, string, error) {
	f.n++
	f.prompts = append(f.prompts, p)
	return fmt.Sprintf("t%d", f.n), "started", nil
}
func (f *fakeConversation) Reply(_ context.Context, _ string, p string) (string, error) {
	f.prompts = append(f.prompts, p)
	return "replied", nil
}
func (f *fakeConversation) Close() error { f.closed = true; return nil }

type fakeFactory struct {
	n             int
	conversations map[string]*fakeConversation
}

func (f *fakeFactory) New(r role.Role) (Conversation, error) {
	f.n++
	c := &fakeConversation{}
	if f.conversations == nil {
		f.conversations = map[string]*fakeConversation{}
	}
	f.conversations[r.ID] = c
	return c, nil
}
func TestManagerReusesRoleRuntime(t *testing.T) {
	f := &fakeFactory{}
	m := NewManager(f)
	r := role.Role{ID: "reviewer"}
	a, _, _ := m.Start(context.Background(), r, "one")
	b, _, _ := m.Start(context.Background(), r, "two")
	if a == b || f.n != 1 {
		t.Fatal(a, b, f.n)
	}
	if _, err := m.Reply(context.Background(), role.Role{ID: "other"}, a, "x"); err == nil {
		t.Fatal("wrong role allowed")
	}
	if _, err := m.Reply(context.Background(), r, a, "reply"); err != nil {
		t.Fatal(err)
	}
	_ = m.Close()
	if !f.conversations["reviewer"].closed {
		t.Fatal("not closed")
	}
}
