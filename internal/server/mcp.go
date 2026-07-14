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

// Input is a Callee MCP tool input.
type Input struct {
	Role     string `json:"role"`
	Prompt   string `json:"prompt"`
	ThreadID string `json:"threadId,omitempty"`
}

// Output is a Callee MCP tool output.
type Output struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
}

// MCP serves the Callee MCP tools.
type MCP struct {
	registry *registry.Registry
	manager  *runtime.Manager
}

func New(reg *registry.Registry, manager *runtime.Manager) *MCP {
	return &MCP{registry: reg, manager: manager}
}

// StartDefinition returns the tool definition for new role conversations.
func (s *MCP) StartDefinition() *mcp.Tool {
	return &mcp.Tool{Name: "callee", Description: description(s.registry), InputSchema: map[string]any{
		"type": "object", "properties": map[string]any{
			"role":   map[string]any{"type": "string", "description": "Registered Callee role to invoke.", "enum": s.registry.IDs()},
			"prompt": map[string]any{"type": "string", "description": "Task to send to the selected role."},
		}, "required": []string{"role", "prompt"}, "additionalProperties": false,
	}, OutputSchema: outputSchema()}
}

// ReplyDefinition returns the tool definition for existing role conversations.
func (s *MCP) ReplyDefinition() *mcp.Tool {
	return &mcp.Tool{Name: "callee-reply", Description: replyDescription(s.registry), InputSchema: map[string]any{
		"type": "object", "properties": map[string]any{
			"threadId": map[string]any{"type": "string", "description": "Opaque thread ID previously returned by Callee."},
			"prompt":   map[string]any{"type": "string", "description": "Follow-up prompt for the existing conversation."},
		}, "required": []string{"threadId", "prompt"}, "additionalProperties": false,
	}, OutputSchema: outputSchema()}
}

func outputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{"threadId": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"threadId", "content"}, "additionalProperties": false}
}

func description(reg *registry.Registry) string {
	var b strings.Builder
	b.WriteString("Start a new ACP agent role conversation.\n\nAvailable roles:")
	for _, r := range reg.Roles() {
		fmt.Fprintf(&b, "\n- %s — %s", r.ID, r.Metadata.Description)
	}
	return b.String()
}

func replyDescription(reg *registry.Registry) string {
	return "Continue an existing ACP agent role conversation using the opaque thread ID returned by Callee.\n\n" + strings.TrimPrefix(description(reg), "Start a new ACP agent role conversation.\n\n")
}

// Install registers the Callee start and reply tools.
func (s *MCP) Install(m *mcp.Server) {
	m.AddTool(s.StartDefinition(), s.handleStart)
	m.AddTool(s.ReplyDefinition(), s.handleReply)
}

func (s *MCP) handleStart(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input Input
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("decode callee tool input: %w", err)
	}
	if input.Role == "" || input.Prompt == "" || input.ThreadID != "" {
		return s.error("role and prompt are required"), nil
	}
	output, err := s.Start(ctx, input)
	if err != nil {
		return s.error(err.Error()), nil
	}
	return result(output), nil
}

func (s *MCP) handleReply(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var input Input
	if err := json.Unmarshal(request.Params.Arguments, &input); err != nil {
		return nil, fmt.Errorf("decode callee-reply tool input: %w", err)
	}
	if input.ThreadID == "" || input.Prompt == "" || input.Role != "" {
		return s.error("threadId and prompt are required"), nil
	}
	output, err := s.Reply(ctx, input)
	if err != nil {
		return s.error(err.Error()), nil
	}
	return result(output), nil
}

func result(output Output) *mcp.CallToolResult {
	return &mcp.CallToolResult{StructuredContent: output, Content: []mcp.Content{&mcp.TextContent{Text: output.Content}}}
}

func (s *MCP) error(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: message}}}
}

// Start executes the new-conversation tool logic.
func (s *MCP) Start(ctx context.Context, input Input) (Output, error) {
	r, err := s.registry.Get(input.Role)
	if err != nil {
		return Output{}, err
	}
	rendered, err := r.Render(input.Prompt)
	if err != nil {
		return Output{}, err
	}
	id, content, err := s.manager.Start(ctx, r, rendered)
	return Output{ThreadID: id, Content: content}, err
}

// Reply executes the existing-conversation tool logic.
func (s *MCP) Reply(ctx context.Context, input Input) (Output, error) {
	content, err := s.manager.Reply(ctx, input.ThreadID, input.Prompt)
	return Output{ThreadID: input.ThreadID, Content: content}, err
}

// RunStdio serves standard MCP over stdio.
func (s *MCP) RunStdio(ctx context.Context, version string) error {
	m := mcp.NewServer(&mcp.Implementation{Name: "callee", Version: version}, nil)
	s.Install(m)
	return m.Run(ctx, &mcp.StdioTransport{})
}
