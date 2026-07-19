// Package runtime adapts Callee agents to Norma Runtime ACP processes.
package runtime

import (
	"fmt"
	"os"
	"strings"

	resource "github.com/baldaworks/callee/internal/agent"
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

// ProviderForAgent returns the reusable ACP process configuration for a Role.
func ProviderForAgent(r resource.Resource) (Provider, error) {
	if r.Kind != resource.RoleKind || r.Spec.Provider == nil {
		return Provider{}, fmt.Errorf("agent %q is not a provider-backed Role", r.ID)
	}

	cfg, err := NormalizeAgent(r)
	if err != nil {
		return Provider{}, err
	}

	if r.Spec.Provider.Type == "codex" && strings.TrimSpace(r.Spec.Provider.Cmd) == "" {
		executable, err := os.Executable()
		if err != nil {
			return Provider{}, fmt.Errorf("resolve current executable for agent %q: %w", r.ID, err)
		}

		cfg.CodexACP.Cmd = []string{executable, "bridge", "codex"}
	}

	resolved, err := agentconfig.NormalizeConfig(cfg, "")
	if err != nil {
		return Provider{}, fmt.Errorf("resolve provider for agent %q: %w", r.ID, err)
	}

	command := append([]string(nil), resolved.Command...)

	providerConfig := agentconfig.Config{
		Type:       agentconfig.AgentTypeGenericACP,
		GenericACP: &agentconfig.ACPConfig{Cmd: command},
	}
	if err := providerConfig.Validate(); err != nil {
		return Provider{}, fmt.Errorf("validate provider for agent %q: %w", r.ID, err)
	}

	return Provider{typeName: r.Spec.Provider.Type, command: command, config: providerConfig}, nil
}

func providerKey(typeName string, command []string) string {
	return typeName + "\x00" + strings.Join(command, "\x00")
}

// NormalizeAgent returns the Norma Runtime session configuration for a Role.
func NormalizeAgent(r resource.Resource) (agentconfig.Config, error) {
	provider := r.Spec.Provider
	if provider == nil {
		return agentconfig.Config{}, fmt.Errorf("agent %q: missing spec.provider", r.ID)
	}

	runtimeType, ok := resource.RuntimeType(provider.Type)
	if !ok {
		return agentconfig.Config{}, fmt.Errorf("agent %q: unsupported spec.provider.type %q", r.ID, provider.Type)
	}

	block := &agentconfig.ACPConfig{
		Model:           provider.Model,
		ReasoningEffort: provider.Reasoning,
		Mode:            provider.Mode,
		ExtraArgs:       append([]string(nil), provider.ExtraArgs...),
	}
	if command := strings.TrimSpace(provider.Cmd); command != "" {
		block.Cmd = []string{command}
	}

	if provider.Type == "cursor" && len(block.Cmd) == 0 {
		block.Cmd = []string{"agent", "acp"}
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
		return agentconfig.Config{}, fmt.Errorf("agent %q: invalid runtime configuration: %w", r.ID, err)
	}

	return cfg, nil
}
