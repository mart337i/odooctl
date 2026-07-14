package cmd

import (
	"fmt"
	"os"

	"github.com/mart337i/odooctl/cmd/ai"
	browsercmd "github.com/mart337i/odooctl/cmd/browser"
	"github.com/mart337i/odooctl/cmd/docker"
	"github.com/mart337i/odooctl/cmd/module"
	odoocmd "github.com/mart337i/odooctl/cmd/odoo"
	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var version = "0.2.5"

var rootCmd = &cobra.Command{
	Use:           "odooctl",
	Short:         "CLI tool for Odoo Docker development environments",
	Long:          `odooctl helps you create and manage Docker-based Odoo development environments.`,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(ai.Cmd)
	rootCmd.AddCommand(browsercmd.Cmd)
	rootCmd.AddCommand(docker.Cmd)
	rootCmd.AddCommand(module.Cmd)
	rootCmd.AddCommand(odoocmd.Cmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		jsonOutput, _ := cmd.Flags().GetBool("json")
		if jsonOutput {
			_ = output.PrintJSON(map[string]string{"version": version})
			return
		}
		fmt.Printf("odooctl %s\n", version)
	},
}

func init() {
	versionCmd.Flags().Bool("json", false, "Print JSON output")
}
