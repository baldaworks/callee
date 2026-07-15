package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed assets/opencode/skills/callee-run-role/SKILL.md assets/opencode/skills/callee-create-role/SKILL.md assets/opencode/commands/callee.md assets/opencode/commands/callee-promptkit.md
var openCodeAssets embed.FS

type openCodeAsset struct {
	source      string
	destination string
}

var openCodeAssetFiles = []openCodeAsset{
	{source: "assets/opencode/skills/callee-run-role/SKILL.md", destination: ".opencode/skills/callee-run-role/SKILL.md"},
	{source: "assets/opencode/skills/callee-create-role/SKILL.md", destination: ".opencode/skills/callee-create-role/SKILL.md"},
	{source: "assets/opencode/commands/callee.md", destination: ".opencode/commands/callee.md"},
	{source: "assets/opencode/commands/callee-promptkit.md", destination: ".opencode/commands/callee-promptkit.md"},
}

func writeOpenCodeIntegration(force bool) (setupInstallResult, error) {
	result := setupInstallResult{}

	for _, asset := range openCodeAssetFiles {
		content, err := openCodeAssets.ReadFile(asset.source)
		if err != nil {
			return setupInstallResult{}, fmt.Errorf("read embedded OpenCode asset %q: %w", asset.source, err)
		}

		created, err := writeSetupFile(filepath.FromSlash(asset.destination), content, force)
		if err != nil {
			return setupInstallResult{}, fmt.Errorf("write OpenCode asset %q: %w", asset.destination, err)
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

	if err := os.MkdirAll(filepath.Dir(path), reviewerDirMode); err != nil {
		return false, fmt.Errorf("create parent directory: %w", err)
	}

	if err := os.WriteFile(path, content, reviewerFileMode); err != nil {
		return false, fmt.Errorf("write file: %w", err)
	}

	return true, nil
}
