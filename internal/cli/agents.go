package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	resource "github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/callee/internal/registry"
	"github.com/baldaworks/callee/internal/runtime"
	"github.com/baldaworks/callee/internal/workflow"
	acp "github.com/coder/acp-go-sdk"
	"github.com/spf13/cobra"
)

const agentListTabPadding = 2

var newWorkflowFactory = func(stderr io.Writer, interactor *terminalInteractor, pauses *workflow.PauseController) runtime.ProcessFactory {
	return runtime.NormaFactory{
		Stderr:            stderr,
		PermissionHandler: workflowPermissionHandler(interactor, pauses),
	}
}

type agentRunOptions struct {
	message     string
	params      []string
	paramFiles  []string
	replTimeout time.Duration
}

type agentListOutput struct {
	Agents []agentListItem `json:"agents"`
}

type agentListItem struct {
	ResourceID  string        `json:"resourceId"`
	APIVersion  string        `json:"apiVersion"`
	Kind        resource.Kind `json:"kind"`
	Description string        `json:"description"`
}

type agentViewOutput struct {
	ResourceID     string                   `json:"resourceId"`
	Resource       resource.Resource        `json:"resource"`
	ResolvedTree   *registry.ResolvedNode   `json:"resolvedTree"`
	RequiredParams []registry.RequiredParam `json:"requiredParams"`
}

func loadAgentRegistry() (*registry.AgentRegistry, error) {
	return registry.LoadAgents(registry.AgentLoadOptions{})
}

func agentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Run and inspect Callee agents",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("an agent command is required")
		},
	}
	cmd.AddCommand(agentRunCommand())
	cmd.AddCommand(agentListCommand())
	cmd.AddCommand(agentViewCommand())
	cmd.AddCommand(agentValidateCommand())

	return cmd
}

func agentValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate <path>",
		Short: "Validate one Markdown or YAML agent file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			if !resource.SupportsFile(path) {
				return fmt.Errorf("unsupported agent file extension %q (want .md, .yaml, or .yml)", filepath.Ext(path))
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read agent file %q: %w", path, err)
			}

			id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if id == "" {
				id = "agent"
			}

			if _, err := resource.Decode(id, path, data); err != nil {
				return fmt.Errorf("validate agent file %q: %w", path, err)
			}

			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: ok\n", path)

			return err
		},
	}

	return cmd
}

func agentRunCommand() *cobra.Command {
	opts := &agentRunOptions{}

	cmd := &cobra.Command{
		Use:   "run <agent-id>",
		Short: "Run a versioned Callee agent tree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWorkflowAgent(cmd, args[0], opts)
		},
	}
	cmd.Flags().StringVar(&opts.message, "message", "", "root user prompt; omit to read one line from the controlling terminal")
	cmd.Flags().StringArrayVar(&opts.params, "param", nil, "qualified Role parameter as node.name=value; repeatable")
	cmd.Flags().StringArrayVar(&opts.paramFiles, "param-file", nil, "qualified Role parameter as node.name=path; repeatable")
	cmd.Flags().DurationVar(&opts.replTimeout, "repl-timeout", resource.DefaultREPLTimeout(), "maximum wait for each operator prompt")

	return cmd
}

func runWorkflowAgent(cmd *cobra.Command, id string, opts *agentRunOptions) error {
	if opts.replTimeout <= 0 {
		return fmt.Errorf("repl-timeout must be greater than zero")
	}

	configured, err := loadAgentRegistry()
	if err != nil {
		return err
	}

	root, err := configured.Resolve(id)
	if err != nil {
		return err
	}

	values, err := parameterValues(opts.params, opts.paramFiles, "workflow parameter", "is specified more than once")
	if err != nil {
		return err
	}

	terminal, reader, err := agentTerminal()
	if err != nil {
		return err
	}
	defer terminal.Close()

	interactor := &terminalInteractor{
		reader:   reader,
		terminal: terminal,
		timeout:  opts.replTimeout,
	}

	prompt := opts.message
	if !cmd.Flags().Changed("message") {
		prompt, err = interactor.Prompt(cmd.Context(), "Prompt")
		if err != nil {
			return err
		}
	} else if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("message must not be blank")
	}

	pauses := workflow.NewPauseController()
	factory := newWorkflowFactory(cmd.ErrOrStderr(), interactor, pauses)

	artifact, err := (workflow.Runner{
		Root:       root,
		Factory:    factory,
		Interactor: interactor,
		Params:     values,
		Pauses:     pauses,
	}).Run(cmd.Context(), prompt)
	if err != nil {
		return err
	}

	_, err = io.WriteString(cmd.OutOrStdout(), artifact)

	return err
}

func agentListCommand() *cobra.Command {
	var (
		kind       string
		jsonOutput bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List versioned Callee agents",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			configured, err := loadAgentRegistry()
			if err != nil {
				return err
			}

			filter, err := parseKindFilter(kind)
			if err != nil {
				return err
			}

			resources := configured.Agents()
			if jsonOutput {
				output := agentListOutput{Agents: make([]agentListItem, 0, len(resources))}
				for _, item := range resources {
					if filter != "" && item.Kind != filter {
						continue
					}

					output.Agents = append(output.Agents, agentListItem{
						ResourceID:  item.ID,
						APIVersion:  item.APIVersion,
						Kind:        item.Kind,
						Description: strings.TrimSpace(item.Spec.Description),
					})
				}

				return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
			}

			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, agentListTabPadding, ' ', 0)
			if _, err := fmt.Fprintln(out, "ID\tKIND\tDESCRIPTION"); err != nil {
				return err
			}

			for _, item := range resources {
				if filter != "" && item.Kind != filter {
					continue
				}

				if _, err := fmt.Fprintf(out, "%s\t%s\t%s\n", item.ID, item.Kind, strings.TrimSpace(item.Spec.Description)); err != nil {
					return err
				}
			}

			return out.Flush()
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "filter by Role, Sequential, or Loop")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the catalog as JSON")

	return cmd
}

func agentViewCommand() *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "view <agent-id>",
		Short: "View a versioned Callee agent and its resolved tree",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configured, err := loadAgentRegistry()
			if err != nil {
				return err
			}

			selected, err := configured.GetAgent(args[0])
			if err != nil {
				return err
			}

			resolved, err := configured.Resolve(args[0])
			if err != nil {
				return err
			}

			required := registry.RequiredParams(resolved)
			if jsonOutput {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(agentViewOutput{
					ResourceID:     selected.ID,
					Resource:       selected,
					ResolvedTree:   resolved,
					RequiredParams: required,
				})
			}

			return writeAgentView(cmd.OutOrStdout(), selected, resolved, required)
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output the canonical resource and resolved tree as JSON")

	return cmd
}

func parseKindFilter(value string) (resource.Kind, error) {
	switch resource.Kind(value) {
	case "":
		return "", nil
	case resource.RoleKind, resource.SequentialKind, resource.LoopKind:
		return resource.Kind(value), nil
	default:
		return "", fmt.Errorf("unsupported kind %q (want Role, Sequential, or Loop)", value)
	}
}

func writeAgentView(output io.Writer, selected resource.Resource, resolved *registry.ResolvedNode, required []registry.RequiredParam) error {
	if _, err := fmt.Fprintf(output, "Resource\n  ID: %s\n  API version: %s\n  Kind: %s\n  Description: %s\n\n", selected.ID, selected.APIVersion, selected.Kind, strings.TrimSpace(selected.Spec.Description)); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(output, "Resolved Tree"); err != nil {
		return err
	}

	if err := writeResolvedNode(output, resolved, "  "); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(output, "\nRequired Parameters"); err != nil {
		return err
	}

	if len(required) == 0 {
		_, err := fmt.Fprintln(output, "  none")

		return err
	}

	for _, parameter := range required {
		if _, err := fmt.Fprintf(output, "  %s — %s (source: %s)\n", parameter.Key, parameter.Description, parameter.SourceRoleID); err != nil {
			return err
		}
	}

	return nil
}

func writeResolvedNode(output io.Writer, node *registry.ResolvedNode, indent string) error {
	policy := fmt.Sprintf(" canEscalate=%t", node.CanEscalate)

	switch node.Kind {
	case resource.RoleKind:
		policy += fmt.Sprintf(" repl=%t", node.REPL != nil && *node.REPL)
	case resource.LoopKind:
		maxIterations := 0
		if node.MaxIterations != nil {
			maxIterations = *node.MaxIterations
		}

		policy += fmt.Sprintf(" maxIterations=%d onExhausted=%s", maxIterations, node.OnExhausted)
	}

	if _, err := fmt.Fprintf(output, "%s%s [%s] -> %s%s\n", indent, node.EffectiveID, node.Kind, node.ResourceID, policy); err != nil {
		return err
	}

	for _, child := range node.Children {
		if err := writeResolvedNode(output, child, indent+"  "); err != nil {
			return err
		}
	}

	return nil
}

type terminalInteractor struct {
	reader   *bufio.Reader
	terminal io.Writer
	timeout  time.Duration
}

func (i *terminalInteractor) Prompt(ctx context.Context, label string) (string, error) {
	for {
		if _, err := fmt.Fprintf(i.terminal, "%s: ", label); err != nil {
			return "", err
		}

		promptCtx, cancel := context.WithTimeout(ctx, i.timeout)
		line, err := readLine(promptCtx, i.reader)

		cancel()

		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}

		if errors.Is(err, io.EOF) && line == "" {
			return "", fmt.Errorf("interactive terminal closed")
		}

		value := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
		if value == "/abort" {
			return "", fmt.Errorf("workflow aborted by operator")
		}

		if strings.TrimSpace(value) != "" {
			return value, nil
		}
	}
}

func (i *terminalInteractor) Display(text string) error {
	_, err := fmt.Fprintln(i.terminal, text)

	return err
}

func workflowPermissionHandler(interactor *terminalInteractor, pauses *workflow.PauseController) func(context.Context, acp.RequestPermissionRequest) (acp.RequestPermissionResponse, error) {
	return func(ctx context.Context, request acp.RequestPermissionRequest) (response acp.RequestPermissionResponse, resultErr error) {
		if len(request.Options) == 0 {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
		}

		if err := pauses.Pause(ctx); err != nil {
			return acp.RequestPermissionResponse{}, err
		}
		defer func() {
			resultErr = errors.Join(resultErr, pauses.Resume(ctx))
		}()

		var choices strings.Builder
		choices.WriteString("Permission required:\n")

		for index, option := range request.Options {
			_, _ = fmt.Fprintf(&choices, "%d) %s [%s]\n", index+1, option.Name, option.Kind)
		}

		if err := interactor.Display(strings.TrimSuffix(choices.String(), "\n")); err != nil {
			return acp.RequestPermissionResponse{}, err
		}

		answer, err := interactor.Prompt(ctx, "Select")
		if err != nil {
			return acp.RequestPermissionResponse{}, err
		}

		selection, err := strconv.Atoi(strings.TrimSpace(answer))
		if err != nil || selection < 1 || selection > len(request.Options) {
			return acp.RequestPermissionResponse{Outcome: acp.NewRequestPermissionOutcomeCancelled()}, nil
		}

		return acp.RequestPermissionResponse{
			Outcome: acp.NewRequestPermissionOutcomeSelected(request.Options[selection-1].OptionId),
		}, nil
	}
}

func agentTerminal() (io.ReadWriteCloser, *bufio.Reader, error) {
	terminal, err := openTerminal()
	if err != nil {
		return nil, nil, fmt.Errorf("interactive terminal is required: %w", err)
	}

	return terminal, bufio.NewReader(terminal), nil
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
