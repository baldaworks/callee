package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

const (
	setupDirMode  = 0o755
	setupFileMode = 0o644
)

type setupTarget struct {
	name             string
	prepare          func(context.Context, io.Writer) error
	commands         [][]string
	providerType     string
	install          func(bool) (setupInstallResult, error)
	installedMessage string
	unchangedMessage string
}

type setupInstallResult struct {
	created   []string
	unchanged []string
}

var runSetupCommand = runSetupCommandDefault

func setupCommand() *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "setup <codex|claude|grok|copilot|opencode|cursor>",
		Short: "Install a host integration and starter agents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target, err := setupTargetFor(args[0])
			if err != nil {
				return err
			}

			if err := installSetupTarget(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), target, force); err != nil {
				return err
			}

			result, err := writeStarterAgents(target.providerType, force)
			if err != nil {
				return err
			}

			return reportStarterAgentInstall(cmd.OutOrStdout(), target.name, result)
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

	return reportLocalIntegrationInstall(stdout, target, result)
}

func reportLocalIntegrationInstall(stdout io.Writer, target setupTarget, result setupInstallResult) error {
	if len(result.created) > 0 {
		if _, err := fmt.Fprintln(stdout, target.installedMessage); err != nil {
			return err
		}
	}

	if len(result.unchanged) > 0 {
		if _, err := fmt.Fprintln(stdout, target.unchangedMessage); err != nil {
			return err
		}
	}

	return nil
}

func setupTargetFor(name string) (setupTarget, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "codex":
		return setupTarget{
			name:         "Codex",
			providerType: "codex",
			prepare:      prepareCodexMarketplace,
			commands: [][]string{
				{"codex", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"codex", "plugin", "add", "callee@callee"},
			},
		}, nil
	case "claude":
		return setupTarget{
			name:         "Claude Code",
			providerType: "claude",
			commands: [][]string{
				{"claude", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"claude", "plugin", "install", "callee@callee", "--scope", "project"},
			},
		}, nil
	case "grok":
		return setupTarget{
			name:         "Grok Build",
			providerType: "grok",
			commands: [][]string{
				{"grok", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"grok", "plugin", "install", "callee@callee", "--trust"},
			},
		}, nil
	case "copilot":
		return setupTarget{
			name:         "Copilot CLI",
			providerType: "copilot",
			commands: [][]string{
				{"copilot", "plugin", "marketplace", "add", "baldaworks/callee"},
				{"copilot", "plugin", "install", "callee@callee"},
			},
		}, nil
	case "opencode":
		return setupTarget{
			name:             "OpenCode",
			providerType:     "opencode",
			install:          writeOpenCodeIntegration,
			installedMessage: "Installed OpenCode skills and commands.",
			unchangedMessage: "Existing OpenCode skills and commands were left unchanged.",
		}, nil
	case "cursor":
		return setupTarget{
			name:             "Cursor",
			providerType:     "cursor",
			install:          writeCursorIntegration,
			installedMessage: "Installed Cursor skills.",
			unchangedMessage: "Existing Cursor skills were left unchanged.",
		}, nil
	default:
		return setupTarget{}, fmt.Errorf("unsupported setup target %q (want codex, claude, grok, copilot, opencode, or cursor)", name)
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
