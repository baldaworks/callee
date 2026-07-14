// Package server exposes Callee through the official MCP Go SDK.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Input is the sole Callee MCP tool input.
type Input struct {
	Role     string `json:"role"`
	Prompt   string `json:"prompt"`
	ThreadID string `json:"threadId,omitempty"`
}

// Output is the sole Callee MCP tool output.
type Output struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
}

// MCP serves the one-tool MCP surface.
type MCP struct {
	registry *registry.Registry
	manager  *runtime.Manager
}

func New(reg *registry.Registry, manager *runtime.Manager) *MCP {
	return &MCP{registry: reg, manager: manager}
}

// Definition returns the exact public tool definition.
func (s *MCP) Definition() *mcp.Tool {
	return &mcp.Tool{Name: "callee", Description: description(s.registry), InputSchema: map[string]any{
		"type": "object", "properties": map[string]any{
			"role":     map[string]any{"type": "string", "description": "Configured role to start or continue.", "enum": s.registry.IDs()},
			"prompt":   map[string]any{"type": "string", "description": "Initial task or follow-up prompt."},
			"threadId": map[string]any{"type": "string", "description": "Existing thread ID for this role."},
		}, "required": []string{"role", "prompt"}, "additionalProperties": false,
	}, OutputSchema: map[string]any{"type": "object", "properties": map[string]any{"threadId": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"threadId", "content"}}}
}

func description(reg *registry.Registry) string {
	var b strings.Builder
	b.WriteString("Start or continue an ACP agent role.\n\nAvailable roles:")
	for _, r := range reg.Roles() {
		fmt.Fprintf(&b, "\n- %s — %s", r.ID, r.Metadata.Description)
	}
	return b.String()
}

// Install registers exactly one MCP tool.
func (s *MCP) Install(m *mcp.Server) { m.AddTool(s.Definition(), s.handle) }

func (s *MCP) handle(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input Input
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("decode callee tool input: %w", err)
	}
	if input.Role == "" || input.Prompt == "" {
		return s.error("role and prompt are required"), nil
	}
	output, err := s.Call(ctx, input)
	if err != nil {
		return s.error(err.Error()), nil
	}
	return &mcp.CallToolResult{StructuredContent: output, Content: []mcp.Content{&mcp.TextContent{Text: output.Content}}}, nil
}

func (s *MCP) error(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

// Call executes the tool logic and is used by integration-style tests.
func (s *MCP) Call(ctx context.Context, input Input) (Output, error) {
	r, err := s.registry.Get(input.Role)
	if err != nil {
		return Output{}, err
	}
	if input.ThreadID != "" {
		content, err := s.manager.Reply(ctx, r, input.ThreadID, input.Prompt)
		return Output{ThreadID: input.ThreadID, Content: content}, err
	}
	rendered, err := r.Render(input.Prompt)
	if err != nil {
		return Output{}, err
	}
	id, content, err := s.manager.Start(ctx, r, rendered)
	return Output{ThreadID: id, Content: content}, err
}

// RunStdio serves standard MCP over stdio.
func (s *MCP) RunStdio(ctx context.Context, version string) error {
	m := mcp.NewServer(&mcp.Implementation{Name: "callee", Version: version}, nil)
	s.Install(m)
	return m.Run(ctx, &mcp.StdioTransport{})
}
