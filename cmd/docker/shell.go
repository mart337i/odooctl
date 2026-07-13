package docker

import (
	"github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var (
	flagService   string
	flagOdooShell bool
	flagShellRoot bool
)

var shellCmd = &cobra.Command{
	Use:          "shell [service]",
	Short:        "Open bash or Odoo shell in container",
	SilenceUsage: true,
	Long: `Opens an interactive shell inside a Docker container.

Examples:
  odooctl docker shell              # Bash shell in odoo container
  odooctl docker shell db           # Bash shell in db container
  odooctl docker shell --root       # Root shell in odoo container
  odooctl docker shell --odoo       # Odoo Python shell
  odooctl docker shell --service db # Bash shell in db container`,
	Args: cobra.MaximumNArgs(1),
	RunE: runShell,
}

func init() {
	shellCmd.Flags().StringVarP(&flagService, "service", "s", "odoo", "Service name to connect to")
	shellCmd.Flags().BoolVarP(&flagOdooShell, "odoo", "o", false, "Open Odoo shell instead of bash")
	shellCmd.Flags().BoolVar(&flagShellRoot, "root", false, "Open shell as root")
}

func runShell(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}

	service := flagService
	if len(args) > 0 {
		service = args[0]
	}
	composeArgs := []string{"exec"}
	if flagShellRoot {
		composeArgs = append(composeArgs, "--user", "root")
	}

	if flagOdooShell {
		// Odoo shell mode
		database := state.DBName()
		composeArgs = append(composeArgs, service, "odoo", "shell", "-d", database)
		return docker.Compose(state, composeArgs...)
	}

	// Bash shell mode
	composeArgs = append(composeArgs, service, "bash")
	return docker.Compose(state, composeArgs...)
}
