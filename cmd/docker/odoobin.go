package docker

import (
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var odooBinCmd = &cobra.Command{
	Use:   "odoo-bin [args...]",
	Short: "Run odoo-bin commands directly",
	Long: `Executes odoo-bin commands inside the Odoo container.

Examples:
  odooctl docker odoo-bin --help
  odooctl docker odoo-bin -d mydb --test-enable -i sale
  odooctl docker odoo-bin shell -d mydb`,
	RunE:               runOdooBin,
	DisableFlagParsing: true, // Pass all args to odoo-bin
}

func runOdooBin(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	// Build command: docker compose exec odoo odoo <args>
	execArgs := []string{"exec", "odoo", "odoo"}
	execArgs = append(execArgs, args...)

	return docker.Compose(state, execArgs...)
}
