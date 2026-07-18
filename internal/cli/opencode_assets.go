package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/opencode assets/cursor
var localIntegrationAssets embed.FS

type localIntegrationAsset struct {
	source      string
	destination string
}

var openCodeAssetFiles = []localIntegrationAsset{
	{source: "assets/opencode/skills/callee-run-agent/SKILL.md", destination: ".opencode/skills/callee-run-agent/SKILL.md"},
	{source: "assets/opencode/skills/callee-create-agent/SKILL.md", destination: ".opencode/skills/callee-create-agent/SKILL.md"},
	{source: "assets/opencode/skills/callee-create-agent/references/workflows.md", destination: ".opencode/skills/callee-create-agent/references/workflows.md"},
	{source: "assets/opencode/commands/callee.md", destination: ".opencode/commands/callee.md"},
	{source: "assets/opencode/commands/callee-create-agent.md", destination: ".opencode/commands/callee-create-agent.md"},
}

var cursorAssetFiles = []localIntegrationAsset{
	{source: "assets/cursor/skills/callee-run-agent/SKILL.md", destination: ".cursor/skills/callee-run-agent/SKILL.md"},
	{source: "assets/cursor/skills/callee-create-agent/SKILL.md", destination: ".cursor/skills/callee-create-agent/SKILL.md"},
	{source: "assets/cursor/skills/callee-create-agent/references/workflows.md", destination: ".cursor/skills/callee-create-agent/references/workflows.md"},
}

func writeOpenCodeIntegration(force bool) (setupInstallResult, error) {
	return writeLocalIntegration("OpenCode", openCodeAssetFiles, force)
}

func writeCursorIntegration(force bool) (setupInstallResult, error) {
	return writeLocalIntegration("Cursor", cursorAssetFiles, force)
}

func writeLocalIntegration(host string, assets []localIntegrationAsset, force bool) (setupInstallResult, error) {
	result := setupInstallResult{}

	for _, asset := range assets {
		content, err := localIntegrationAssets.ReadFile(asset.source)
		if err != nil {
			return setupInstallResult{}, fmt.Errorf("read embedded %s asset %q: %w", host, asset.source, err)
		}

		created, err := writeSetupFile(filepath.FromSlash(asset.destination), content, force)
		if err != nil {
			return setupInstallResult{}, fmt.Errorf("write %s asset %q: %w", host, asset.destination, err)
		}

		if created {
			result.created = append(result.created, asset.destination)
		} else {
			result.unchanged = append(result.unchanged, asset.destination)
		}
	}

	return result, nil
}

func writeSetupFile(path string, content []byte, force bool) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("check existing file: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), setupDirMode); err != nil {
		return false, fmt.Errorf("create parent directory: %w", err)
	}

	if err := os.WriteFile(path, content, setupFileMode); err != nil {
		return false, fmt.Errorf("write file: %w", err)
	}

	return true, nil
}
