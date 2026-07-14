// Package cli implements the Callee command-line surface.
package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/server"
	"github.com/spf13/cobra"
)

const Version = "0.1.0"

// Run runs Callee and returns its process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(ctx)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		return 1
	}
	return 0
}

// NewRootCommand creates the Callee Cobra command tree.
func NewRootCommand() *cobra.Command {
	var rolesDir string
	root := &cobra.Command{Use: "callee", Short: "Turn Markdown roles into callable ACP agents.", Version: Version, SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().StringVar(&rolesDir, "roles-dir", "", "load roles only from this directory")
	root.AddCommand(execCommand(&rolesDir), mcpServerCommand(&rolesDir))
	return root
}

func load(rolesDir string) (*registry.Registry, error) {
	return registry.Load(registry.LoadOptions{RolesDir: rolesDir})
}

func execCommand(rolesDir *string) *cobra.Command {
	var roleID, prompt string
	cmd := &cobra.Command{Use: "exec", Short: "Execute a role once", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := load(*rolesDir)
		if err != nil {
			return err
		}
		r, err := reg.Get(roleID)
		if err != nil {
			return err
		}
		rendered, err := r.Render(prompt)
		if err != nil {
			return err
		}
		manager := runtime.NewManager(runtime.NormaFactory{})
		defer manager.Close()
		_, content, err := manager.Start(cmd.Context(), r, rendered)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), content)
		return err
	}}
	cmd.Flags().StringVar(&roleID, "role", "", "role ID")
	cmd.Flags().StringVar(&prompt, "prompt", "", "initial prompt")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("prompt")
	return cmd
}

func mcpServerCommand(rolesDir *string) *cobra.Command {
	return &cobra.Command{Use: "mcp-server", Short: "Serve Callee over MCP stdio", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := load(*rolesDir)
		if err != nil {
			return err
		}
		manager := runtime.NewManager(runtime.NormaFactory{})
		defer manager.Close()
		return server.New(reg, manager).RunStdio(cmd.Context(), Version)
	}}
}
