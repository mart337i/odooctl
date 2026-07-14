package browser

import (
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var inspectFlags = defaultBrowserFlags()

var inspectCmd = &cobra.Command{
	Use:          "inspect [path-or-url]",
	Short:        "Inspect a page and return AI-readable text, console errors, and requests",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		state, err := loadState()
		if err != nil {
			return err
		}
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		report, err := runPage(state, target, inspectFlags, "", "")
		if err != nil {
			return err
		}
		if inspectFlags.JSON {
			return output.PrintJSON(report)
		}
		printPageSummary(report)
		return nil
	},
}

func init() {
	addPageFlags(inspectCmd, &inspectFlags)
}

func printPageSummary(report pageReport) {
	fmt.Printf("URL:   %s\n", report.URL)
	fmt.Printf("Title: %s\n", report.Title)
	if len(report.ConsoleErrors) > 0 {
		fmt.Println("\nConsole errors:")
		for _, msg := range report.ConsoleErrors {
			fmt.Printf("- %s: %s\n", msg.Type, msg.Text)
		}
	}
	if len(report.FailedRequests) > 0 {
		fmt.Println("\nFailed requests:")
		for _, req := range report.FailedRequests {
			fmt.Printf("- %s %s: %s\n", req.Method, req.URL, req.Error)
		}
	}
	if len(report.VisibleText) > 0 {
		fmt.Println("\nVisible text:")
		fmt.Println(strings.Join(report.VisibleText, "\n"))
	}
}

func addPageFlags(cmd *cobra.Command, flags *browserFlags) {
	cmd.Flags().StringVar(&flags.Login, "login", "", "Odoo login before opening the target page")
	cmd.Flags().StringVar(&flags.Password, "password", "", "Odoo password for --login")
	cmd.Flags().BoolVar(&flags.JSON, "json", false, "Print JSON output")
	cmd.Flags().IntVar(&flags.Timeout, "timeout", flags.Timeout, "Browser timeout in milliseconds")
	cmd.Flags().IntVar(&flags.Wait, "wait", flags.Wait, "Extra wait after page load in milliseconds")
	cmd.Flags().IntVar(&flags.Width, "width", flags.Width, "Viewport width")
	cmd.Flags().IntVar(&flags.Height, "height", flags.Height, "Viewport height")
}
