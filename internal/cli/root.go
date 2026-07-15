// Package cli implements the Callee command-line surface.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/baldaworks/callee/internal/doctor"
	"github.com/baldaworks/callee/internal/logging"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/role"
	"github.com/baldaworks/callee/internal/runtime"
	acp "github.com/coder/acp-go-sdk"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const (
	Version                  = "0.9.0"
	defaultPromptTimeout     = 15 * time.Minute
	defaultPromptTimeoutText = "15m"
)

var (
	runDoctor = doctor.Run
	runRole   = runtime.RunOnce
	openRole  = runtime.Open
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

type promptOptions struct {
	roleID      string
	message     string
	messageFile string
	threadID    string
	params      []string
	paramFiles  []string
	timeout     time.Duration
	jsonOutput  bool
}

func promptCommand(rolesDir *string) *cobra.Command {
	opts := &promptOptions{}
	cmd := &cobra.Command{
		Use:   "prompt",
		Short: "Prompt a configured Callee role",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runPromptCommand(cmd, *rolesDir, opts)
		},
	}
	cmd.Flags().StringVar(&opts.roleID, "role", "", "role ID")
	cmd.Flags().StringVar(&opts.message, "message", "", "message to send to the role")
	cmd.Flags().StringVar(&opts.messageFile, "message-file", "", "read the exact message from this file")
	cmd.Flags().StringArrayVar(&opts.params, "param", nil, "role parameter as key=value; repeatable on a new thread")
	cmd.Flags().StringArrayVar(&opts.paramFiles, "param-file", nil, "role parameter as key=path; repeatable on a new thread")
	cmd.Flags().StringVar(&opts.threadID, "thread-id", "", "opaque thread handle to resume")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", defaultPromptTimeout, "maximum time for runtime startup and the prompt")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "output the response as JSON and diagnostics as JSON Lines")
	cmd.MarkFlagsMutuallyExclusive("message", "message-file")
	cmd.MarkFlagsOneRequired("message", "message-file")
	_ = cmd.MarkFlagRequired("role")

	return cmd
}

func runPromptCommand(cmd *cobra.Command, rolesDir string, opts *promptOptions) error {
	if cmd.Flags().Changed("timeout") && opts.timeout <= 0 {
		return errors.New("timeout must be greater than zero")
	}

	reg, err := load(rolesDir)
	if err != nil {
		return err
	}

	r, err := reg.Get(opts.roleID)
	if err != nil {
		return err
	}

	if !cmd.Flags().Changed("timeout") {
		opts.timeout = r.PromptTimeout(defaultPromptTimeout)
	}

	if r.Metadata.Provider.REPL && opts.jsonOutput {
		return errors.New("--json is not supported for roles with provider.repl enabled")
	}

	input, err := promptInput(opts.message, opts.messageFile, cmd.Flags().Changed("message-file"))
	if err != nil {
		return err
	}

	outbound, err := renderPromptInput(cmd, r, input, opts)
	if err != nil {
		return err
	}

	if r.Metadata.Provider.REPL {
		return runPromptREPL(cmd, r, outbound, opts.threadID, opts.timeout)
	}

	return runOneShotPrompt(cmd, r, outbound, opts)
}

func renderPromptInput(cmd *cobra.Command, r role.Role, input string, opts *promptOptions) (string, error) {
	if opts.threadID != "" {
		if cmd.Flags().Changed("param") || cmd.Flags().Changed("param-file") {
			return "", errors.New("role parameters can only be supplied when starting a thread")
		}

		return input, nil
	}

	values, err := runtimeParameterValues(opts.params, opts.paramFiles)
	if err != nil {
		return "", err
	}

	return r.Render(input, values)
}

func runOneShotPrompt(cmd *cobra.Command, r role.Role, outbound string, opts *promptOptions) error {
	turnCtx, cancel := context.WithTimeout(cmd.Context(), opts.timeout)
	defer cancel()

	factory := runtime.NormaFactory{
		Stderr:          cmd.ErrOrStderr(),
		JSONDiagnostics: opts.jsonOutput,
	}

	result, err := runRole(turnCtx, factory, r, outbound, opts.threadID)
	if err != nil {
		return err
	}

	if opts.jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(promptOutput{
			ThreadID: result.ThreadID,
			Content:  result.Content,
			Resumed:  opts.threadID != "" && result.ThreadID == opts.threadID,
		})
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Content)

	return err
}

func runPromptREPL(cmd *cobra.Command, r role.Role, initialPrompt, threadID string, timeout time.Duration) (resultErr error) {
	reader := bufio.NewReader(cmd.InOrStdin())
	factory := runtime.NormaFactory{
		Stderr:            cmd.ErrOrStderr(),
		PermissionHandler: permissionHandler(reader, cmd.OutOrStdout()),
	}

	lifetimeCtx, lifetimeCancel := context.WithCancel(cmd.Context())
	defer lifetimeCancel()

	conversation, err := openREPLRole(lifetimeCtx, factory, r, timeout)
	if err != nil {
		return err
	}

	defer func() {
		if closeErr := conversation.Close(); closeErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close role %q runtime: %w", r.ID, closeErr))
		}
	}()

	if err := runREPLTurn(cmd, conversation, r, initialPrompt, threadID, timeout); err != nil {
		return err
	}

	for {
		if _, err := fmt.Fprint(cmd.OutOrStdout(), "> "); err != nil {
			return err
		}

		line, readErr := readLine(cmd.Context(), reader)
		input := strings.TrimSpace(line)

		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read REPL input: %w", readErr)
		}

		if input == "exit" || input == "quit" {
			return nil
		}

		if input != "" {
			if err := runREPLTurn(cmd, conversation, r, input, "", timeout); err != nil {
				return err
			}
		}

		if errors.Is(readErr, io.EOF) {
			return nil
		}
	}
}

func openREPLRole(ctx context.Context, factory runtime.Factory, r role.Role, timeout time.Duration) (runtime.Conversation, error) {
	type result struct {
		conversation runtime.Conversation
		err          error
	}

	opened := make(chan result, 1)

	go func() {
		conversation, err := openRole(ctx, factory, r)
		opened <- result{conversation: conversation, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case value := <-opened:
		return value.conversation, value.err
	case <-timer.C:
		go func() {
			value := <-opened
			if value.conversation != nil {
				_ = value.conversation.Close()
			}
		}()

		return nil, fmt.Errorf("start role %q runtime: %w", r.ID, context.DeadlineExceeded)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func runREPLTurn(cmd *cobra.Command, conversation runtime.Conversation, r role.Role, prompt, threadID string, timeout time.Duration) error {
	turnCtx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	result, err := conversation.Run(turnCtx, r, prompt, threadID)
	if err != nil {
		return fmt.Errorf("role %q: %w", r.ID, err)
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Content)

	return err
}

func permissionHandler(reader *bufio.Reader, output io.Writer) func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return func(ctx context.Context, request acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
		if len(request.Options) == 0 {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
		}

		if _, err := fmt.Fprintln(output, "Permission required:"); err != nil {
			return acp.RequestPermissionResponse{}, err
		}

		for index, option := range request.Options {
			if _, err := fmt.Fprintf(output, "%d) %s [%s]\n", index+1, option.Name, option.Kind); err != nil {
				return acp.RequestPermissionResponse{}, err
			}
		}

		if _, err := fmt.Fprint(output, "Select: "); err != nil {
			return acp.RequestPermissionResponse{}, err
		}

		line, err := readLine(ctx, reader)
		if err != nil && !errors.Is(err, io.EOF) {
			return acp.RequestPermissionResponse{}, fmt.Errorf("read permission selection: %w", err)
		}

		if errors.Is(err, io.EOF) {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
		}

		selection, parseErr := strconv.Atoi(strings.TrimSpace(line))
		if parseErr != nil || selection < 1 || selection > len(request.Options) {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
		}

		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(request.Options[selection-1].OptionId),
		}, nil
	}
}

func readLine(ctx context.Context, reader *bufio.Reader) (string, error) {
	type result struct {
		line string
		err  error
	}

	read := make(chan result, 1)

	go func() {
		line, err := reader.ReadString('\n')
		read <- result{line: line, err: err}
	}()

	select {
	case value := <-read:
		return value.line, value.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
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
