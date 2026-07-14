package runtime

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/role"
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
	if len(a) < 5 || a[:4] != "cal_" {
		t.Fatal("thread ID is not opaque", a)
	}
	if _, err := m.Reply(context.Background(), a, "reply"); err != nil {
		t.Fatal(err)
	}
	_ = m.Close()
	if !f.conversations["reviewer"].closed {
		t.Fatal("not closed")
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

func (f *crashingFactory) New(role.Role) (Conversation, error) {
	f.n++
	if f.conversation == nil {
		f.conversation = &crashingConversation{}
	}
	return f.conversation, nil
}

func TestManagerInvalidatesThreadsAfterRuntimeCrash(t *testing.T) {
	f := &crashingFactory{}
	m := NewManager(f)
	r := role.Role{ID: "reviewer"}
	threadID, _, err := m.Start(context.Background(), r, "one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Reply(context.Background(), threadID, "two"); err == nil {
		t.Fatal("reply error = nil")
	}
	if _, err := m.Reply(context.Background(), threadID, "three"); err == nil || !strings.Contains(err.Error(), "no longer available") {
		t.Fatal(err)
	}
	if _, _, err := m.Start(context.Background(), r, "four"); err != nil {
		t.Fatal(err)
	}
	if f.n != 2 {
		t.Fatalf("runtime starts = %d, want 2", f.n)
	}
}
