package cli

import (
	"embed"
	"fmt"
	"io"
	"path/filepath"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
)

const starterAssetRoot = "assets/starter/"

//go:embed assets/starter/roles/*.md assets/starter/workflows/*.md
var starterAgentAssets embed.FS

type starterAgentAsset struct {
	id           string
	source       string
	relativePath string
}

type renderedStarterAgent struct {
	resource    agent.Resource
	destination string
	content     []byte
}

var starterAgentFiles = []starterAgentAsset{
	{id: "roles/architect", source: starterAssetRoot + "roles/architect.md", relativePath: "roles/architect.md"},
	{id: "roles/explorer", source: starterAssetRoot + "roles/explorer.md", relativePath: "roles/explorer.md"},
	{id: "roles/implementer", source: starterAssetRoot + "roles/implementer.md", relativePath: "roles/implementer.md"},
	{id: "roles/reviewer", source: starterAssetRoot + "roles/reviewer.md", relativePath: "roles/reviewer.md"},
	{id: "workflows/goalkeeper", source: starterAssetRoot + "workflows/goalkeeper.md", relativePath: "workflows/goalkeeper.md"},
	{id: "workflows/investigate", source: starterAssetRoot + "workflows/investigate.md", relativePath: "workflows/investigate.md"},
}

func writeStarterAgents(providerType, root string, force bool) (setupInstallResult, error) {
	rendered, err := renderStarterAgents(providerType, root)
	if err != nil {
		return setupInstallResult{}, err
	}

	result := setupInstallResult{}

	for _, file := range rendered {
		created, err := writeSetupFile(filepath.FromSlash(file.destination), file.content, force)
		if err != nil {
			return setupInstallResult{}, fmt.Errorf("write starter agent %q: %w", file.resource.ID, err)
		}

		if created {
			result.created = append(result.created, file.destination)
		} else {
			result.unchanged = append(result.unchanged, file.destination)
		}
	}

	return result, nil
}

func renderStarterAgents(providerType, root string) ([]renderedStarterAgent, error) {
	rendered := make([]renderedStarterAgent, 0, len(starterAgentFiles))
	resources := make([]agent.Resource, 0, len(starterAgentFiles))

	for _, file := range starterAgentFiles {
		data, err := starterAgentAssets.ReadFile(file.source)
		if err != nil {
			return nil, fmt.Errorf("read embedded starter agent %q: %w", file.id, err)
		}

		resource, err := agent.DecodeMarkdown(file.id, file.source, data)
		if err != nil {
			return nil, fmt.Errorf("decode embedded starter agent %q: %w", file.id, err)
		}

		if resource.Kind == agent.RoleKind {
			resource.Spec.Provider = &agent.Provider{Type: providerType}
		}

		content, err := agent.EncodeMarkdown(resource)
		if err != nil {
			return nil, fmt.Errorf("encode embedded starter agent %q: %w", file.id, err)
		}

		resources = append(resources, resource)
		rendered = append(rendered, renderedStarterAgent{
			resource:    resource,
			destination: starterAgentDestination(root, file),
			content:     content,
		})
	}

	if _, err := registry.NewAgentRegistry(resources); err != nil {
		return nil, fmt.Errorf("validate embedded starter agents: %w", err)
	}

	return rendered, nil
}

func starterAgentDestination(root string, file starterAgentAsset) string {
	return filepath.Join(root, filepath.FromSlash(file.relativePath))
}

func reportStarterAgentInstall(output io.Writer, targetName string, result setupInstallResult) error {
	if len(result.created) > 0 {
		if _, err := fmt.Fprintf(output, "Installed starter agents for %s:\n", targetName); err != nil {
			return err
		}

		for _, path := range result.created {
			if _, err := fmt.Fprintf(output, "  %s\n", path); err != nil {
				return err
			}
		}
	}

	if len(result.unchanged) > 0 {
		if _, err := fmt.Fprintln(output, "Existing starter agents left unchanged:"); err != nil {
			return err
		}

		for _, path := range result.unchanged {
			if _, err := fmt.Fprintf(output, "  %s\n", path); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprint(output, `
Try the read-only workflow:
  callee agent list
  callee agent view workflows/investigate
  callee agent run workflows/investigate --message "Explain this project's architecture and main entry points"

When you are ready to make changes, use workflows/goalkeeper for iterative implementation and review.
`)

	return err
}
