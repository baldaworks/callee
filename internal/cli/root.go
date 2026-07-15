// Package cli implements the Callee command-line surface.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const (
	Version              = "0.7.0"
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

	root := &cobra.Command{Use: "callee", Short: "Run provider-aware subagent roles described in Markdown.", Version: Version, SilenceErrors: true, SilenceUsage: true, PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
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
	root.AddCommand(roleCommand(&rolesDir))
	root.AddCommand(doctorCommand(&rolesDir))
	root.AddCommand(promptKitCommand())
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

type promptOutput struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
	Resumed  bool   `json:"resumed"`
}

func promptCommand(rolesDir *string) *cobra.Command {
	var (
		roleID, message, messageFile, threadID string
		params, paramFiles                     []string
		timeout                                time.Duration
		jsonOutput                             bool
	)

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

		input, err := promptInput(message, messageFile, cmd.Flags().Changed("message-file"))
		if err != nil {
			return err
		}

		outbound := input

		if threadID != "" {
			if cmd.Flags().Changed("param") || cmd.Flags().Changed("param-file") {
				return errors.New("role parameters can only be supplied when starting a thread")
			}
		} else {
			values, valuesErr := runtimeParameterValues(params, paramFiles)
			if valuesErr != nil {
				return valuesErr
			}

			outbound, err = r.Render(input, values)
			if err != nil {
				return err
			}
		}

		turnCtx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()

		result, err := runRole(turnCtx, runtime.NormaFactory{Stderr: cmd.ErrOrStderr(), JSONDiagnostics: jsonOutput}, r, outbound, threadID)
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
	cmd.Flags().StringVar(&messageFile, "message-file", "", "read the exact message from this file")
	cmd.Flags().StringArrayVar(&params, "param", nil, "role parameter as key=value; repeatable on a new thread")
	cmd.Flags().StringArrayVar(&paramFiles, "param-file", nil, "role parameter as key=path; repeatable on a new thread")
	cmd.Flags().StringVar(&threadID, "thread-id", "", "opaque thread handle to resume")
	cmd.Flags().DurationVar(&timeout, "timeout", defaultPromptTimeout, "maximum time for runtime startup and the prompt")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the response as JSON and diagnostics as JSON Lines")
	cmd.MarkFlagsMutuallyExclusive("message", "message-file")
	cmd.MarkFlagsOneRequired("message", "message-file")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func promptInput(message, path string, useFile bool) (string, error) {
	if !useFile {
		return message, nil
	}

	if path == "" {
		return "", errors.New("--message-file requires a non-empty file path")
	}

	if path == "-" {
		return "", errors.New("--message-file requires a file path, not stdin")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read message file %q: %w", path, err)
	}

	return string(content), nil
}

func runtimeParameterValues(raw, files []string) (map[string]string, error) {
	return parameterValues(raw, files, "role parameter", "is specified more than once")
}

func isExpectedCancellation(ctx context.Context, err error) bool {
	return errors.Is(ctx.Err(), context.Canceled) && errors.Is(err, context.Canceled)
}

func doctorCommand(rolesDir *string) *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{Use: "doctor", Short: "Check configured role runtimes", Long: "Load every configured role, initialize its runtime, and close it without sending a model prompt.", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
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
