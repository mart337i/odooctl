package docker

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "docker",
	Short: "Manage Docker development environments",
	Long:  `Commands for creating and managing Odoo Docker development environments.`,
}

func init() {
	Cmd.AddCommand(createCmd)
	Cmd.AddCommand(runCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(logsCmd)
	Cmd.AddCommand(resetCmd)
	Cmd.AddCommand(installCmd)
	Cmd.AddCommand(dbCmd)
	Cmd.AddCommand(odooBinCmd)
	Cmd.AddCommand(shellCmd)
}
