package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/baldaworks/callee/internal/role"
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
	cmd.Short = "Browse PromptKit, assemble prompts, and generate Callee roles"

	roleCommand := &cobra.Command{Use: "role", Short: "Generate a Callee role", Args: cobra.NoArgs}
	roleCommand.AddCommand(promptKitRoleCreateCommand())
	cmd.AddCommand(roleCommand)

	return cmd
}

func promptKitRoleCreateCommand() *cobra.Command {
	var (
		template, description, roleType, promptParam, output string
		model, reasoning, mode, runtimeCommand               string
		persona, format                                      string
		bindings, bindingFiles, extraArgs                    []string
		protocols, taxonomies                                []string
		dryRun, force, noFormat, repl                        bool
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

			metadata := role.Metadata{
				API:         role.CurrentAPI,
				Kind:        role.RoleKind,
				Description: description,
				REPL:        repl,
				Provider: role.Provider{
					Type:      roleType,
					Cmd:       runtimeCommand,
					Model:     model,
					Reasoning: reasoning,
					Mode:      mode,
					ExtraArgs: extraArgs,
				},
				Params: runtimeParams,
			}
			body := promptKitRoleBody(promptParam, runtimeParams, assembled.Markdown)

			return writePromptKitRole(cmd, args[0], output, body, metadata, dryRun, force)
		},
	}

	cmd.Flags().StringVar(&template, "template", "", "PromptKit template name")
	cmd.Flags().StringVar(&description, "description", "", "role description")
	cmd.Flags().StringVar(&roleType, "type", "", "Callee runtime type")
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
	cmd.Flags().BoolVar(&repl, "repl", false, "enable top-level REPL behavior in the generated role")
	cmd.Flags().StringVar(&output, "output", "", "role file path")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the generated role without writing it")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing role file")
	cmd.MarkFlagsMutuallyExclusive("format", "no-format")
	_ = cmd.MarkFlagRequired("template")
	_ = cmd.MarkFlagRequired("description")
	_ = cmd.MarkFlagRequired("type")
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
	body.WriteString("`:\n\n{{ prompt }}")

	for _, name := range sortedKeys(params) {
		body.WriteString("\n\nPromptKit parameter `")
		body.WriteString(name)
		body.WriteString("` — ")
		body.WriteString(params[name])
		body.WriteString(":\n\n{{ ")
		body.WriteString(name)
		body.WriteString(" }}")
	}

	body.WriteString("\n\n---\n\n")
	body.WriteString(assembled)

	return body.String()
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	return keys
}

func writePromptKitRole(cmd *cobra.Command, roleID, output, body string, metadata role.Metadata, dryRun, force bool) error {
	path, err := generatedRolePath(roleID, output)
	if err != nil {
		return err
	}

	generated, err := marshalPromptKitRole(role.Role{ID: roleID, Metadata: metadata, Template: body})
	if err != nil {
		return err
	}

	if dryRun {
		_, err = cmd.OutOrStdout().Write(generated)

		return err
	}

	if err := writeGeneratedRole(path, generated, force); err != nil {
		return err
	}

	_, err = fmt.Fprintf(cmd.OutOrStdout(), "created %s\n", path)

	return err
}

func marshalPromptKitRole(generated role.Role) ([]byte, error) {
	if strings.HasPrefix(strings.TrimSpace(generated.Template), "---") {
		return nil, fmt.Errorf("PromptKit output must not include YAML frontmatter")
	}

	if err := generated.Validate(); err != nil {
		return nil, err
	}

	return generated.MarshalMarkdown()
}

func generatedRolePath(roleID, output string) (string, error) {
	if output != "" {
		return output, nil
	}

	clean := filepath.Clean(filepath.FromSlash(roleID))
	invalidParent := strings.HasPrefix(clean, ".."+string(filepath.Separator))

	if strings.TrimSpace(roleID) == "" || filepath.IsAbs(clean) || clean == "." || clean == ".." || invalidParent {
		return "", fmt.Errorf("invalid role ID %q", roleID)
	}

	return filepath.Join(".callee", "roles", clean+".md"), nil
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
