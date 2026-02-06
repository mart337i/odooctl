package docker

import (
	"github.com/egeskov/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var (
	flagService   string
	flagOdooShell bool
)

var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Open bash or Odoo shell in container",
	Long: `Opens an interactive shell inside a Docker container.

Examples:
  odooctl docker shell              # Bash shell in odoo container
  odooctl docker shell --odoo       # Odoo Python shell
  odooctl docker shell --service db # Bash shell in db container`,
	RunE: runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&flagService, "service", "s", "odoo", "Service name to connect to")
	shellCmd.Flags().BoolVarP(&flagOdooShell, "odoo", "o", false, "Open Odoo shell instead of bash")
}

func runShell(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	if flagOdooShell {
		// Odoo shell mode
		database := state.DBName()
		return docker.Compose(state, "exec", flagService, "odoo", "shell", "-d", database)
	}

	// Bash shell mode
	return docker.Compose(state, "exec", flagService, "bash")
}
