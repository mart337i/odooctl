package docker

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "docker",
	Short: "Manage Docker development environments",
	Long:  `Commands for creating and managing Odoo Docker development environments.`,
}

func init() {
	Cmd.AddCommand(createCmd)
	Cmd.AddCommand(composeCmd)
	Cmd.AddCommand(runCmd)
	Cmd.AddCommand(execCmd)
	Cmd.AddCommand(restartCmd)
	Cmd.AddCommand(stopCmd)
	Cmd.AddCommand(statusCmd)
	Cmd.AddCommand(logsCmd)
	Cmd.AddCommand(resetCmd)
	Cmd.AddCommand(installCmd)
	Cmd.AddCommand(testCmd)
	Cmd.AddCommand(editCmd)
	Cmd.AddCommand(pathCmd)
	Cmd.AddCommand(reconfigureCmd)
	Cmd.AddCommand(gotoCmd)
	Cmd.AddCommand(dbCmd)
	Cmd.AddCommand(sqlCmd)
	Cmd.AddCommand(odooBinCmd)
	Cmd.AddCommand(shellCmd)
	Cmd.AddCommand(openCmd)
	Cmd.AddCommand(debugInfoCmd)
	Cmd.AddCommand(dumpCmd)
	Cmd.AddCommand(depsCmd)
}
