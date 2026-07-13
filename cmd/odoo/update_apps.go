package odoo

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var updateAppsCmd = &cobra.Command{
	Use:          "update-apps",
	Short:        "Update Odoo's apps/module list",
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := loadState()
		if err != nil {
			return err
		}
		script := "env['ir.module.module'].update_list()\nenv.cr.commit()\nprint('Apps list updated')\n"
		if _, err := runOdooShellScript(state, script, false); err != nil {
			return err
		}
		fmt.Printf("%s Apps list updated\n", color.GreenString("✓"))
		return nil
	},
}
