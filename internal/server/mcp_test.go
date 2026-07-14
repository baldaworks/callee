package server

import (
	"context"
	"strings"
	"testing"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

type factory struct {
	f        *fake
	newCalls *int
}

func (f factory) New(context.Context, runtime.Provider) (runtime.Conversation, error) {
	if f.newCalls != nil {
		*f.newCalls++
	}

	return f.f, nil
}

func TestToolCall(t *testing.T) {
	r, _ := registry.New([]role.Role{{ID: "reviewer", Metadata: role.Metadata{Description: "Reviews code", Type: "codex"}, Template: "Task: {{ prompt }}"}})
	f := &fake{}
	s := New(r, runtime.NewManager(factory{f: f}))

	start, reply, list := s.StartDefinition(), s.ReplyDefinition(), s.RoleListDefinition()
	if start.Name != roleTool || reply.Name != roleReplyTool || list.Name != roleListTool {
		t.Fatal(start.Name, reply.Name, list.Name)
	}

	startProps := start.InputSchema.(map[string]any)["properties"].(map[string]any)

	replyProps := reply.InputSchema.(map[string]any)["properties"].(map[string]any)
	if len(startProps) != 2 || len(replyProps) != 2 {
		t.Fatal(startProps, replyProps)
	}

	if _, ok := list.InputSchema.(map[string]any)["required"]; ok {
		t.Fatal("role list must not require arguments")
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

func TestListRoles(t *testing.T) {
	r, err := registry.New([]role.Role{
		{ID: "zebra", Metadata: role.Metadata{Description: "Zebra role", Type: "codex"}, Template: "{{ prompt }}"},
		{ID: "alpha", Metadata: role.Metadata{Description: "Alpha role", Type: "claude"}, Template: "{{ prompt }}"},
	})
	if err != nil {
		t.Fatal(err)
	}

	f := &fake{}
	s := New(r, runtime.NewManager(factory{f: f}))

	got := s.ListRoles()

	want := []RoleInfo{
		{ID: "alpha", Description: "Alpha role"},
		{ID: "zebra", Description: "Zebra role"},
	}
	if len(got.Roles) != len(want) {
		t.Fatalf("listed %d roles, want %d", len(got.Roles), len(want))
	}

	for i := range want {
		if got.Roles[i] != want[i] {
			t.Errorf("role %d = %#v, want %#v", i, got.Roles[i], want[i])
		}
	}

	if f.n != 0 {
		t.Fatalf("started %d providers while listing roles", f.n)
	}

	result, err := s.handleRoleList(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "alpha — Alpha role") {
		t.Fatal("missing role-list text response")
	}
}

func TestInitializeStartsConfiguredProviders(t *testing.T) {
	r, err := registry.New([]role.Role{
		{ID: "reviewer", Metadata: role.Metadata{Description: "Reviews code", Type: "codex"}, Template: "{{ prompt }}"},
		{ID: "explorer", Metadata: role.Metadata{Description: "Explores code", Type: "codex"}, Template: "{{ prompt }}"},
	})
	if err != nil {
		t.Fatal(err)
	}

	f := &fake{}
	newCalls := 0

	s := New(r, runtime.NewManager(factory{f: f, newCalls: &newCalls}))
	if err := s.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}

	if newCalls != 1 {
		t.Fatalf("provider starts = %d, want 1", newCalls)
	}

	if f.n != 0 {
		t.Fatalf("a runtime conversation received a prompt at startup, got %d", f.n)
	}
}

func TestInstallPublishesOnlyUnprefixedTools(t *testing.T) {
	r, err := registry.New([]role.Role{
		{ID: "reviewer", Metadata: role.Metadata{Description: "Reviews code", Type: "codex"}, Template: "{{ prompt }}"},
	})
	if err != nil {
		t.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	New(r, runtime.NewManager(factory{f: &fake{}})).Install(server)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = server.Run(ctx, serverTransport) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]bool, len(tools.Tools))
	for _, tool := range tools.Tools {
		got[tool.Name] = true
	}

	want := map[string]bool{
		"role":       true,
		"role.reply": true,
		"role.list":  true,
	}
	if len(got) != len(want) {
		t.Fatalf("published tools = %#v, want %#v", got, want)
	}

	for name := range want {
		if !got[name] {
			t.Errorf("missing tool %q", name)
		}
	}

	if got["callee"] || got["callee-reply"] || got["callee.role"] || got["callee.role.reply"] || got["callee.role.list"] || got["subagent.prompt"] || got["subagent.reply"] {
		t.Errorf("legacy tools remain published: %#v", got)
	}
}
