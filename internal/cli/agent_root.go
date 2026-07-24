package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/spf13/cobra"
)

const (
	agentRootFlagName      = "agent-root"
	defaultProjectAgentDir = ".callee"
)

func loadAgentRegistry(cmd *cobra.Command) (*registry.AgentRegistry, error) {
	options, err := agentLoadOptions(cmd)
	if err != nil {
		return nil, err
	}

	return registry.LoadAgents(options)
}

func agentLoadOptions(cmd *cobra.Command) (registry.AgentLoadOptions, error) {
	root, err := commandAgentRoot(cmd)
	if err != nil {
		return registry.AgentLoadOptions{}, err
	}

	if root == "" {
		return registry.AgentLoadOptions{}, nil
	}

	resolved, err := existingAgentRoot(root)
	if err != nil {
		return registry.AgentLoadOptions{}, err
	}

	return registry.AgentLoadOptions{ExclusiveDir: resolved}, nil
}

func defaultAgentRoot(cmd *cobra.Command) (string, error) {
	root, err := commandAgentRoot(cmd)
	if err != nil {
		return "", err
	}

	if root == "" {
		return defaultProjectAgentDir, nil
	}

	return existingAgentRoot(root)
}

func commandAgentRoot(cmd *cobra.Command) (string, error) {
	value, err := cmd.Flags().GetString(agentRootFlagName)
	if err != nil {
		return "", fmt.Errorf("read --%s: %w", agentRootFlagName, err)
	}

	return strings.TrimSpace(value), nil
}

func existingAgentRoot(path string) (string, error) {
	clean := filepath.Clean(path)

	info, err := os.Stat(clean)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("agent root %q does not exist", path)
		}

		return "", fmt.Errorf("stat agent root %q: %w", path, err)
	}

	if !info.IsDir() {
		return "", fmt.Errorf("agent root %q is not a directory", path)
	}

	return clean, nil
}
