// Package runtime adapts Callee roles to Norma Runtime for one-shot execution.
package runtime

import (
	"fmt"
	"strings"

	"github.com/baldaworks/callee/internal/role"
	"github.com/normahq/runtime/v2/agentconfig"
)

// Provider identifies one reusable ACP process.
type Provider struct {
	typeName string
	command  []string
	config   agentconfig.Config
}

// Key returns the stable identity of the provider process.
func (p Provider) Key() string {
	return providerKey(p.typeName, p.command)
}

// Type returns the public Callee runtime type for the provider.
func (p Provider) Type() string {
	return p.typeName
}

// ProviderFor returns the reusable ACP process configuration for a role.
func ProviderFor(r role.Role) (Provider, error) {
	cfg, err := Normalize(r)
	if err != nil {
		return Provider{}, err
	}

	resolved, err := agentconfig.NormalizeConfig(cfg, "")
	if err != nil {
		return Provider{}, fmt.Errorf("resolve provider for role %q: %w", r.ID, err)
	}

	command := append([]string(nil), resolved.Command...)

	providerConfig := agentconfig.Config{
		Type:       agentconfig.AgentTypeGenericACP,
		GenericACP: &agentconfig.ACPConfig{Cmd: command},
	}
	if err := providerConfig.Validate(); err != nil {
		return Provider{}, fmt.Errorf("validate provider for role %q: %w", r.ID, err)
	}

	return Provider{typeName: r.Metadata.Type, command: command, config: providerConfig}, nil
}

func providerKey(typeName string, command []string) string {
	return typeName + "\x00" + strings.Join(command, "\x00")
}

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
	case agentconfig.AgentTypeGrokACP:
		cfg.GrokACP = block
	case agentconfig.AgentTypeGenericACP:
		cfg.GenericACP = block
	}

	if err := cfg.Validate(); err != nil {
		return agentconfig.Config{}, fmt.Errorf("role %q: invalid runtime configuration: %w", r.ID, err)
	}

	return cfg, nil
}
