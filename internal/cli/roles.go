package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/baldaworks/callee/internal/role"
	"github.com/spf13/cobra"
)

const roleListTabPadding = 2

type roleListOutput struct {
	Roles []roleListItem `json:"roles"`
}

type roleListItem struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	REPL        bool              `json:"repl"`
	Params      map[string]string `json:"params"`
}

type roleViewOutput struct {
	ID          string            `json:"id"`
	API         string            `json:"api"`
	Kind        string            `json:"kind"`
	Description string            `json:"description"`
	REPL        bool              `json:"repl"`
	Provider    roleProviderView  `json:"provider"`
	Params      map[string]string `json:"params"`
}

type roleProviderView struct {
	Type             string   `json:"type"`
	Cmd              string   `json:"cmd,omitempty"`
	Model            string   `json:"model,omitempty"`
	Reasoning        string   `json:"reasoning,omitempty"`
	Mode             string   `json:"mode,omitempty"`
	ExtraArgs        []string `json:"extraArgs,omitempty"`
	Timeout          string   `json:"timeout,omitempty"`
	EffectiveTimeout string   `json:"effectiveTimeout"`
	TimeoutSource    string   `json:"timeoutSource"`
}

func roleCommand(rolesDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "role",
		Short: "Inspect configured Callee roles",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("a role command is required")
		},
	}
	cmd.AddCommand(roleListCommand(rolesDir))
	cmd.AddCommand(roleViewCommand(rolesDir))

	return cmd
}

func roleListCommand(rolesDir *string) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List configured Callee roles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			reg, err := load(*rolesDir)
			if err != nil {
				return err
			}

			roles := reg.Roles()
			if jsonOutput {
				output := roleListOutput{Roles: make([]roleListItem, 0, len(roles))}
				for _, configuredRole := range roles {
					output.Roles = append(output.Roles, roleListItem{
						ID:          configuredRole.ID,
						Description: strings.TrimSpace(configuredRole.Metadata.Description),
						REPL:        configuredRole.Metadata.REPL,
						Params:      normalizedParams(configuredRole.Metadata.Params),
					})
				}

				return json.NewEncoder(cmd.OutOrStdout()).Encode(output)
			}

			out := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, roleListTabPadding, ' ', 0)
			if _, err := fmt.Fprintln(out, "ID\tREPL\tDESCRIPTION\tPARAMETERS"); err != nil {
				return err
			}

			for _, configuredRole := range roles {
				paramNames := strings.Join(sortedKeys(configuredRole.Metadata.Params), ", ")
				if paramNames == "" {
					paramNames = "-"
				}

				if _, err := fmt.Fprintf(
					out,
					"%s\t%t\t%s\t%s\n",
					configuredRole.ID,
					configuredRole.Metadata.REPL,
					strings.TrimSpace(configuredRole.Metadata.Description),
					paramNames,
				); err != nil {
					return err
				}
			}

			return out.Flush()
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output roles as JSON and diagnostics as JSON Lines")

	return cmd
}

func roleViewCommand(rolesDir *string) *cobra.Command {
	var jsonOutput, markdownOutput bool

	cmd := &cobra.Command{
		Use:   "view <role-id>",
		Short: "View a configured Callee role",
		Args:  cobra.ExactArgs(1),
		PreRunE: func(_ *cobra.Command, _ []string) error {
			if jsonOutput && markdownOutput {
				return errors.New("--json and --markdown are mutually exclusive")
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			reg, err := load(*rolesDir)
			if err != nil {
				return err
			}

			configuredRole, err := reg.Get(args[0])
			if err != nil {
				return err
			}

			switch {
			case jsonOutput:
				return json.NewEncoder(cmd.OutOrStdout()).Encode(newRoleViewOutput(configuredRole))
			case markdownOutput:
				contents, marshalErr := configuredRole.MarshalMarkdown()
				if marshalErr != nil {
					return marshalErr
				}

				_, err = cmd.OutOrStdout().Write(contents)

				return err
			default:
				return writeRoleView(cmd, configuredRole)
			}
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output role metadata as JSON and diagnostics as JSON Lines")
	cmd.Flags().BoolVar(&markdownOutput, "markdown", false, "output the normalized Markdown role")

	return cmd
}

func normalizedParams(params map[string]string) map[string]string {
	normalized := make(map[string]string, len(params))
	for name, description := range params {
		normalized[name] = strings.TrimSpace(description)
	}

	return normalized
}

func newRoleViewOutput(configuredRole role.Role) roleViewOutput {
	provider := configuredRole.Metadata.Provider
	effectiveTimeout, timeoutSource := roleTimeoutDetails(configuredRole)

	return roleViewOutput{
		ID:          configuredRole.ID,
		API:         configuredRole.API(),
		Kind:        configuredRole.Kind(),
		Description: strings.TrimSpace(configuredRole.Metadata.Description),
		REPL:        configuredRole.Metadata.REPL,
		Provider: roleProviderView{
			Type:             provider.Type,
			Cmd:              provider.Cmd,
			Model:            provider.Model,
			Reasoning:        provider.Reasoning,
			Mode:             provider.Mode,
			ExtraArgs:        provider.ExtraArgs,
			Timeout:          provider.Timeout,
			EffectiveTimeout: effectiveTimeout,
			TimeoutSource:    timeoutSource,
		},
		Params: normalizedParams(configuredRole.Metadata.Params),
	}
}

func writeRoleView(cmd *cobra.Command, configuredRole role.Role) error {
	metadata := configuredRole.Metadata
	provider := metadata.Provider
	effectiveTimeout, timeoutSource := roleTimeoutDetails(configuredRole)

	lines := []string{
		"ID: " + configuredRole.ID,
		"API: " + configuredRole.API(),
		"Kind: " + configuredRole.Kind(),
		"Description: " + strings.TrimSpace(metadata.Description),
		"Provider type: " + provider.Type,
		"REPL: " + fmt.Sprint(metadata.REPL),
		"Timeout: " + effectiveTimeout + " (" + timeoutSource + ")",
	}

	optionalFields := []struct {
		label string
		value string
	}{
		{label: "Command", value: provider.Cmd},
		{label: "Model", value: provider.Model},
		{label: "Reasoning", value: provider.Reasoning},
		{label: "Mode", value: provider.Mode},
	}
	for _, field := range optionalFields {
		if field.value != "" {
			lines = append(lines, field.label+": "+field.value)
		}
	}

	if len(provider.ExtraArgs) > 0 {
		lines = append(lines, fmt.Sprintf("Extra args: %q", provider.ExtraArgs))
	}

	if len(metadata.Params) == 0 {
		lines = append(lines, "Parameters: none")
	} else {
		lines = append(lines, "Parameters:")
		for _, name := range sortedKeys(metadata.Params) {
			lines = append(lines, fmt.Sprintf("  %s: %s", name, strings.TrimSpace(metadata.Params[name])))
		}
	}

	_, err := fmt.Fprintln(cmd.OutOrStdout(), strings.Join(lines, "\n"))

	return err
}

func roleTimeoutDetails(configuredRole role.Role) (string, string) {
	if configuredRole.Metadata.Provider.Timeout != "" {
		return configuredRole.Metadata.Provider.Timeout, "provider"
	}

	return defaultRoleTimeoutText, "cli default"
}
