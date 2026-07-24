package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	resource "github.com/baldaworks/callee/internal/agent"
	"github.com/baldaworks/promptkitty"
	promptkittycli "github.com/baldaworks/promptkitty/cli"
	"github.com/spf13/cobra"
)

const (
	generatedRoleDirectoryMode = 0o750
	generatedRoleFileMode      = 0o600
	runtimePromptReference     = "the user message supplied in the Runtime Input section"
)

func promptKitCommand() *cobra.Command {
	cmd := promptkittycli.NewCommand(promptkittycli.Options{Use: "promptkit"})
	for _, child := range cmd.Commands() {
		switch child.Name() {
		case "list", "search", "show":
		default:
			cmd.RemoveCommand(child)
		}
	}

	cmd.Short = "Browse the PromptKit catalog and generate Callee roles"
	cmd.Args = cobra.NoArgs
	cmd.RunE = promptKitGroupHelp

	roleCommand := &cobra.Command{
		Use:   "role",
		Short: "Generate a Callee role",
		Args:  cobra.NoArgs,
		RunE:  promptKitGroupHelp,
	}
	roleCommand.AddCommand(promptKitRoleCreateCommand())
	cmd.AddCommand(roleCommand)

	return cmd
}

func promptKitGroupHelp(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}

func promptKitRoleCreateCommand() *cobra.Command {
	var (
		template, description, providerType, promptParam, output string
		model, reasoning, mode, runtimeCommand                   string
		persona, format                                          string
		bindings, bindingFiles, extraArgs                        []string
		protocols, taxonomies                                    []string
		dryRun, force, noFormat, interactive, repl               bool
	)

	cmd := &cobra.Command{
		Use:   "create <role-id>",
		Short: "Assemble and write a Callee role",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			library, err := promptkitty.New()
			if err != nil {
				return fmt.Errorf("load embedded PromptKit catalog: %w", err)
			}

			detail, err := library.Show(template)
			if err != nil {
				return err
			}

			descriptions, err := promptKitParameterDescriptions(detail)
			if err != nil {
				return err
			}

			templateInteractive, err := promptKitTemplateInteractive(detail)
			if err != nil {
				return err
			}

			values, runtimeParams, err := compilePromptKitParameters(descriptions, promptParam, bindings, bindingFiles, persona)
			if err != nil {
				return err
			}

			assembled, err := library.Assemble(promptkitty.AssembleRequest{
				Template:             template,
				Params:               values,
				Persona:              persona,
				AdditionalProtocols:  protocols,
				AdditionalTaxonomies: taxonomies,
				Format:               promptKitFormatOverride(cmd, format, noFormat),
			})
			if err != nil {
				return err
			}

			if strings.HasPrefix(strings.TrimSpace(assembled.Markdown), "---") {
				return fmt.Errorf("PromptKit output must not include YAML frontmatter")
			}

			var interactiveValue *bool

			if interactive || repl || templateInteractive {
				enabled := true
				interactiveValue = &enabled
			}

			generated := resource.Resource{
				APIVersion: resource.APIVersion,
				Kind:       resource.RoleKind,
				ID:         args[0],
				Spec: resource.Spec{
					Description: description,
					Interactive: interactiveValue,
					Provider: &resource.Provider{
						Type:      providerType,
						Cmd:       runtimeCommand,
						Model:     model,
						Reasoning: reasoning,
						Mode:      mode,
						ExtraArgs: extraArgs,
					},
					Params: runtimeParams,
				},
			}
			generated.Spec.Body = promptKitRoleBody(promptParam, runtimeParams, assembled.Markdown)

			return writePromptKitRole(cmd, args[0], output, generated, dryRun, force)
		},
	}

	cmd.Flags().StringVar(&template, "template", "", "PromptKit template name")
	cmd.Flags().StringVar(&description, "description", "", "role description")
	cmd.Flags().StringVar(&providerType, "provider", "", "Callee provider type")
	cmd.Flags().StringVar(&promptParam, "prompt-param", "", "PromptKit parameter supplied by the user message")
	cmd.Flags().StringArrayVar(&bindings, "bind", nil, "bind a PromptKit parameter as key=value; repeatable")
	cmd.Flags().StringArrayVar(&bindingFiles, "bind-file", nil, "bind a PromptKit parameter as key=path; repeatable")
	cmd.Flags().StringVar(&persona, "persona", "", "replace the PromptKit persona at role creation time")
	cmd.Flags().StringArrayVar(&protocols, "protocol", nil, "add a PromptKit protocol; repeatable")
	cmd.Flags().StringArrayVar(&taxonomies, "taxonomy", nil, "add a PromptKit taxonomy; repeatable")
	cmd.Flags().StringVar(&format, "format", "", "replace the PromptKit output format")
	cmd.Flags().BoolVar(&noFormat, "no-format", false, "omit the PromptKit output format")
	cmd.Flags().StringVar(&model, "model", "", "model identifier")
	cmd.Flags().StringVar(&reasoning, "reasoning", "", "runtime reasoning effort")
	cmd.Flags().StringVar(&mode, "mode", "", "runtime session mode")
	cmd.Flags().StringVar(&runtimeCommand, "cmd", "", "runtime command override")
	cmd.Flags().StringArrayVar(&extraArgs, "extra-arg", nil, "argument appended to the runtime; repeatable")
	cmd.Flags().BoolVar(&interactive, "interactive", false, "enable interactive multi-turn behavior; interactive PromptKit templates enable it automatically")
	cmd.Flags().BoolVar(&repl, "repl", false, "deprecated alias for --interactive")
	cmd.Flags().StringVar(&output, "output", "", "role file path")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the generated role without writing it")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing role file")
	cmd.MarkFlagsMutuallyExclusive("format", "no-format")
	_ = cmd.Flags().MarkHidden("repl")
	_ = cmd.MarkFlagRequired("template")
	_ = cmd.MarkFlagRequired("description")
	_ = cmd.MarkFlagRequired("provider")
	_ = cmd.MarkFlagRequired("prompt-param")

	return cmd
}

func promptKitParameterDescriptions(detail promptkitty.ComponentDetail) (map[string]string, error) {
	if detail.Type != promptkitty.ComponentTemplate {
		return nil, fmt.Errorf("PromptKit component %q is not a template", detail.Name)
	}

	raw, ok := detail.Metadata["params"]
	if !ok {
		return map[string]string{}, nil
	}

	values, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("PromptKit template %q has invalid parameter metadata", detail.Name)
	}

	descriptions := make(map[string]string, len(values))
	for name, value := range values {
		description, ok := value.(string)
		if !ok || strings.TrimSpace(description) == "" {
			return nil, fmt.Errorf("PromptKit template %q parameter %q requires a description", detail.Name, name)
		}

		descriptions[name] = description
	}

	return descriptions, nil
}

func promptKitTemplateInteractive(detail promptkitty.ComponentDetail) (bool, error) {
	raw, ok := detail.Metadata["mode"]
	if !ok {
		return false, nil
	}

	mode, ok := raw.(string)
	if !ok {
		return false, fmt.Errorf("PromptKit template %q has invalid mode metadata", detail.Name)
	}

	return mode == "interactive", nil
}

func compilePromptKitParameters(descriptions map[string]string, promptParam string, raw, files []string, persona string) (map[string]string, map[string]string, error) {
	promptParam = strings.TrimSpace(promptParam)
	if _, ok := descriptions[promptParam]; !ok {
		return nil, nil, fmt.Errorf("PromptKit prompt parameter %q is not declared by the template", promptParam)
	}

	if promptParam == "persona" {
		return nil, nil, fmt.Errorf("PromptKit parameter %q must be selected with --persona, not --prompt-param", promptParam)
	}

	bound, err := promptKitBindings(raw, files)
	if err != nil {
		return nil, nil, err
	}

	for name := range bound {
		if _, ok := descriptions[name]; !ok {
			return nil, nil, fmt.Errorf("PromptKit parameter %q is not declared by the template", name)
		}

		if name == promptParam {
			return nil, nil, fmt.Errorf("PromptKit prompt parameter %q cannot also be bound", name)
		}

		if name == "persona" {
			return nil, nil, fmt.Errorf("PromptKit parameter %q must be selected with --persona, not --bind", name)
		}
	}

	values := make(map[string]string, len(descriptions))
	options := sortedKeys(descriptions)
	runtimeParams := make(map[string]string)

	for _, name := range options {
		boundValue, isBound := bound[name]
		switch {
		case name == promptParam:
			values[name] = runtimePromptReference
		case name == "persona":
			if strings.TrimSpace(persona) == "" {
				return nil, nil, fmt.Errorf("PromptKit template declares configurable persona; use --persona")
			}

			values[name] = persona
		case isBound:
			values[name] = boundValue
		default:
			values[name] = fmt.Sprintf("the `%s` value supplied in the Runtime Input section", name)
			runtimeParams[name] = descriptions[name]
		}
	}

	return values, runtimeParams, nil
}

func promptKitBindings(raw, files []string) (map[string]string, error) {
	return parameterValues(raw, files, "PromptKit parameter", "is bound more than once")
}

func promptKitFormatOverride(cmd *cobra.Command, format string, noFormat bool) *string {
	if noFormat {
		empty := ""

		return &empty
	}

	if cmd.Flags().Changed("format") {
		return &format
	}

	return nil
}

func promptKitRoleBody(promptParam string, params map[string]string, assembled string) string {
	var body strings.Builder
	body.WriteString("# Runtime Input\n\nPromptKit parameter `")
	body.WriteString(promptParam)
	body.WriteString("`:\n\n{{ .Input }}")

	for _, name := range sortedKeys(params) {
		body.WriteString("\n\nPromptKit parameter `")
		body.WriteString(name)
		body.WriteString("` — ")
		body.WriteString(params[name])
		body.WriteString(":\n\n{{ index .Params ")
		body.WriteString(fmt.Sprintf("%q", name))
		body.WriteString(" }}")
	}

	body.WriteString("\n\n---\n\n")
	body.WriteString(escapeGoTemplateText(assembled))

	return body.String()
}

func escapeGoTemplateText(value string) string {
	const (
		openMarker  = "\ufdd0CALLEE_OPEN_TEMPLATE\ufdd1"
		closeMarker = "\ufdd0CALLEE_CLOSE_TEMPLATE\ufdd1"
	)

	value = strings.ReplaceAll(value, "{{", openMarker)
	value = strings.ReplaceAll(value, "}}", closeMarker)
	value = strings.ReplaceAll(value, openMarker, `{{ "{{" }}`)
	value = strings.ReplaceAll(value, closeMarker, `{{ "}}" }}`)

	return value
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func writePromptKitRole(cmd *cobra.Command, roleID, output string, generated resource.Resource, dryRun, force bool) error {
	path, err := generatedRolePath(cmd, roleID, output)
	if err != nil {
		return err
	}

	contents, err := marshalPromptKitRole(generated)
	if err != nil {
		return err
	}

	if dryRun {
		_, err = cmd.OutOrStdout().Write(contents)

		return err
	}

	if err := writeGeneratedRole(path, contents, force); err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)

	return err
}

func marshalPromptKitRole(generated resource.Resource) ([]byte, error) {
	if strings.HasPrefix(strings.TrimSpace(generated.Spec.Body), "---") {
		return nil, fmt.Errorf("PromptKit output must not include YAML frontmatter")
	}

	return resource.EncodeMarkdown(generated)
}

func generatedRolePath(cmd *cobra.Command, roleID, output string) (string, error) {
	if output != "" {
		return output, nil
	}

	clean := filepath.Clean(filepath.FromSlash(roleID))
	invalidParent := strings.HasPrefix(clean, ".."+string(filepath.Separator))

	if strings.TrimSpace(roleID) == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || invalidParent {
		return "", fmt.Errorf("invalid role ID %q", roleID)
	}

	root, err := defaultAgentRoot(cmd)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "roles", clean+".md"), nil
}

func writeGeneratedRole(path string, content []byte, force bool) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), generatedRoleDirectoryMode); err != nil {
		return fmt.Errorf("create role directory: %w", err)
	}

	flags := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if force {
		flags = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}

	file, err := os.OpenFile(path, flags, generatedRoleFileMode)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("role file %q already exists; use --force to overwrite it", path)
		}

		return fmt.Errorf("create generated role: %w", err)
	}

	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = fmt.Errorf("close generated role: %w", closeErr)
		}
	}()

	if _, err := file.Write(content); err != nil {
		return fmt.Errorf("write generated role: %w", err)
	}

	return nil
}
