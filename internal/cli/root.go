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
	"sort"
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
	Version                = "0.10.0"
	defaultRoleTimeout     = 15 * time.Minute
	defaultRoleTimeoutText = "15m"
)

var (
	runDoctor    = doctor.Run
	openRole     = runtime.Open
	openTerminal = func() (io.ReadWriteCloser, error) {
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
	root.AddCommand(agentCommand(&rolesDir))
	root.AddCommand(execCommand(&rolesDir))
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

type executionOutput struct {
	ThreadID string `json:"threadId"`
	Content  string `json:"content"`
	Resumed  bool   `json:"resumed"`
}

type executionOptions struct {
	roleID      string
	message     string
	messageFile string
	threadID    string
	params      []string
	paramFiles  []string
	timeout     time.Duration
	jsonOutput  bool
}

func agentCommand(rolesDir *string) *cobra.Command {
	opts := &executionOptions{}
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run a configured Callee role for a human",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentCommand(cmd, *rolesDir, opts)
		},
	}
	addExecutionFlags(cmd, opts)
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "unsupported; use callee exec --json")
	_ = cmd.Flags().MarkHidden("json")

	return cmd
}

func execCommand(rolesDir *string) *cobra.Command {
	opts := &executionOptions{}
	cmd := &cobra.Command{
		Use:   "exec",
		Short: "Execute one turn of a configured Callee role",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runExecCommand(cmd, *rolesDir, opts)
		},
	}
	addExecutionFlags(cmd, opts)
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "output the response as JSON and diagnostics as JSON Lines")

	return cmd
}

func addExecutionFlags(cmd *cobra.Command, opts *executionOptions) {
	cmd.Flags().StringVar(&opts.roleID, "role", "", "role ID")
	cmd.Flags().StringVar(&opts.message, "message", "", "message to send to the role")
	cmd.Flags().StringVar(&opts.messageFile, "message-file", "", "read the exact message from this file")
	cmd.Flags().StringArrayVar(&opts.params, "param", nil, "role parameter as key=value; repeatable on a new thread")
	cmd.Flags().StringArrayVar(&opts.paramFiles, "param-file", nil, "role parameter as key=path; repeatable on a new thread")
	cmd.Flags().StringVar(&opts.threadID, "thread-id", "", "opaque thread handle to resume")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", defaultRoleTimeout, "maximum time for runtime startup and one role turn")
	cmd.MarkFlagsMutuallyExclusive("message", "message-file")
	cmd.MarkFlagsOneRequired("message", "message-file")
	_ = cmd.MarkFlagRequired("role")
}

func runExecCommand(cmd *cobra.Command, rolesDir string, opts *executionOptions) error {
	r, input, err := executionInput(cmd, rolesDir, opts)
	if err != nil {
		return err
	}

	if r.Metadata.REPL {
		return fmt.Errorf("role %q requires interactive execution; use callee agent", r.ID)
	}

	outbound, err := renderExecutionInput(cmd, r, input, opts)
	if err != nil {
		return err
	}

	return runOneShotExecution(cmd, r, outbound, opts)
}

func runAgentCommand(cmd *cobra.Command, rolesDir string, opts *executionOptions) error {
	if opts.jsonOutput {
		return errors.New("--json is only supported by callee exec")
	}

	r, input, err := executionInput(cmd, rolesDir, opts)
	if err != nil {
		return err
	}

	if opts.threadID != "" {
		outbound, renderErr := renderExecutionInput(cmd, r, input, opts)
		if renderErr != nil {
			return renderErr
		}

		if !r.Metadata.REPL {
			return runOneShotExecution(cmd, r, outbound, opts)
		}

		terminal, reader, terminalErr := agentTerminal()
		if terminalErr != nil {
			return terminalErr
		}
		defer terminal.Close()

		return runAgentREPL(cmd, r, outbound, opts.threadID, opts.timeout, terminal, reader)
	}

	values, err := runtimeParameterValues(opts.params, opts.paramFiles)
	if err != nil {
		return err
	}

	missing, err := missingRuntimeParameters(r, values)
	if err != nil {
		return err
	}

	if len(missing) == 0 && !r.Metadata.REPL {
		outbound, renderErr := r.Render(input, values)
		if renderErr != nil {
			return renderErr
		}

		return runOneShotExecution(cmd, r, outbound, opts)
	}

	terminal, reader, err := agentTerminal()
	if err != nil {
		return err
	}
	defer terminal.Close()

	if err := collectRuntimeParameters(cmd.Context(), terminal, reader, r, values, missing); err != nil {
		return err
	}

	outbound, err := r.Render(input, values)
	if err != nil {
		return err
	}

	if !r.Metadata.REPL {
		return runOneShotExecution(cmd, r, outbound, opts)
	}

	return runAgentREPL(cmd, r, outbound, "", opts.timeout, terminal, reader)
}

func executionInput(cmd *cobra.Command, rolesDir string, opts *executionOptions) (role.Role, string, error) {
	if cmd.Flags().Changed("timeout") && opts.timeout <= 0 {
		return role.Role{}, "", errors.New("timeout must be greater than zero")
	}

	reg, err := load(rolesDir)
	if err != nil {
		return role.Role{}, "", err
	}

	r, err := reg.Get(opts.roleID)
	if err != nil {
		return role.Role{}, "", err
	}

	if !cmd.Flags().Changed("timeout") {
		opts.timeout = r.PromptTimeout(defaultRoleTimeout)
	}

	input, err := messageInput(opts.message, opts.messageFile, cmd.Flags().Changed("message-file"))
	if err != nil {
		return role.Role{}, "", err
	}

	return r, input, nil
}

func renderExecutionInput(cmd *cobra.Command, r role.Role, input string, opts *executionOptions) (string, error) {
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

func runOneShotExecution(cmd *cobra.Command, r role.Role, outbound string, opts *executionOptions) (resultErr error) {
	executionCtx, cancel := context.WithTimeout(cmd.Context(), opts.timeout)
	defer cancel()

	factory := runtime.NormaFactory{
		Stderr:          cmd.ErrOrStderr(),
		JSONDiagnostics: opts.jsonOutput,
	}

	lifecycle, err := newRoleLifecycle(executionCtx, factory, r)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = stopRoleLifecycle(lifecycle, resultErr)
	}()

	if err := lifecycle.Start(executionCtx); err != nil {
		return err
	}

	conversation, err := lifecycle.Conversation()
	if err != nil {
		return err
	}

	result, err := conversation.Run(executionCtx, r, outbound, opts.threadID)
	if err != nil {
		return fmt.Errorf("role %q: %w", r.ID, err)
	}

	if opts.jsonOutput {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(executionOutput{
			ThreadID: result.ThreadID,
			Content:  result.Content,
			Resumed:  opts.threadID != "" && result.ThreadID == opts.threadID,
		})
	}

	_, err = fmt.Fprintln(cmd.OutOrStdout(), result.Content)

	return err
}

func runAgentREPL(cmd *cobra.Command, r role.Role, initialPrompt, threadID string, timeout time.Duration, terminal io.Writer, reader *bufio.Reader) (resultErr error) {
	factory := runtime.NormaFactory{
		Stderr:            cmd.ErrOrStderr(),
		PermissionHandler: permissionHandler(reader, terminal),
	}

	lifecycle, err := newRoleLifecycle(cmd.Context(), factory, r)
	if err != nil {
		return err
	}
	defer func() {
		resultErr = stopRoleLifecycle(lifecycle, resultErr)
	}()

	startupCtx, startupCancel := context.WithTimeout(cmd.Context(), timeout)
	err = lifecycle.Start(startupCtx)

	startupCancel()

	if err != nil {
		return err
	}

	conversation, err := lifecycle.Conversation()
	if err != nil {
		return err
	}

	lastResponse, err := runAgentTurn(cmd, conversation, r, initialPrompt, threadID, timeout)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintln(terminal, lastResponse); err != nil {
		return err
	}

	for {
		if _, err := fmt.Fprint(terminal, "> "); err != nil {
			return err
		}

		line, readErr := readLine(cmd.Context(), reader)
		input := strings.TrimSpace(line)

		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return fmt.Errorf("read REPL input: %w", readErr)
		}

		if input == "exit" || input == "quit" {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), lastResponse)

			return err
		}

		if input != "" {
			lastResponse, err = runAgentTurn(cmd, conversation, r, input, "", timeout)
			if err != nil {
				return err
			}

			if _, err := fmt.Fprintln(terminal, lastResponse); err != nil {
				return err
			}
		}

		if errors.Is(readErr, io.EOF) {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), lastResponse)

			return err
		}
	}
}

func runAgentTurn(cmd *cobra.Command, conversation runtime.Conversation, r role.Role, prompt, threadID string, timeout time.Duration) (string, error) {
	turnCtx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	result, err := conversation.Run(turnCtx, r, prompt, threadID)
	if err != nil {
		return "", fmt.Errorf("role %q: %w", r.ID, err)
	}

	return result.Content, nil
}

func agentTerminal() (io.ReadWriteCloser, *bufio.Reader, error) {
	terminal, err := openTerminal()
	if err != nil {
		return nil, nil, fmt.Errorf("interactive terminal is required: %w", err)
	}

	return terminal, bufio.NewReader(terminal), nil
}

func missingRuntimeParameters(r role.Role, values map[string]string) ([]string, error) {
	unknown := make([]string, 0)

	for name := range values {
		if _, ok := r.Metadata.Params[name]; !ok {
			unknown = append(unknown, name)
		}
	}

	sort.Strings(unknown)

	if len(unknown) > 0 {
		return nil, fmt.Errorf("role %q parameters: missing=[] unknown=%v", r.ID, unknown)
	}

	missing := make([]string, 0)

	for name := range r.Metadata.Params {
		if _, ok := values[name]; !ok {
			missing = append(missing, name)
		}
	}

	sort.Strings(missing)

	return missing, nil
}

func collectRuntimeParameters(ctx context.Context, terminal io.Writer, reader *bufio.Reader, r role.Role, values map[string]string, missing []string) error {
	for _, name := range missing {
		description := strings.TrimSpace(r.Metadata.Params[name])
		if _, err := fmt.Fprintf(terminal, "%s — %s: ", name, description); err != nil {
			return fmt.Errorf("prompt for role parameter %q: %w", name, err)
		}

		line, err := readLine(ctx, reader)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("read role parameter %q: %w", name, err)
		}

		if errors.Is(err, io.EOF) && line == "" {
			return fmt.Errorf("read role parameter %q: interactive terminal closed", name)
		}

		values[name] = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	}

	return nil
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

func messageInput(message, path string, useFile bool) (string, error) {
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
	if !errors.Is(ctx.Err(), context.Canceled) {
		return false
	}

	var stopErr roleLifecycleStopError
	if errors.As(err, &stopErr) {
		return false
	}

	return isCancellationOnly(err)
}

func isCancellationOnly(err error) bool {
	if err == nil {
		return false
	}

	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		nested := joined.Unwrap()
		if len(nested) == 0 {
			return false
		}

		for _, nestedErr := range nested {
			if !isCancellationOnly(nestedErr) {
				return false
			}
		}

		return true
	}

	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		if nestedErr := wrapped.Unwrap(); nestedErr != nil {
			return isCancellationOnly(nestedErr)
		}
	}

	return errors.Is(err, context.Canceled)
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
