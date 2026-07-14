package browser

import "github.com/spf13/cobra"

var Cmd = &cobra.Command{
	Use:   "browser",
	Short: "Inspect Odoo with headless Playwright Chromium",
	Long:  `Browser tooling for AI-assisted design/debugging and Odoo web test readiness checks.`,
}

func init() {
	Cmd.AddCommand(doctorCmd)
	Cmd.AddCommand(inspectCmd)
	Cmd.AddCommand(snapshotCmd)
	Cmd.AddCommand(screenshotCmd)
	Cmd.AddCommand(checkCmd)
	Cmd.AddCommand(traceCmd)
}
