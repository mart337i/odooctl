package odoo

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "odoo",
	Short: "Interact with the Odoo runtime inside Docker",
	Long:  `Odoo-specific helpers for shell access, ORM evaluation, app list updates, and module state inspection.`,
}

func init() {
	Cmd.AddCommand(shellCmd)
	Cmd.AddCommand(evalCmd)
	Cmd.AddCommand(updateAppsCmd)
	Cmd.AddCommand(moduleStateCmd)
}
