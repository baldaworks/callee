// Package cli implements the Callee command-line surface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const Version = "0.1.0"

var runDoctor = doctor.Run

// Run runs Callee and returns its process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(ctx)

	if err := cmd.Execute(); err != nil {
		if isExpectedCancellation(ctx, err) {
			return 0
		}

		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)

		return 1
	}

	return 0
}

// NewRootCommand creates the Callee Cobra command tree.
func NewRootCommand() *cobra.Command {
	var (
		rolesDir, roleID, prompt string
		debug, trace             bool
	)

	root := &cobra.Command{Use: "callee", Short: "Turn Markdown roles into callable ACP agents.", Version: Version, SilenceErrors: true, SilenceUsage: true, PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		if err := logging.Init(logging.WithLevel(loggingLevel(cmd.Name(), debug, trace))); err != nil {
			return err
		}

		log.Debug().Str("command", cmd.Name()).Msg("starting Callee command")

		return nil
	}, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := load(rolesDir)
		if err != nil {
			return err
		}

		log.Debug().Strs("roles", reg.IDs()).Msg("loaded role registry")

		r, err := reg.Get(roleID)
		if err != nil {
			return err
		}

		log.Debug().Str("role", r.ID).Str("type", r.Metadata.Type).Msg("resolved one-shot role")

		rendered, err := r.Render(prompt)
		if err != nil {
			return err
		}

		log.Debug().Str("role", r.ID).Msg("rendered one-shot role prompt")

		manager := runtime.NewManager(runtime.NormaFactory{})

		content, err := manager.RunOnce(cmd.Context(), r, rendered)
		if err != nil {
			return err
		}

		log.Debug().Str("role", r.ID).Int("content_length", len(content)).Msg("one-shot role completed")
		_, err = fmt.Fprintln(cmd.OutOrStdout(), content)

		return err
	}}
	root.PersistentFlags().StringVar(&rolesDir, "roles-dir", "", "load roles only from this directory")
	root.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&trace, "trace", false, "enable trace logging (overrides --debug)")
	root.Flags().StringVar(&roleID, "role", "", "role ID")
	root.Flags().StringVar(&prompt, "prompt", "", "initial prompt")
	_ = root.MarkFlagRequired("role")
	_ = root.MarkFlagRequired("prompt")
	root.AddCommand(mcpServerCommand(&rolesDir))
	root.AddCommand(doctorCommand(&rolesDir))

	return root
}

func loggingLevel(commandName string, debug, trace bool) string {
	level := logging.LevelInfo
	if commandName == "doctor" {
		level = logging.LevelError
	}

	if debug {
		level = logging.LevelDebug
	}

	if trace {
		level = logging.LevelTrace
	}

	return level
}

func load(rolesDir string) (*registry.Registry, error) {
	return registry.Load(registry.LoadOptions{RolesDir: rolesDir})
}

func mcpServerCommand(rolesDir *string) *cobra.Command {
	return &cobra.Command{Use: "mcp-server", Short: "Serve Callee over MCP stdio", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := load(*rolesDir)
		if err != nil {
			return err
		}

		log.Debug().Strs("roles", reg.IDs()).Msg("loaded role registry for MCP server")
		log.Debug().Msg("starting MCP stdio server")

		return runMCPServer(cmd.Context(), reg, Version)
	}}
}

func isExpectedCancellation(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) && errors.Is(err, context.Canceled)
}

func doctorCommand(rolesDir *string) *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{Use: "doctor", Short: "Check configured role runtimes", Long: "Load every configured role, initialize its ACP runtime, and close it without sending a model prompt.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := load(*rolesDir)
		if err != nil {
			return err
		}

		log.Debug().Strs("roles", reg.IDs()).Dur("timeout", timeout).Msg("loaded role registry for doctor")

		return runDoctor(cmd.Context(), reg.Roles(), runtime.NormaFactory{}, timeout, cmd.OutOrStdout())
	}}
	cmd.Flags().DurationVar(&timeout, "timeout", time.Minute, "maximum initialization time for each role")

	return cmd
}
