package docker

import (
	"fmt"

	dockerlib "github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var (
	flagExecRoot  bool
	flagExecNoTTY bool
)

var execCmd = &cobra.Command{
	Use:          "exec [flags] <service> -- <command...>",
	Short:        "Run a command inside a Docker service",
	SilenceUsage: true,
	Long: `Run arbitrary commands inside a Compose service without locating the
generated Docker environment directory.

Examples:
  odooctl docker exec odoo -- python --version
  odooctl docker exec odoo -- ls /mnt/extra-addons
  odooctl docker exec --root odoo -- apt update
  odooctl docker exec -T db -- psql -U odoo -d odoo-190 -c "select now();"`,
	Args: cobra.MinimumNArgs(2),
	RunE: runExec,
}

func init() {
	execCmd.Flags().BoolVar(&flagExecRoot, "root", false, "Run command as root")
	execCmd.Flags().BoolVarP(&flagExecNoTTY, "no-tty", "T", false, "Disable pseudo-TTY allocation")
}

func runExec(cmd *cobra.Command, args []string) error {
	state, err := loadState()
	if err != nil {
		return err
	}
	service := args[0]
	command := args[1:]
	if service == "" || len(command) == 0 {
		return fmt.Errorf("usage: odooctl docker exec <service> -- <command...>")
	}
	composeArgs := []string{"exec"}
	if flagExecNoTTY {
		composeArgs = append(composeArgs, "-T")
	}
	if flagExecRoot {
		composeArgs = append(composeArgs, "--user", "root")
	}
	composeArgs = append(composeArgs, service)
	composeArgs = append(composeArgs, command...)
	return dockerlib.Compose(state, composeArgs...)
}
