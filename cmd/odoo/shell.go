package odoo

import (
	dockerlib "github.com/mart337i/odooctl/internal/docker"
	"github.com/spf13/cobra"
)

var shellCmd = &cobra.Command{
	Use:          "shell",
	Short:        "Open an Odoo Python shell",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := loadState()
		if err != nil {
			return err
		}
		return dockerlib.Compose(state, "exec", "odoo", "odoo", "shell", "-d", state.DBName())
	},
}
