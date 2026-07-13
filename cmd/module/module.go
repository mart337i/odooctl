package module

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "module",
	Short: "Manage Odoo modules",
	Long:  `Commands for creating and managing Odoo modules.`,
}

func init() {
	Cmd.AddCommand(scaffoldCmd)
	Cmd.AddCommand(listCmd)
	Cmd.AddCommand(depsCmd)
	Cmd.AddCommand(manifestCmd)
	Cmd.AddCommand(changedCmd)
	Cmd.AddCommand(testCmd)
	Cmd.AddCommand(upgradeCmd)
	Cmd.AddCommand(migrateCmd)
}
