package cli

import (
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
	commands [][]string
	role     string
}

var runSetupCommand = runSetupCommandDefault

func setupCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "setup <codex|claude>",
		Short: "Install a host plugin and create a reviewer role",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := setupTargetFor(args[0])
			if err != nil {
				return err
			}

			for _, command := range target.commands {
				if err := runSetupCommand(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), command[0], command[1:]...); err != nil {
					return err
				}
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
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing reviewer role")

	return cmd
}

func setupTargetFor(name string) (setupTarget, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "codex":
		return setupTarget{
			name: "Codex",
			commands: [][]string{
				{"codex", "plugin", "marketplace", "add", "baldaworks/callee", "--sparse", ".agents/plugins"},
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
	default:
		return setupTarget{}, fmt.Errorf("unsupported setup target %q (want codex or claude)", name)
	}
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
description: Reviews code changes for correctness and regressions.
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
description: Reviews code changes for correctness and regressions.
type: claude
---

You are an independent code reviewer.

Review the following task:

{{ prompt }}

Do not modify files. Return concrete, evidence-backed findings.
`
