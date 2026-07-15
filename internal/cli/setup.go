package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const (
	reviewerRolePath = ".callee/roles/reviewer.md"
	reviewerDirMode  = 0o755
	reviewerFileMode = 0o644
)

type setupTarget struct {
	name     string
	prepare  func(context.Context, io.Writer) error
	commands [][]string
	role     string
	install  func(bool) (setupInstallResult, error)
}

type setupInstallResult struct {
	created   []string
	unchanged []string
}

var runSetupCommand = runSetupCommandDefault

func setupCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "setup <codex|claude|grok|copilot|opencode>",
		Short: "Install a host integration and create a reviewer role",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := setupTargetFor(args[0])
			if err != nil {
				return err
			}

			if err := installSetupTarget(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), target, force); err != nil {
				return err
			}

			created, err := writeReviewerRole(target.role, force)
			if err != nil {
				return err
			}

			if created {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "Created %s for %s.\n", reviewerRolePath, target.name)
			} else {
				_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s already exists; leaving it unchanged.\n", reviewerRolePath)
			}

			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing setup files")

	return cmd
}

func installSetupTarget(ctx context.Context, stdout, stderr io.Writer, target setupTarget, force bool) error {
	if target.prepare != nil {
		if err := target.prepare(ctx, stderr); err != nil {
			return err
		}
	}

	for _, command := range target.commands {
		if err := runSetupCommand(ctx, stdout, stderr, command[0], command[1:]...); err != nil {
			return err
		}
	}

	if target.install == nil {
		return nil
	}

	result, err := target.install(force)
	if err != nil {
		return err
	}

	return reportOpenCodeInstall(stdout, result)
}

func reportOpenCodeInstall(stdout io.Writer, result setupInstallResult) error {
	if len(result.created) > 0 {
		if _, err := fmt.Fprintln(stdout, "Installed OpenCode skills and commands."); err != nil {
			return err
		}
	}

	if len(result.unchanged) > 0 {
		if _, err := fmt.Fprintln(stdout, "Existing OpenCode skills and commands were left unchanged."); err != nil {
			return err
		}
	}

	return nil
}

func setupTargetFor(name string) (setupTarget, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "codex":
		return setupTarget{
			name:    "Codex",
			prepare: prepareCodexMarketplace,
			commands: [][]string{
				{"codex", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"codex", "plugin", "add", "callee@callee"},
			},
			role: codexReviewerRole,
		}, nil
	case "claude":
		return setupTarget{
			name: "Claude Code",
			commands: [][]string{
				{"claude", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"claude", "plugin", "install", "callee@callee", "--scope", "project"},
			},
			role: claudeReviewerRole,
		}, nil
	case "grok":
		return setupTarget{
			name: "Grok Build",
			commands: [][]string{
				{"grok", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"grok", "plugin", "install", "callee@callee", "--trust"},
			},
			role: grokReviewerRole,
		}, nil
	case "copilot":
		return setupTarget{
			name: "Copilot CLI",
			commands: [][]string{
				{"copilot", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"copilot", "plugin", "install", "callee@callee"},
			},
			role: copilotReviewerRole,
		}, nil
	case "opencode":
		return setupTarget{
			name:    "OpenCode",
			role:    openCodeReviewerRole,
			install: writeOpenCodeIntegration,
		}, nil
	default:
		return setupTarget{}, fmt.Errorf("unsupported setup target %q (want codex, claude, grok, copilot, or opencode)", name)
	}
}

func prepareCodexMarketplace(ctx context.Context, stderr io.Writer) error {
	var diagnostics bytes.Buffer

	err := runSetupCommand(
		ctx,
		io.Discard,
		&diagnostics,
		"codex",
		"plugin",
		"marketplace",
		"remove",
		"callee",
	)
	if err != nil && strings.Contains(diagnostics.String(), "not configured or installed") {
		return nil
	}

	if _, writeErr := io.Copy(stderr, &diagnostics); writeErr != nil {
		return fmt.Errorf("write Codex marketplace diagnostics: %w", writeErr)
	}

	if err != nil {
		return fmt.Errorf("remove existing Codex marketplace: %w", err)
	}

	return nil
}

func runSetupCommandDefault(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout = stdout
	command.Stderr = stderr

	if err := command.Run(); err != nil {
		return fmt.Errorf("run %s: %w", strings.Join(append([]string{name}, args...), " "), err)
	}

	return nil
}

func writeReviewerRole(content string, force bool) (bool, error) {
	path := filepath.FromSlash(reviewerRolePath)
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		} else if !os.IsNotExist(err) {
			return false, fmt.Errorf("check existing reviewer role: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), reviewerDirMode); err != nil {
		return false, fmt.Errorf("create reviewer role directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(content), reviewerFileMode); err != nil {
		return false, fmt.Errorf("write reviewer role: %w", err)
	}

	return true, nil
}

const codexReviewerRole = `---
api: callee.metalagman.dev
kind: role
description: Reviews code changes for correctness and regressions.
provider:
  type: codex
  model: gpt-5-codex
  reasoning: high
  mode: review
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
`

const claudeReviewerRole = `---
api: callee.metalagman.dev
kind: role
description: Reviews code changes for correctness and regressions.
provider:
  type: claude
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
`

const grokReviewerRole = `---
api: callee.metalagman.dev
kind: role
description: Reviews code changes for correctness and regressions.
provider:
  type: grok
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
`

const copilotReviewerRole = `---
api: callee.metalagman.dev
kind: role
description: Reviews code changes for correctness and regressions.
provider:
  type: copilot
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
`

const openCodeReviewerRole = `---
api: callee.metalagman.dev
kind: role
description: Reviews code changes for correctness and regressions.
provider:
  type: opencode
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
`
