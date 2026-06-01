package cmd

import (
	"fmt"
	"os"

	"github.com/mart337i/odooctl/cmd/docker"
	"github.com/mart337i/odooctl/cmd/module"
	"github.com/spf13/cobra"
)

var version = "0.2.5"

var rootCmd = &cobra.Command{
	Use:   "odooctl",
	Short: "CLI tool for Odoo Docker development environments",
	Long:  `odooctl helps you create and manage Docker-based Odoo development environments.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(docker.Cmd)
	rootCmd.AddCommand(module.Cmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("odooctl %s\n", version)
	},
}
