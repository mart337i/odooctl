package docker

import (
	"fmt"

	dockerlib "github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var composeCmd = &cobra.Command{
	Use:          "compose -- <docker compose args...>",
	Short:        "Run docker compose in the generated environment directory",
	SilenceUsage: true,
	Long: `Run docker compose with the current odooctl environment directory already
selected. Use -- before Compose flags so Cobra does not parse them.

Examples:
  odooctl docker compose ps
  odooctl docker compose -- ps --services
  odooctl docker compose -- top`,
	Args: cobra.ArbitraryArgs,
	RunE: runCompose,
}

func runCompose(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	args = stripLeadingSeparator(args)
	if len(args) == 0 {
		return fmt.Errorf("docker compose arguments are required")
	}
	return dockerlib.Compose(state, args...)
}

func stripLeadingSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}
