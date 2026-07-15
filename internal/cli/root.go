// Package cli implements the Callee command-line surface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const (
	Version              = "0.6.0"
	roleListTabPadding   = 2
	defaultPromptTimeout = 10 * time.Minute
)

var (
	runDoctor = doctor.Run
	runRole   = runtime.RunOnce
)

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

		if jsonRequested(args) {
			_ = logging.WriteJSONError(stderr, err)
		} else {
			_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		}

		return 1
	}

	return 0
}

// NewRootCommand creates the Callee Cobra command tree.
func NewRootCommand() *cobra.Command {
	var (
		rolesDir     string
		debug, trace bool
	)

	root := &cobra.Command{Use: "callee", Short: "Turn Markdown roles into callable ACP agents.", Version: Version, SilenceErrors: true, SilenceUsage: true, PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
		jsonOutput := commandJSONOutput(cmd)
		if err := logging.Init(logging.WithLevel(loggingLevel(cmd.Name(), debug, trace)), logging.WithJSON(jsonOutput), logging.WithWriter(cmd.ErrOrStderr())); err != nil {
			return err
		}

		log.Debug().Str("command", cmd.Name()).Msg("starting Callee command")

		return nil
	}, Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		return errors.New("a command is required")
	}}
	root.PersistentFlags().StringVar(&rolesDir, "roles-dir", "", "load roles only from this directory")
	root.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&trace, "trace", false, "enable trace logging (overrides --debug)")
	root.AddCommand(promptCommand(&rolesDir))
	root.AddCommand(listCommand(&rolesDir))
	root.AddCommand(doctorCommand(&rolesDir))
	root.AddCommand(setupCommand())

	return root
}

func commandJSONOutput(cmd *cobra.Command) bool {
	jsonOutput, err := cmd.Flags().GetBool("json")

	return err == nil && jsonOutput
}

func jsonRequested(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "--json=true" {
			return true
		}
	}

	return false
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

type roleListOutput struct {
	Roles []roleListItem `json:"roles"`
}

type roleListItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

func listCommand(rolesDir *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{Use: "list", Short: "List configured Callee roles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		reg, err := load(*rolesDir)
		if err != nil {
			return err
		}

		roles := reg.Roles()
		if jsonOutput {
			output := roleListOutput{Roles: make([]roleListItem, 0, len(roles))}
			for _, r := range roles {
				output.Roles = append(output.Roles, roleListItem{ID: r.ID, Description: r.Metadata.Description})
			}

			return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
		}

		out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, roleListTabPadding, ' ', 0)
		if _, err := fmt.Fprintln(out, "ID\tDESCRIPTION"); err != nil {
			return err
		}

		for _, r := range roles {
			if _, err := fmt.Fprintf(out, "%s\t%s\n", r.ID, strings.TrimSpace(r.Metadata.Description)); err != nil {
				return err
			}
		}

		return out.Flush()
	}}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output roles as JSON and diagnostics as JSON Lines")

	return cmd
}

type promptOutput struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
	Resumed  bool   `json:"resumed"`
}

func promptCommand(rolesDir *string) *cobra.Command {
	var roleID, message, threadID string

	var timeout time.Duration

	var jsonOutput bool

	cmd := &cobra.Command{Use: "prompt", Short: "Prompt a configured Callee role", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if timeout <= 0 {
			return errors.New("timeout must be greater than zero")
		}

		reg, err := load(*rolesDir)
		if err != nil {
			return err
		}

		r, err := reg.Get(roleID)
		if err != nil {
			return err
		}

		rendered, err := r.Render(message)
		if err != nil {
			return err
		}

		turnCtx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()

		result, err := runRole(turnCtx, runtime.NormaFactory{Stderr: cmd.ErrOrStderr(), JSONDiagnostics: jsonOutput}, r, rendered, threadID)
		if err != nil {
			return err
		}

		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(promptOutput{
				ThreadID: result.ThreadID,
				Content:  result.Content,
				Resumed:  threadID != "" && result.ThreadID == threadID,
			})
		}

		_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Content)

		return err
	}}
	cmd.Flags().StringVar(&roleID, "role", "", "role ID")
	cmd.Flags().StringVar(&message, "message", "", "message to send to the role")
	cmd.Flags().StringVar(&threadID, "thread-id", "", "ACP session ID to resume")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultPromptTimeout, "maximum time for ACP startup and the prompt")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the response as JSON and diagnostics as JSON Lines")
	_ = cmd.MarkFlagRequired("role")
	_ = cmd.MarkFlagRequired("message")

	return cmd
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

		return runDoctor(cmd.Context(), reg.Roles(), runtime.NormaFactory{Stderr: cmd.ErrOrStderr()}, timeout, cmd.OutOrStdout())
	}}
	cmd.Flags().DurationVar(&timeout, "timeout", time.Minute, "maximum initialization time for each role")

	return cmd
}
