package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/server"
	"go.uber.org/fx/fxtest"
)

type lifecycleConversation struct{ closed bool }

func (c *lifecycleConversation) Start(context.Context, role.Role, string) (string, string, error) {
	return "backend-thread", "started", nil
}

func (c *lifecycleConversation) Reply(context.Context, string, string) (string, error) {
	return "replied", nil
}

func (c *lifecycleConversation) Close() error {
	c.closed = true

	return nil
}

type lifecycleFactory struct{ conversation *lifecycleConversation }

func (f *lifecycleFactory) New(context.Context, runtime.Provider) (runtime.Conversation, error) {
	if f.conversation == nil {
		f.conversation = &lifecycleConversation{}
	}

	return f.conversation, nil
}

func TestMCPAppStopsRoleRuntime(t *testing.T) {
	reg, err := registry.New([]role.Role{{ID: "reviewer", Metadata: role.Metadata{Description: "Reviews code", Type: "codex"}, Template: "Review {{ prompt }}"}})
	if err != nil {
		t.Fatal(err)
	}

	factory := &lifecycleFactory{}

	var mcpServer *server.MCP

	app := fxtest.New(t, mcpOptions(reg, factory, &mcpServer))
	app.RequireStart()

	if _, err := mcpServer.Start(context.Background(), server.Input{Role: "reviewer", Prompt: "this"}); err != nil {
		t.Fatal(err)
	}

	app.RequireStop()

	if factory.conversation == nil || !factory.conversation.closed {
		t.Fatal("MCP lifecycle did not close the role runtime")
	}
}

func TestExpectedCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if !isExpectedCancellation(ctx, fmt.Errorf("role runtime: %w", context.Canceled)) {
		t.Fatal("context cancellation was not recognized")
	}
}
