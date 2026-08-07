// Package cli implements the Callee command-line surface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const (
	Version             = "0.20.1"
	exitError           = 1
	exitInterrupt       = 130
	exitTerminate       = 143
	permissionsFlagName = "permissions"
)

var (
	// ErrInterrupt marks cancellation caused by SIGINT.
	ErrInterrupt = errors.New("interrupted")
	// ErrTerminate marks cancellation caused by SIGTERM.
	ErrTerminate = errors.New("terminated")
)

var (
	runAgentDoctor = doctor.RunAgents
	openTerminal   = func() (io.ReadWriteCloser, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}
)

// Run runs Callee and returns its process exit code.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	cmd := NewRootCommand()
	cmd.SetArgs(args)
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetContext(ctx)

	err := cmd.Execute()
	if err == nil {
		return 0
	}

	if code := signalExitCode(ctx); code != 0 {
		return code
	}

	writeCommandError(args, stderr, err)

	return exitError
}

func signalExitCode(ctx context.Context) int {
	switch {
	case errors.Is(context.Cause(ctx), ErrInterrupt):
		return exitInterrupt
	case errors.Is(context.Cause(ctx), ErrTerminate):
		return exitTerminate
	default:
		return 0
	}
}

func writeCommandError(args []string, stderr io.Writer, err error) {
	if jsonRequested(args) {
		_ = logging.WriteJSONError(stderr, err)

		return
	}

	_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
}

// NewRootCommand creates the Callee Cobra command tree.
func NewRootCommand() *cobra.Command {
	var debug, trace bool

	var agentRoot string

	root := &cobra.Command{
		Use:           "callee",
		Short:         "Run provider-aware agents and workflows defined in Markdown or YAML.",
		Version:       Version,
		SilenceErrors: true,
		SilenceUsage:  true,
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateCommandPolicyFlags(cmd); err != nil {
				return err
			}

			jsonOutput := commandJSONOutput(cmd)
			if err := logging.Init(logging.WithLevel(loggingLevel(cmd.Name(), debug, trace)), logging.WithJSON(jsonOutput), logging.WithWriter(cmd.ErrOrStderr())); err != nil {
				return err
			}

			log.Debug().Str("command", cmd.Name()).Msg("starting Callee command")

			return nil
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().BoolVar(&debug, "debug", false, "enable debug logging")
	root.PersistentFlags().BoolVar(&trace, "trace", false, "enable trace logging (overrides --debug)")
	root.PersistentFlags().StringVar(&agentRoot, agentRootFlagName, "", "use this directory as the only Callee agent root")
	root.PersistentFlags().String(permissionsFlagName, "", "override every Role's ACP permission policy for agent run or view (ask, allow, or deny)")
	root.AddCommand(agentCommand())
	root.AddCommand(bridgeCommand())
	root.AddCommand(doctorCommand())
	root.AddCommand(promptKitCommand())
	root.AddCommand(setupCommand())

	return root
}

func commandPermissionOverride(cmd *cobra.Command) (*agent.PermissionMode, error) {
	if cmd.Flags().Lookup(permissionsFlagName) == nil || !cmd.Flags().Changed(permissionsFlagName) {
		return nil, nil
	}

	value, err := cmd.Flags().GetString(permissionsFlagName)
	if err != nil {
		return nil, err
	}

	mode := agent.PermissionMode(value)
	switch mode {
	case agent.PermissionModeAsk, agent.PermissionModeAllow, agent.PermissionModeDeny:
		return &mode, nil
	default:
		return nil, fmt.Errorf("--%s must be ask, allow, or deny", permissionsFlagName)
	}
}

func validateCommandPolicyFlags(cmd *cobra.Command) error {
	permissions, err := commandPermissionOverride(cmd)
	if err != nil || permissions == nil || *permissions != agent.PermissionModeAsk {
		return err
	}

	if cmd.Flags().Lookup("interactive") == nil || !cmd.Flags().Changed("interactive") {
		return nil
	}

	interactive, err := cmd.Flags().GetBool("interactive")
	if err != nil {
		return err
	}

	if !interactive {
		return fmt.Errorf("--interactive=false is incompatible with --permissions=ask")
	}

	return nil
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

func doctorCommand() *cobra.Command {
	var (
		timeout time.Duration
		graph   string
	)

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Validate agents and check configured Role runtimes",
		Long:  "Validate the complete agent graph, then initialize every configured Role runtime without sending a model prompt.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configured, err := loadAgentRegistry(cmd)
			if err != nil {
				return err
			}

			if graph != "" {
				return doctor.WriteGraph(cmd.OutOrStdout(), configured, graph)
			}

			log.Debug().Strs("agents", configured.IDs()).Dur("timeout", timeout).Msg("loaded agent registry for doctor")

			return runAgentDoctor(cmd.Context(), configured.Agents(), runtime.NormaFactory{Stderr: cmd.ErrOrStderr()}, timeout, cmd.OutOrStdout())
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", time.Minute, "maximum initialization time for each Role runtime")
	cmd.Flags().StringVar(&graph, "graph", "", "render the static agent graph as text, mermaid, or dot")

	return cmd
}
