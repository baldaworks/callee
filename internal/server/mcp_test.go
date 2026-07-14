package server

import (
	"context"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
)

type fake struct {
	prompts []string
	n       int
}

func (f *fake) Start(_ context.Context, _ role.Role, p string) (string, string, error) {
	f.n++
	f.prompts = append(f.prompts, p)

	return "thread", "first", nil
}
func (f *fake) Reply(_ context.Context, _ string, p string) (string, error) {
	f.prompts = append(f.prompts, p)

	return "reply", nil
}
func (*fake) Close() error { return nil }

type factory struct{ f *fake }

func (f factory) New(runtime.Provider) (runtime.Conversation, error) { return f.f, nil }
func TestToolCall(t *testing.T) {
	r, _ := registry.New([]role.Role{{ID: "reviewer", Metadata: role.Metadata{Description: "Reviews code", Type: "codex"}, Template: "Task: {{ prompt }}"}})
	f := &fake{}
	s := New(r, runtime.NewManager(factory{f}))

	start, reply := s.StartDefinition(), s.ReplyDefinition()
	if start.Name != "callee" || reply.Name != "callee-reply" {
		t.Fatal(start.Name, reply.Name)
	}

	startProps := start.InputSchema.(map[string]any)["properties"].(map[string]any)

	replyProps := reply.InputSchema.(map[string]any)["properties"].(map[string]any)
	if len(startProps) != 2 || len(replyProps) != 2 {
		t.Fatal(startProps, replyProps)
	}

	if !strings.Contains(start.Description, "reviewer — Reviews code") || !strings.Contains(reply.Description, "reviewer — Reviews code") {
		t.Fatal("missing role descriptions")
	}

	first, err := s.Start(context.Background(), Input{Role: "reviewer", Prompt: "first"})
	if err != nil || !strings.HasPrefix(first.ThreadID, "cal_") {
		t.Fatal(first, err)
	}

	second, err := s.Reply(context.Background(), Input{ThreadID: first.ThreadID, Prompt: "followup"})
	if err != nil || second.Content != "reply" {
		t.Fatal(second, err)
	}

	if f.prompts[0] != "Task: first" || f.prompts[1] != "followup" {
		t.Fatal(f.prompts)
	}

	if _, err := s.Reply(context.Background(), Input{ThreadID: "missing", Prompt: "x"}); err == nil {
		t.Fatal("missing thread")
	}
}
