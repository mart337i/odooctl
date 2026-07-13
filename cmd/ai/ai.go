package ai

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "ai",
	Short: "Generate AI-friendly development context",
	Long:  `Commands that generate local, redacted context and prompts for AI-assisted Odoo development.`,
}

func init() {
	Cmd.AddCommand(contextCmd)
	Cmd.AddCommand(debugReportCmd)
	Cmd.AddCommand(promptCmd)
}
