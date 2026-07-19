package cli

import (
	"errors"
	"strings"

	"github.com/normahq/codex-acp-bridge/pkg/cobracmd"
	"github.com/spf13/cobra"
)

func bridgeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "Run built-in ACP bridges",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("a bridge command is required")
		},
	}

	codex := cobracmd.New()
	codex.Use = "codex"
	codex.Short = "Expose Codex as an ACP agent over stdio"
	codex.Example = strings.ReplaceAll(codex.Example, "codex-acp-bridge", "callee bridge codex")
	cmd.AddCommand(codex)

	return cmd
}
