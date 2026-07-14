package server

import (
	"context"
	"testing"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
)

type fake struct {
	prompts []string
	n       int
}

func (f *fake) Start(_ context.Context, p string) (string, string, error) {
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

func (f factory) New(role.Role) (runtime.Conversation, error) { return f.f, nil }
func TestToolCall(t *testing.T) {
	r, _ := registry.New([]role.Role{{ID: "reviewer", Metadata: role.Metadata{Description: "Reviews code", Type: "codex"}, Template: "Task: {{ prompt }}"}})
	f := &fake{}
	s := New(r, runtime.NewManager(factory{f}))
	d := s.Definition()
	if d.Name != "callee" {
		t.Fatal(d.Name)
	}
	props := d.InputSchema.(map[string]any)["properties"].(map[string]any)
	if len(props) != 3 {
		t.Fatal(props)
	}
	first, err := s.Call(context.Background(), Input{Role: "reviewer", Prompt: "first"})
	if err != nil || first.ThreadID != "thread" {
		t.Fatal(first, err)
	}
	second, err := s.Call(context.Background(), Input{Role: "reviewer", ThreadID: first.ThreadID, Prompt: "followup"})
	if err != nil || second.Content != "reply" {
		t.Fatal(second, err)
	}
	if f.prompts[0] != "Task: first" || f.prompts[1] != "followup" {
		t.Fatal(f.prompts)
	}
	if _, err := s.Call(context.Background(), Input{Role: "other", ThreadID: "thread", Prompt: "x"}); err == nil {
		t.Fatal("wrong role")
	}
}
