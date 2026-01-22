package module

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "module",
	Short: "Manage Odoo modules",
	Long:  `Commands for creating and managing Odoo modules.`,
}

func init() {
	Cmd.AddCommand(scaffoldCmd)
}
