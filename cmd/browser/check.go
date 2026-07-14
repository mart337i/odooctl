package browser

import (
	"fmt"
	"strings"

	"github.com/mart337i/odooctl/internal/output"
	"github.com/spf13/cobra"
)

var checkFlags = defaultBrowserFlags()
var flagExpectText string

type checkReport struct {
	PageReport pageReport `json:"page"`
	ExpectText string     `json:"expect_text"`
	Matched    bool       `json:"matched"`
}

var checkCmd = &cobra.Command{
	Use:          "check [path-or-url] --expect-text <text>",
	Short:        "Check that a page contains expected visible text",
	SilenceUsage: true,
	Args:         cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if flagExpectText == "" {
			return fmt.Errorf("--expect-text is required")
		}
		state, err := loadState()
		if err != nil {
			return err
		}
		target := ""
		if len(args) > 0 {
			target = args[0]
		}
		report, err := runPage(state, target, checkFlags, "", "")
		if err != nil {
			return err
		}
		matched := strings.Contains(strings.Join(report.VisibleText, "\n"), flagExpectText)
		check := checkReport{PageReport: report, ExpectText: flagExpectText, Matched: matched}
		if checkFlags.JSON {
			if err := output.PrintJSON(check); err != nil {
				return err
			}
			if !matched {
				return fmt.Errorf("expected text %q was not visible on %s", flagExpectText, report.URL)
			}
			return nil
		}
		if !matched {
			return fmt.Errorf("expected text %q was not visible on %s", flagExpectText, report.URL)
		}
		fmt.Printf("Matched %q on %s\n", flagExpectText, report.URL)
		return nil
	},
}

func init() {
	addPageFlags(checkCmd, &checkFlags)
	checkCmd.Flags().StringVar(&flagExpectText, "expect-text", "", "Visible text expected on the page")
}
