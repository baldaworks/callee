// Package runtime adapts Callee roles to Norma Runtime and manages role conversations.
package runtime

import (
	"fmt"
	"strings"

	"github.com/baldaworks/callee/internal/role"
	"github.com/normahq/runtime/v2/agentconfig"
)

// Normalize returns the official Norma Runtime configuration for a role.
func Normalize(r role.Role) (agentconfig.Config, error) {
	runtimeType, ok := role.RuntimeType(r.Metadata.Type)
	if !ok {
		return agentconfig.Config{}, fmt.Errorf("role %q: unsupported type %q", r.ID, r.Metadata.Type)
	}
	block := &agentconfig.ACPConfig{
		Model: r.Metadata.Model, ReasoningEffort: r.Metadata.Reasoning,
		Mode: r.Metadata.Mode, ExtraArgs: append([]string(nil), r.Metadata.ExtraArgs...),
	}
	if command := strings.TrimSpace(r.Metadata.Cmd); command != "" {
		block.Cmd = []string{command}
	}
	cfg := agentconfig.Config{Type: runtimeType}
	switch runtimeType {
	case agentconfig.AgentTypeCodexACP:
		cfg.CodexACP = block
	case agentconfig.AgentTypeClaudeCodeACP:
		cfg.ClaudeCodeACP = block
	case agentconfig.AgentTypeOpenCodeACP:
		cfg.OpenCodeACP = block
	case agentconfig.AgentTypeCopilotACP:
		cfg.CopilotACP = block
	case agentconfig.AgentTypeGenericACP:
		cfg.GenericACP = block
	}
	if err := cfg.Validate(); err != nil {
		return agentconfig.Config{}, fmt.Errorf("role %q: invalid runtime configuration: %w", r.ID, err)
	}
	return cfg, nil
}
